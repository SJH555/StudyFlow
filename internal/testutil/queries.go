// Package testutil provides lifecycle-scoped database fixtures for module tests.
package testutil

import (
	"context"
	"database/sql"
	"exam_notify/internal/infrastructure/persistence"
	"testing"
)

// Borrow holds a scope for the test's lifetime. Only the test uses Queries;
// the scope goroutine waits until cleanup, so there is no concurrent SQL use.
func Borrow(t *testing.T, db *sql.DB, path string) *persistence.Queries {
	t.Helper()
	owner, err := persistence.AdoptDatabase(db, path)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan *persistence.Queries, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- owner.WithAccess(context.Background(), func(a *persistence.Access) error {
			ready <- a.Queries(context.Background())
			<-release
			return nil
		})
	}()
	queries := <-ready
	t.Cleanup(func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	return queries
}
