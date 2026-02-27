package main

import (
	"bytes"
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
