package upgrade

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"exam_notify/internal/infrastructure/persistence"
)

// Frozen baseline: future schema changes must not alter this manifest.
//
//go:embed baseline.json
var baselineJSON []byte

type indexColumn struct {
	Name string `json:"name"`
	Desc int    `json:"desc"`
	Coll string `json:"coll"`
}
type schemaIndex struct {
	Unique  int           `json:"unique"`
	Origin  string        `json:"origin"`
	Partial int           `json:"partial"`
	Columns []indexColumn `json:"columns"`
}
type schemaTable struct {
	Columns     map[string][]any `json:"columns"`
	ForeignKeys [][]any          `json:"foreignKeys"`
	Indexes     []schemaIndex    `json:"indexes"`
}

// recognizeBaseline compares names/types/constraints rather than column order.
// It executes no DDL, repair SQL or arbitrary expressions from the input file.
func recognizeBaseline(ctx context.Context, exec persistence.Executor) error {
	return recognizeSchema(ctx, exec, false)
}
func recognizeHistoricalGoals(ctx context.Context, exec persistence.Executor) error {
	return recognizeSchema(ctx, exec, true)
}
func recognizeSchema(ctx context.Context, exec persistence.Executor, allowGoalHistory bool) error {
	var want map[string]schemaTable
	if err := json.Unmarshal(baselineJSON, &want); err != nil {
		return err
	}
	rows, err := exec.QueryContext(ctx, `SELECT type,name FROM main.sqlite_schema WHERE name NOT GLOB 'sqlite_*' AND name != ? ORDER BY name`, versionTable)
	if err != nil {
		return err
	}
	names := []string{}
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			rows.Close()
			return err
		}
		if kind != "table" {
			rows.Close()
			return fmt.Errorf("%w: unexpected %s %s", ErrUnknownSchema, kind, name)
		}
		if _, ok := want[name]; !ok {
			rows.Close()
			return fmt.Errorf("%w: unexpected table %s", ErrUnknownSchema, name)
		}
		names = append(names, name)
	}
	if err := closeRows(rows); err != nil {
		return err
	}
	if allowGoalHistory && len(names) == len(want)-1 {
		hasGoals := false
		for _, name := range names {
			hasGoals = hasGoals || name == "stage_goals"
		}
		if !hasGoals {
			delete(want, "stage_goals")
		}
	}
	if len(names) != len(want) {
		return fmt.Errorf("%w: missing baseline tables", ErrUnknownSchema)
	}
	for _, name := range names {
		got, err := readTable(ctx, exec, name)
		if err != nil {
			return err
		}
		// JSON normalizes integer interfaces to float64 for comparison.
		data, err := json.Marshal(got)
		if err != nil {
			return err
		}
		var normalized schemaTable
		if err := json.Unmarshal(data, &normalized); err != nil {
			return err
		}
		expected := want[name]
		if allowGoalHistory && name == "stage_goals" {
			if _, exists := normalized.Columns["accepted_at"]; !exists {
				delete(expected.Columns, "accepted_at")
			}
		}
		// Known five-table releases used this old color default. The schema remains
		// writable with the baseline; retain the original default rather than rebuild.
		if name == "subjects" {
			if col := normalized.Columns["color"]; len(col) > 2 && col[2] == "'#7668e8'" {
				col[2] = "'#007AFF'"
			}
		}
		sortIndexes(expected.Indexes)
		sortIndexes(normalized.Indexes)
		if !reflect.DeepEqual(expected, normalized) {
			return fmt.Errorf("%w: table %s differs from baseline 1", ErrUnknownSchema, name)
		}
	}
	return nil
}

// The legacy column/key matcher alone cannot expose CHECK, deferrability or
// ON CONFLICT REPLACE/IGNORE. Inspect the original SQL before offering adoption.
func recognizeLegacyBaseline(ctx context.Context, exec persistence.Executor) (bool, error) {
	ok, err := persistence.RecognizeHistoricalBaseline(ctx, exec)
	if err != nil || !ok {
		return ok, err
	}
	for _, name := range []string{"subjects", "plans", "sessions", "activity_logs", "settings"} {
		if _, err := readTable(ctx, exec, name); err != nil {
			return false, err
		}
	}
	return true, nil
}

