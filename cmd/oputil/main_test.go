package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsageWhenNoArgs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(out.String(), "WolfBBS oputil") {
		t.Fatalf("expected usage output, got: %s", out.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"nope"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("expected unknown command error, got: %s", errOut.String())
	}
}

func TestRunHelpIncludesNetworkAndMods(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	body := out.String()
	if !strings.Contains(body, "network status") {
		t.Fatalf("expected network usage in help output, got: %s", body)
	}
	if !strings.Contains(body, "mods list") {
		t.Fatalf("expected mods usage in help output, got: %s", body)
	}
}

func TestRunNetworkStatus(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "oputil-network-status.sqlite")
	dsn := "sqlite://" + dbPath
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--db", dsn, "network", "status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "spool=") {
		t.Fatalf("expected spool status output, got: %s", out.String())
	}
}
