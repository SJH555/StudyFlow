//go:build windows && upgradegui

package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"exam_notify/internal/infrastructure/persistence"
	"fmt"
	"hash/fnv"
	"modernc.org/sqlite"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type upgradeGUIGuard struct{}

func (upgradeGUIGuard) ListenForSecondLaunch(func()) error { return nil }
func (upgradeGUIGuard) StopListening() error               { return nil }
func (upgradeGUIGuard) Close() error                       { return nil }

// TestMain is compiled only for the disposable Windows WebView UI harness. It
// runs the real startup controller, services, assets, and desktop lifecycle.
// It never acquires or interrupts the user's regular application's instance.
func TestMain(_ *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--verify" {
		if err := verifyUpgradeGUIFixture(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	// go test keeps the import path for package main; production go build uses
	// main. Register the generated production IDs on the same actual methods.
	for _, service := range []any{(*APIService)(nil), (*StartupService)(nil)} {
		typ := reflect.TypeOf(service)
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			h := fnv.New32a()
			_, _ = h.Write([]byte("main." + typ.Elem().Name() + "." + method.Name))
			application.RegisterBindingMethodID(method.Func.Interface(), h.Sum32())
		}
	}
	dir := filepath.Dir(exe)
	selected, err := os.ReadFile(filepath.Join(dir, "case.txt"))
	if err != nil {
		panic(err)
	}
	name := strings.TrimSpace(string(selected))
	if !filepath.IsLocal(name) || strings.ContainsAny(name, "/\\") {
		panic("invalid QA case")
	}
	root := filepath.Join(dir, "cases", name)
	marker, err := os.ReadFile(filepath.Join(root, ".studyflow-upgrade-qa"))
	if err != nil || string(marker) != "temporary upgrade UI fixture\n" {
		panic("not an owned QA fixture")
	}
	paths := startupDatabasePaths{locationFile: filepath.Join(root, "storage-location.txt"), defaultDir: filepath.Join(root, "data"), installDir: root, legacyConfigDir: filepath.Join(root, "legacy")}
	// Only the disposable extreme-scenario binary supplies these SQL functions.
	if err := sqlite.RegisterScalarFunction("studyflow_upgrade_qa_checkpoint", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		phase := args[0].(string)
		if phase == "fail-once" {
			if _, err := os.Stat(filepath.Join(root, "resume")); os.IsNotExist(err) {
				return nil, fmt.Errorf("QA injected SQL function error after DML and DDL")
			}
			return int64(1), nil
		}
		if phase == "tick" {
			if _, err := os.Stat(filepath.Join(root, "release")); err == nil {
				return int64(1), nil
			}
			time.Sleep(5 * time.Millisecond)
			return int64(0), nil
		}
		return int64(1), os.WriteFile(filepath.Join(root, "checkpoint.txt"), []byte(phase), 0600)
	}); err != nil {
		panic(err)
	}
	store, startup := prepareStartupUpgrade(paths)
	err = runDesktopWithStartup(store, upgradeGUIGuard{}, root, paths.defaultDir, paths.locationFile, startup, newDesktopTransport(), func(app *application.App) error {
		if _, resumeErr := os.Stat(filepath.Join(root, "resume")); strings.HasPrefix(name, "full-") && os.IsNotExist(resumeErr) {
			if err := store.database.WithAccess(context.Background(), func(a *persistence.Access) error {
				return a.WithTransactionProvider(context.Background(), func(db *sql.DB, _ persistence.PinnedTransactionRunner) error {
					var pages int
					if err := db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
						return err
					}
					_, err := db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages+16))
					return err
				})
			}); err != nil {
				panic(err)
			}
		}
		if strings.HasPrefix(name, "committed-") {
			activate := startup.activate
			startup.activate = func() error {
				if _, err := os.Stat(filepath.Join(root, "resume")); os.IsNotExist(err) {
					if err := os.WriteFile(filepath.Join(root, "checkpoint.txt"), []byte("after-commit"), 0600); err != nil {
						return err
					}
					for {
						if _, err := os.Stat(filepath.Join(root, "resume")); err == nil {
							break
						}
						time.Sleep(5 * time.Millisecond)
					}
				}
				return activate()
			}
		}
		for _, window := range app.Window.GetAll() {
			window.SetTitle("StudyFlow 升级测试")
		}
		return app.Run()
	})
	report := struct {
		Status startupStatus `json:"status"`
		Error  string        `json:"error"`
	}{Status: startup.snapshot()}
	if err != nil {
		report.Error = err.Error()
	}
	data, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "ui-result.json"), data, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