func readTable(ctx context.Context, exec persistence.Executor, name string) (schemaTable, error) {
	out := schemaTable{Columns: map[string][]any{}, ForeignKeys: [][]any{}, Indexes: []schemaIndex{}}
	var definition string
	if err := exec.QueryRowContext(ctx, `SELECT sql FROM main.sqlite_schema WHERE type='table' AND name=?`, name).Scan(&definition); err != nil {
		return out, err
	}
	tokens, err := sqlTokens(definition)
	if err != nil {
		return out, err
	}
	// These constraint/table features do not exist in the frozen baseline and
	// cannot be inferred completely from table_xinfo/foreign_key_list alone.
	for _, token := range tokens {
		switch token {
		case "CHECK", "GENERATED", "COLLATE", "DEFERRABLE", "WITHOUT", "STRICT", "CONSTRAINT", "VIRTUAL", "CONFLICT", "AUTOINCREMENT":
			return out, fmt.Errorf("%w: unexpected table feature %s in %s", ErrUnknownSchema, token, name)
		}
	}

	var wr, strict int
	if err := exec.QueryRowContext(ctx, `SELECT wr,strict FROM pragma_table_list WHERE schema='main' AND name=?`, name).Scan(&wr, &strict); err != nil {
		return out, err
	}
	if wr != 0 || strict != 0 {
		return out, fmt.Errorf("%w: table mode %s", ErrUnknownSchema, name)
	}
	rows, err := exec.QueryContext(ctx, `SELECT name,type,"notnull",dflt_value,pk,hidden FROM pragma_table_xinfo(?, 'main')`, name)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var col, kind string
		var nn, pk, hidden int
		var def sql.NullString
		if err := rows.Scan(&col, &kind, &nn, &def, &pk, &hidden); err != nil {
			rows.Close()
			return out, err
		}
		var defaultValue any
		if def.Valid {
			defaultValue = strings.TrimSpace(def.String)
		}
		out.Columns[col] = []any{strings.ToUpper(kind), nn, defaultValue, pk, hidden}
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}
	rows, err = exec.QueryContext(ctx, `SELECT "table","from","to",on_update,on_delete,match FROM pragma_foreign_key_list(?, 'main') ORDER BY "table","from","to"`, name)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var a, b, c, d, e, f string
		if err := rows.Scan(&a, &b, &c, &d, &e, &f); err != nil {
			rows.Close()
			return out, err
		}
		out.ForeignKeys = append(out.ForeignKeys, []any{a, b, c, d, e, f})
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}
	rows, err = exec.QueryContext(ctx, `SELECT name,"unique",origin,partial FROM pragma_index_list(?, 'main')`, name)
	if err != nil {
		return out, err
	}
	type namedIndex struct {
		name  string
		value schemaIndex
	}
	indexes := []namedIndex{}
	for rows.Next() {
		var x namedIndex
		if err := rows.Scan(&x.name, &x.value.Unique, &x.value.Origin, &x.value.Partial); err != nil {
			rows.Close()
			return out, err
		}
		indexes = append(indexes, x)
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}
	for _, x := range indexes {
		rows, err = exec.QueryContext(ctx, `SELECT name,"desc",coll FROM pragma_index_xinfo(?, 'main') WHERE "key"=1 ORDER BY seqno`, x.name)
		if err != nil {
			return out, err
		}
		x.value.Columns = []indexColumn{}
		for rows.Next() {
			var c indexColumn
			if err := rows.Scan(&c.Name, &c.Desc, &c.Coll); err != nil {
				rows.Close()
				return out, err
			}
			x.value.Columns = append(x.value.Columns, c)
		}
		if err := closeRows(rows); err != nil {
			return out, err
		}
		out.Indexes = append(out.Indexes, x.value)
	}
	return out, nil
}

func sortIndexes(indexes []schemaIndex) {
	sort.Slice(indexes, func(i, j int) bool {
		a, _ := json.Marshal(indexes[i])
		b, _ := json.Marshal(indexes[j])
		return string(a) < string(b)
	})
}
func closeRows(rows *sql.Rows) error { err := rows.Err(); return errors.Join(err, rows.Close()) }
