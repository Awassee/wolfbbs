package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type functionRegistry struct {
	Version int                      `json:"version"`
	Entries []functionRegistryRecord `json:"entries"`
}

type functionRegistryRecord struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Entrypoint      string `json:"entrypoint"`
	ConfigUI        string `json:"config_ui"`
	StatusUI        string `json:"status_ui"`
	StateVisibility string `json:"state_visibility"`
	Notes           string `json:"notes"`
}

func TestFunctionRegistryCoverage(t *testing.T) {
	root := repoRoot(t)
	registryPath := filepath.Join(root, "docs", "function-registry.json")
	raw, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read function registry: %v", err)
	}

	var registry functionRegistry
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatalf("decode function registry: %v", err)
	}
	if len(registry.Entries) == 0 {
		t.Fatal("function registry has no entries")
	}

	byID := map[string]functionRegistryRecord{}
	for _, row := range registry.Entries {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			t.Fatal("registry entry has empty id")
		}
		if _, exists := byID[id]; exists {
			t.Fatalf("duplicate registry id: %s", id)
		}
		if strings.TrimSpace(row.Type) == "" {
			t.Fatalf("entry %s missing type", id)
		}
		if strings.TrimSpace(row.Entrypoint) == "" {
			t.Fatalf("entry %s missing entrypoint", id)
		}
		if strings.TrimSpace(row.ConfigUI) == "" {
			t.Fatalf("entry %s missing config_ui", id)
		}
		if strings.TrimSpace(row.StatusUI) == "" {
			t.Fatalf("entry %s missing status_ui", id)
		}
		if strings.TrimSpace(row.StateVisibility) == "" {
			t.Fatalf("entry %s missing state_visibility", id)
		}
		if strings.Contains(strings.ToUpper(row.Notes), "TODO") {
			t.Fatalf("entry %s contains TODO placeholder in notes", id)
		}
		byID[id] = row
	}

	// Discovered routes in web companion must be represented as web:<route>.
	webMain := mustRead(t, filepath.Join(root, "cmd", "wolfbbs-web", "main.go"))
	routeMatches := regexp.MustCompile(`http\.Handle(?:Func)?\("([^"]+)"`).FindAllStringSubmatch(webMain, -1)
	routeSet := map[string]struct{}{}
	for _, m := range routeMatches {
		if len(m) < 2 {
			continue
		}
		routeSet[m[1]] = struct{}{}
	}
	for route := range routeSet {
		id := "web:" + route
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing registry entry for discovered web route %s", route)
		}
	}

	// SSH state machine states must be represented as tui:<state>.
	sshServer := mustRead(t, filepath.Join(root, "internal", "sshserver", "server.go"))
	stateMatches := regexp.MustCompile(`\b(state[A-Z][A-Za-z0-9_]*)\b`).FindAllStringSubmatch(sshServer, -1)
	stateSet := map[string]struct{}{}
	for _, m := range stateMatches {
		if len(m) < 2 {
			continue
		}
		stateSet[m[1]] = struct{}{}
	}
	for state := range stateSet {
		id := "tui:" + state
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing registry entry for discovered ssh state %s", state)
		}
	}

	// Sysop CLI commands are mandatory in the registry.
	for _, cmd := range []string{
		"status",
		"users.list",
		"users.set-role",
		"boards.list",
		"boards.create",
		"boards.delete",
	} {
		id := "oputil:" + cmd
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing registry entry for oputil command %s", cmd)
		}
	}

	// Every door manifest must be present in the registry.
	manifestPaths, err := filepath.Glob(filepath.Join(root, "doors", "*", "door.json"))
	if err != nil {
		t.Fatalf("glob doors manifests: %v", err)
	}
	sort.Strings(manifestPaths)
	for _, path := range manifestPaths {
		doorID := extractDoorID(t, path)
		id := "door:" + doorID
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing registry entry for door %s", doorID)
		}
	}

	// Coverage matrix must exist as human-readable companion artifact.
	matrixPath := filepath.Join(root, "docs", "UI_COVERAGE_MATRIX.md")
	matrix := mustRead(t, matrixPath)
	if !strings.Contains(matrix, "| ID | Type |") {
		t.Fatalf("coverage matrix table header missing in %s", matrixPath)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	t.Fatalf("failed to locate repository root from %s", file)
	return ""
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func extractDoorID(t *testing.T, path string) string {
	t.Helper()
	type doorManifest struct {
		ID string `json:"id"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var manifest doorManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	id := strings.TrimSpace(manifest.ID)
	if id == "" {
		t.Fatalf("manifest missing id: %s", path)
	}
	return id
}
