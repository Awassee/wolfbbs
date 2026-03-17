package sshserver

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecuteAppUpgradeCommandRequiresConfiguredCommand(t *testing.T) {
	previous := appUpgradeExec
	defer func() { appUpgradeExec = previous }()
	appUpgradeExec = func(ctx context.Context, command, workDir string, env []string) (string, error) {
		t.Fatal("executor should not run without configured command")
		return "", nil
	}

	t.Setenv(appUpgradeCommandEnv, "")
	if _, err := executeAppUpgradeCommand("sysop"); err == nil || !strings.Contains(err.Error(), appUpgradeCommandEnv) {
		t.Fatalf("expected missing command error, got %v", err)
	}
}

func TestExecuteAppUpgradeCommandPassesCommandEnvAndWorkDir(t *testing.T) {
	previous := appUpgradeExec
	defer func() { appUpgradeExec = previous }()

	capturedCommand := ""
	capturedWorkDir := ""
	capturedEnv := []string{}
	appUpgradeExec = func(ctx context.Context, command, workDir string, env []string) (string, error) {
		capturedCommand = command
		capturedWorkDir = workDir
		capturedEnv = append([]string{}, env...)
		return "ok", nil
	}

	t.Setenv(appUpgradeCommandEnv, "bash install.sh --rapid-upgrade --yes")
	t.Setenv(appUpgradeWorkDirEnv, "/tmp/wolfbbs")
	t.Setenv(appUpgradeTimeoutEnv, "30")

	out, err := executeAppUpgradeCommand("sysop")
	if err != nil {
		t.Fatalf("execute app upgrade: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected output ok, got %q", out)
	}
	if capturedCommand != "bash install.sh --rapid-upgrade --yes" {
		t.Fatalf("unexpected command %q", capturedCommand)
	}
	if capturedWorkDir != "/tmp/wolfbbs" {
		t.Fatalf("unexpected work dir %q", capturedWorkDir)
	}
	if !sliceContains(capturedEnv, "WOLFBBS_UPGRADE_TRIGGER=ssh") {
		t.Fatalf("missing upgrade trigger env in %#v", capturedEnv)
	}
	if !sliceContains(capturedEnv, "WOLFBBS_UPGRADE_USER=sysop") {
		t.Fatalf("missing upgrade user env in %#v", capturedEnv)
	}
}

func TestExecuteAppUpgradeCommandWrapsExecutorFailure(t *testing.T) {
	previous := appUpgradeExec
	defer func() { appUpgradeExec = previous }()
	appUpgradeExec = func(ctx context.Context, command, workDir string, env []string) (string, error) {
		return "upgrade output", errors.New("exit status 1")
	}

	t.Setenv(appUpgradeCommandEnv, "bad-command")
	out, err := executeAppUpgradeCommand("sysop")
	if out != "upgrade output" {
		t.Fatalf("unexpected output %q", out)
	}
	if err == nil || !strings.Contains(err.Error(), "upgrade command failed") {
		t.Fatalf("expected wrapped command failure, got %v", err)
	}
}

func TestLimitCommandOutputTruncatesWhenNeeded(t *testing.T) {
	got := limitCommandOutput("abcdef", 4)
	if got != "abcd\n...[truncated]" {
		t.Fatalf("unexpected truncated output %q", got)
	}
}

func sliceContains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
