// Package storetest opens the store on the integration tests' database,
// the one the test-integration task provisions.
package storetest

import (
	"log/slog"
	"os"
	"testing"

	"github.com/perfectra1n/kritique/internal/store"
)

// Env is the environment variable key, skipping the test when it is
// unset.
func Env(t testing.TB, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Skipf("%s not set", key)
	}
	return v
}

// Open opens the store on KRITIQUE_TEST_APP_URL and KRITIQUE_TEST_OWNER_URL,
// migrated for the kritique_app and kritique_runner roles and closed when the
// test ends.
func Open(t testing.TB) *store.Store {
	t.Helper()
	ctx := t.Context()
	st, err := store.Open(ctx, store.Options{
		AppURL: Env(t, "KRITIQUE_TEST_APP_URL"), OwnerURL: Env(t, "KRITIQUE_TEST_OWNER_URL"),
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("storetest: open: %v", err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx, "kritique_app", "kritique_runner"); err != nil {
		t.Fatalf("storetest: migrate: %v", err)
	}
	return st
}
