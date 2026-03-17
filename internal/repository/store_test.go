package repository

import (
	"os"
	"testing"
)

func withEnv(t *testing.T, key, value string) {
	t.Helper()
	original, had := os.LookupEnv(key)
	if had {
		t.Cleanup(func() { _ = os.Setenv(key, original) })
	} else {
		t.Cleanup(func() { _ = os.Unsetenv(key) })
	}
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
}

func TestResolveDatabaseURLUsesExplicitDSN(t *testing.T) {
	withEnv(t, "WOLFBBS_DATABASE_URL", "postgres://explicit")
	withEnv(t, "DATABASE_URL", "")
	withEnv(t, "PGHOST", "")
	withEnv(t, "PGUSER", "")
	withEnv(t, "PGPASSWORD", "")
	withEnv(t, "PGDATABASE", "")
	withEnv(t, "PGPORT", "")

	got := ResolveDatabaseURL()
	if got != "postgres://explicit" {
		t.Fatalf("expected explicit URL, got %q", got)
	}
}

func TestResolveDatabaseURLBuildsFromPGVars(t *testing.T) {
	withEnv(t, "WOLFBBS_DATABASE_URL", "")
	withEnv(t, "DATABASE_URL", "")
	withEnv(t, "PGHOST", "db.internal")
	withEnv(t, "PGPORT", "5433")
	withEnv(t, "PGUSER", "bbs")
	withEnv(t, "PGPASSWORD", "pass")
	withEnv(t, "PGDATABASE", "bbsdb")
	withEnv(t, "PGSSLMODE", "disable")

	got := ResolveDatabaseURL()
	want := "postgres://bbs:pass@db.internal:5433/bbsdb?sslmode=disable"
	if got != want {
		t.Fatalf("unexpected DSN.\n got:  %q\n want: %q", got, want)
	}
}
