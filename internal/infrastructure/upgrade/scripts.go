package upgrade

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// mapFS keeps the compiled target isolated even if a later SQL file exists.
type mapFS map[string][]byte

func (m mapFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	data, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &scriptFile{Reader: bytes.NewReader(data), name: name, size: int64(len(data))}, nil
}
func (m mapFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, fs.FileInfoToDirEntry(scriptInfo{name: name, size: int64(len(m[name]))}))
	}
	return entries, nil
}

type scriptFile struct {
	*bytes.Reader
	name string
	size int64
}

func (f *scriptFile) Close() error               { return nil }
func (f *scriptFile) Stat() (fs.FileInfo, error) { return scriptInfo{name: f.name, size: f.size}, nil }

type scriptInfo struct {
	name string
	size int64
}

func (f scriptInfo) Name() string       { return f.name }
func (f scriptInfo) Size() int64        { return f.size }
func (f scriptInfo) Mode() fs.FileMode  { return 0444 }
func (f scriptInfo) ModTime() time.Time { return time.Time{} }
func (f scriptInfo) IsDir() bool        { return false }
func (f scriptInfo) Sys() any           { return nil }

var _ io.Reader = (*scriptFile)(nil)

// Scripts are trusted application sources, but must not take transaction
// ownership or change connection-level state. Scan outside SQL literals and
// comments, including StatementBegin blocks and multiple statements per line.
var envSubAnnotation = regexp.MustCompile(`(?im)^\s*--\s*\+goose\s+ENVSUB\s+ON\s*$`)

func checkScript(script string) error {
	if envSubAnnotation.MatchString(script) {
		return fmt.Errorf("environment substitution is forbidden in upgrade SQL")
	}
	if strings.Contains(strings.ToUpper(script), "+GOOSE NO TRANSACTION") {
		return fmt.Errorf("NO TRANSACTION is forbidden")
	}
	tokens, err := sqlTokens(script)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		switch token {
		case "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE", "TRANSACTION", "VACUUM", "ATTACH", "DETACH", "PRAGMA":
			return fmt.Errorf("SQL keyword %s is forbidden in upgrade scripts", token)
		}
	}
	// BEGIN/END are permitted inside CREATE TRIGGER, and END in CASE
	// expressions. A standalone BEGIN or END would steal transaction ownership.
	trigger := false
	head := []string{}
	for _, token := range append(tokens, ";") {
		if token != ";" {
			head = append(head, token)
			continue
		}
		if len(head) > 0 {
			if head[0] == "CREATE" && len(head) > 1 {
				n := 1
				if head[n] == "TEMP" || head[n] == "TEMPORARY" {
					n++
				}
				if n < len(head) && head[n] == "TRIGGER" {
					trigger = true
				}
			}
			if head[0] == "BEGIN" || (head[0] == "END" && !trigger) {
				return fmt.Errorf("explicit transaction boundary is forbidden")
			}
			if head[0] == "END" && trigger {
				trigger = false
			}
		}
		head = nil
	}
	return nil
}

// sqlTokens ignores literal contents and comments; quoted identifiers are opaque.
func sqlTokens(script string) ([]string, error) {
	tokens := []string{}
	for i := 0; i < len(script); {
		c := script[i]
		if i+1 < len(script) && script[i:i+2] == "--" {
			for i < len(script) && script[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(script) && script[i:i+2] == "/*" {
			end := strings.Index(script[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated SQL comment")
			}
			i += end + 4
			continue
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			end := c
			if c == '[' {
				end = ']'
			}
			i++
			closed := false
			for i < len(script) {
				if script[i] == end {
					if end != ']' && i+1 < len(script) && script[i+1] == end {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			tokens = append(tokens, "#")
			if !closed {
				return nil, fmt.Errorf("unterminated SQL literal")
			}
			continue
		}
		if unicode.IsLetter(rune(c)) || c == '_' {
			start := i
			i++
			for i < len(script) && (unicode.IsLetter(rune(script[i])) || unicode.IsDigit(rune(script[i])) || script[i] == '_') {
				i++
			}
			tokens = append(tokens, strings.ToUpper(script[start:i]))
			continue
		}
		if c == ';' {
			tokens = append(tokens, ";")
		}
		i++
	}

	return tokens, nil
}
