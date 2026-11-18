package main

import (
	"context"
	"net/http"
)

type databaseEffectsKey struct{}

func requestWithDatabaseEffects(r *http.Request) (*http.Request, *[]func()) {
	effects := []func(){}
	return r.WithContext(context.WithValue(r.Context(), databaseEffectsKey{}, &effects)), &effects
}

// Effects are registered only after commit and run after releasing the lifecycle
// lock, so a synchronous settings callback may safely read the database again.
func queueAfterDatabase(r *http.Request, work func()) {
	effects := r.Context().Value(databaseEffectsKey{}).(*[]func())
	*effects = append(*effects, work)
}
