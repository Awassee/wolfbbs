package doors

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestDoorManifestsLoadCatalog(t *testing.T) {
	reg := NewRegistry()
	reg.SetRepository(repository.NewInMemoryDoorRepository())
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	required := []string{
		"space-trader-wars",
		"dragon-tavern-legends",
		"barren-realms-commander",
		"solar-realms-dominion",
		"yankee-trader-syndicate",
		"assassins-guild",
		"arrowbridge-quest",
		"exitilus-realms",
		"falcon-relic-wars",
		"global-war-command",
		"pit-arena",
		"overkill-ops",
		"siege-engines",
		"fishing-derby",
		"word-duel-arena",
		"casino-royale-suite",
		"door-hub",
		"doorparty-connector",
		"bbslink-connector",
		"telnet-bridge",
		"voting-booth",
		"bulletin-news-center",
		"filebase-pro",
		"ansi-art-gallery",
		"tournaments-achievements-center",
	}

	for _, id := range required {
		door, ok := reg.DoorByID(id)
		if !ok {
			t.Fatalf("required door missing: %s", id)
		}
		if door.Hotkey == "" {
			t.Fatalf("required door hotkey missing: %s", id)
		}
		cfg, err := reg.GetDoorConfig(id)
		if err != nil {
			t.Fatalf("door config missing for %s: %v", id, err)
		}
		if cfg == nil || !cfg.Enabled {
			t.Fatalf("door should be enabled by default: %s", id)
		}
	}

	if got := len(reg.Doors()); got < len(required) {
		t.Fatalf("expected at least %d doors, got %d", len(required), got)
	}
}

func TestDoorTurnsAndTimeBankRules(t *testing.T) {
	repo := repository.NewInMemoryDoorRepository()
	reg := NewRegistry()
	reg.SetRepository(repo)
	reg.Register(Door{
		ID:              "turn-bank-check",
		Name:            "Turn Bank Check",
		Description:     "Turns and bank test door",
		Category:        "utility",
		Type:            "native",
		Command:         "native:turn-bank-check",
		Hotkey:          "J",
		ConcurrencyMode: "per_user",
		EnabledDefault:  true,
		DailyTurns:      2,
		TimeBankMax:     3,
		MaxRunSec:       30,
		MaxOutputRate:   4096,
	})

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	if err := repo.UpsertTurn(&domain.DoorTurnLedger{
		UserID:    41,
		DoorID:    "turn-bank-check",
		DayKey:    yesterday,
		TurnsUsed: 1,
		TimeBank:  0,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed previous day turn: %v", err)
	}

	remaining, err := reg.TurnsRemaining(41, "turn-bank-check", today)
	if err != nil {
		t.Fatalf("turns remaining: %v", err)
	}
	if remaining != 3 {
		t.Fatalf("expected 3 turns (2 daily + 1 carry), got %d", remaining)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "41",
		"WOLFBBS_HANDLE":    "turntester",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	for i := 0; i < 3; i++ {
		in := bytes.NewBufferString("Q")
		out := bytes.Buffer{}
		if err := reg.StartDoorByID(context.Background(), "turn-bank-check", in, &out, &out, env); err != nil {
			t.Fatalf("launch %d failed: %v", i+1, err)
		}
	}
	in := bytes.NewBufferString("Q")
	out := bytes.Buffer{}
	err = reg.StartDoorByID(context.Background(), "turn-bank-check", in, &out, &out, env)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no turns remaining") {
		t.Fatalf("expected no turns remaining error, got: %v", err)
	}
}

func TestNativeDoorLaunchesForCatalog(t *testing.T) {
	reg := NewRegistry()
	reg.SetRepository(repository.NewInMemoryDoorRepository())
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "7",
		"WOLFBBS_HANDLE":    "smoketest",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	for _, door := range reg.Doors() {
		if strings.ToLower(strings.TrimSpace(door.Type)) != "native" {
			continue
		}
		cfg, _ := reg.GetDoorConfig(door.ID)
		if cfg != nil && !cfg.Enabled {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		in := bytes.NewBufferString("Q")
		out := bytes.Buffer{}
		err := reg.StartDoorByID(ctx, door.ID, in, &out, &out, env)
		cancel()
		if err != nil {
			t.Fatalf("native door launch failed for %s: %v", door.ID, err)
		}
		rendered := out.String()
		if !strings.Contains(rendered, door.Name) {
			t.Fatalf("expected title output for %s", door.ID)
		}
	}
}

func TestExternalDoorLaunchSmoke(t *testing.T) {
	tmp := t.TempDir()
	doorCmd := filepath.Join(tmp, "external-door.sh")
	script := "#!/usr/bin/env bash\nset -euo pipefail\necho 'External Door Smoke Title'\n"
	if err := os.WriteFile(doorCmd, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("WOLFBBS_DOOR_ALLOW_DIR", tmp)

	reg := NewRegistry()
	reg.SetRepository(repository.NewInMemoryDoorRepository())
	reg.Register(Door{
		ID:              "external-smoke",
		Name:            "External Smoke",
		Description:     "External launch test",
		Category:        "utility",
		Type:            "external",
		Command:         doorCmd,
		Hotkey:          "E",
		ConcurrencyMode: "shared",
		EnabledDefault:  true,
		DailyTurns:      0,
		TimeBankMax:     0,
		MaxRunSec:       10,
		MaxOutputRate:   4096,
	})

	env := map[string]string{
		"WOLFBBS_USER_ID": "9",
		"WOLFBBS_HANDLE":  "external",
		"WOLFBBS_ROLE":    "user",
	}
	out := bytes.Buffer{}
	if err := reg.StartDoorByID(context.Background(), "external-smoke", bytes.NewBuffer(nil), &out, &out, env); err != nil {
		t.Fatalf("external launch failed: %v", err)
	}
	if !strings.Contains(out.String(), "External Door Smoke Title") {
		t.Fatal("expected external door title output")
	}
}

func TestExternalDoorWritesConfiguredDropfiles(t *testing.T) {
	tmp := t.TempDir()
	doorCmd := filepath.Join(tmp, "external-dropfiles.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
test -f DOOR32.SYS
test -f DOOR.SYS
test -f DORINFO1.DEF
grep -q "dropuser" DOOR32.SYS
echo "dropfiles-ready"
`
	if err := os.WriteFile(doorCmd, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	t.Setenv("WOLFBBS_DOOR_ALLOW_DIR", tmp)
	t.Setenv("WOLFBBS_DOOR_DATA_ROOT", tmp)
	t.Setenv("WOLFBBS_DOOR_DROPFILES", "door32,doorsys,dorinfo")

	reg := NewRegistry()
	reg.SetRepository(repository.NewInMemoryDoorRepository())
	reg.Register(Door{
		ID:              "external-dropfiles",
		Name:            "External Dropfiles",
		Description:     "Dropfile launch test",
		Category:        "utility",
		Type:            "external",
		Command:         doorCmd,
		Hotkey:          "F",
		ConcurrencyMode: "shared",
		EnabledDefault:  true,
		MaxRunSec:       10,
		MaxOutputRate:   4096,
	})

	env := map[string]string{
		"WOLFBBS_USER_ID": "9",
		"WOLFBBS_HANDLE":  "dropuser",
		"WOLFBBS_ROLE":    "user",
		"WOLFBBS_NODE":    "1",
	}
	out := bytes.Buffer{}
	if err := reg.StartDoorByID(context.Background(), "external-dropfiles", bytes.NewBuffer(nil), &out, &out, env); err != nil {
		t.Fatalf("external dropfile launch failed: %v", err)
	}
	if !strings.Contains(out.String(), "dropfiles-ready") {
		t.Fatalf("expected dropfile output, got: %s", out.String())
	}
	dataDir := filepath.Join(tmp, "external-dropfiles", "9")
	for _, name := range []string{"DOOR32.SYS", "DOOR.SYS", "DORINFO1.DEF"} {
		if _, err := os.Stat(filepath.Join(dataDir, name)); err != nil {
			t.Fatalf("expected %s in data dir: %v", name, err)
		}
	}
}

func TestConnectorDoorsLaunchConfiguredCommands(t *testing.T) {
	tmp := t.TempDir()
	doorPartyCmd := filepath.Join(tmp, "doorparty.sh")
	bbsLinkCmd := filepath.Join(tmp, "bbslink.sh")
	telnetBridgeCmd := filepath.Join(tmp, "telnet-bridge.sh")
	if err := os.WriteFile(doorPartyCmd, []byte("#!/usr/bin/env bash\nset -euo pipefail\necho 'doorparty-bridge-ok'\n"), 0o755); err != nil {
		t.Fatalf("write doorparty connector script: %v", err)
	}
	if err := os.WriteFile(bbsLinkCmd, []byte("#!/usr/bin/env bash\nset -euo pipefail\necho 'bbslink-bridge-ok'\n"), 0o755); err != nil {
		t.Fatalf("write bbslink connector script: %v", err)
	}
	if err := os.WriteFile(telnetBridgeCmd, []byte("#!/usr/bin/env bash\nset -euo pipefail\necho 'telnet-bridge-ok'\n"), 0o755); err != nil {
		t.Fatalf("write telnet bridge connector script: %v", err)
	}
	t.Setenv("WOLFBBS_DOOR_ALLOW_DIR", tmp)
	t.Setenv("WOLFBBS_DOOR_DATA_ROOT", tmp)
	t.Setenv("WOLFBBS_DOORPARTY_ENABLE", "1")
	t.Setenv("WOLFBBS_DOORPARTY_COMMAND", doorPartyCmd)
	t.Setenv("WOLFBBS_BBSLINK_ENABLE", "1")
	t.Setenv("WOLFBBS_BBSLINK_COMMAND", bbsLinkCmd)
	t.Setenv("WOLFBBS_TELNET_BRIDGE_ENABLE", "1")
	t.Setenv("WOLFBBS_TELNET_BRIDGE_COMMAND", telnetBridgeCmd)

	reg := NewRegistry()
	reg.SetRepository(repository.NewInMemoryDoorRepository())
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "55",
		"WOLFBBS_HANDLE":    "connectortest",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}

	tests := []struct {
		doorID   string
		expected string
	}{
		{doorID: "doorparty-connector", expected: "doorparty-bridge-ok"},
		{doorID: "bbslink-connector", expected: "bbslink-bridge-ok"},
		{doorID: "telnet-bridge", expected: "telnet-bridge-ok"},
	}
	for _, tc := range tests {
		out := bytes.Buffer{}
		if err := reg.StartDoorByID(context.Background(), tc.doorID, bytes.NewBufferString("CQQ"), &out, &out, env); err != nil {
			t.Fatalf("connector launch failed for %s: %v", tc.doorID, err)
		}
		rendered := out.String()
		if !strings.Contains(rendered, tc.expected) {
			t.Fatalf("expected connector output %q for %s, got: %s", tc.expected, tc.doorID, rendered)
		}
	}
}

func TestSpaceTraderWarsFlowPersistsState(t *testing.T) {
	reg := NewRegistry()
	repo := repository.NewInMemoryDoorRepository()
	reg.SetRepository(repo)
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "55",
		"WOLFBBS_HANDLE":    "captain",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	out := bytes.Buffer{}
	in := bytes.NewBufferString("TBO2\nXQ")
	if err := reg.StartDoorByID(context.Background(), "space-trader-wars", in, &out, &out, env); err != nil {
		t.Fatalf("space trader launch failed: %v", err)
	}

	row, err := repo.GetUserState(55, "space-trader-wars")
	if err != nil {
		t.Fatalf("expected persisted user state: %v", err)
	}
	var state spaceTraderState
	if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Trades < 1 {
		t.Fatalf("expected trade count to increase, got %+v", state)
	}
}

func TestDragonTavernLegendsFlowPersistsState(t *testing.T) {
	reg := NewRegistry()
	repo := repository.NewInMemoryDoorRepository()
	reg.SetRepository(repo)
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "66",
		"WOLFBBS_HANDLE":    "hero",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	out := bytes.Buffer{}
	in := bytes.NewBufferString("FXQ")
	if err := reg.StartDoorByID(context.Background(), "dragon-tavern-legends", in, &out, &out, env); err != nil {
		t.Fatalf("dragon tavern launch failed: %v", err)
	}

	row, err := repo.GetUserState(66, "dragon-tavern-legends")
	if err != nil {
		t.Fatalf("expected persisted user state: %v", err)
	}
	var state dragonTavernState
	if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.ForestRuns < 1 {
		t.Fatalf("expected forest run count to increase, got %+v", state)
	}
}

func TestVotingBoothCreateAndVotePersists(t *testing.T) {
	reg := NewRegistry()
	repo := repository.NewInMemoryDoorRepository()
	reg.SetRepository(repo)
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "77",
		"WOLFBBS_HANDLE":    "moduser",
		"WOLFBBS_ROLE":      "moderator",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	out := bytes.Buffer{}
	in := bytes.NewBufferString("CFavorite color?\nBlue,Green\nXV1\n1\nXQ")
	if err := reg.StartDoorByID(context.Background(), "voting-booth", in, &out, &out, env); err != nil {
		t.Fatalf("voting booth launch failed: %v", err)
	}

	global, err := repo.GetGlobalState("voting-booth")
	if err != nil {
		t.Fatalf("expected voting global state: %v", err)
	}
	var polls votingGlobalState
	if err := json.Unmarshal([]byte(global.StateJSON), &polls); err != nil {
		t.Fatalf("decode global state: %v", err)
	}
	if len(polls.Polls) != 1 {
		t.Fatalf("expected one poll, got %+v", polls)
	}
	if len(polls.Polls[0].Counts) == 0 || polls.Polls[0].Counts[0] != 1 {
		t.Fatalf("expected poll vote count on first option, got %+v", polls.Polls[0].Counts)
	}
}

func TestFileBaseProAddAndDownloadPersists(t *testing.T) {
	reg := NewRegistry()
	repo := repository.NewInMemoryDoorRepository()
	reg.SetRepository(repo)
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "88",
		"WOLFBBS_HANDLE":    "archivist",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	out := bytes.Buffer{}
	in := bytes.NewBufferString("Amain\nreadme.txt\nSample file\ndocs,ansi\nfingerprint\nXD1\nXQ")
	if err := reg.StartDoorByID(context.Background(), "filebase-pro", in, &out, &out, env); err != nil {
		t.Fatalf("filebase launch failed: %v", err)
	}

	global, err := repo.GetGlobalState("filebase-pro")
	if err != nil {
		t.Fatalf("expected filebase global state: %v", err)
	}
	var files filebaseGlobalState
	if err := json.Unmarshal([]byte(global.StateJSON), &files); err != nil {
		t.Fatalf("decode global state: %v", err)
	}
	if len(files.Files) != 1 {
		t.Fatalf("expected one indexed file, got %+v", files)
	}
	if files.Files[0].Downloads != 1 {
		t.Fatalf("expected download count 1, got %d", files.Files[0].Downloads)
	}
}

func TestOracleDoorLabelsResponses(t *testing.T) {
	reg := NewRegistry()
	repo := repository.NewInMemoryDoorRepository()
	reg.SetRepository(repo)
	if err := reg.LoadManifestDir(filepath.Join("..", "..", "doors")); err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	if err := reg.SetDoorConfig(&domain.DoorConfig{
		DoorID:         "oracle-door",
		Enabled:        true,
		DailyTurns:     10,
		TimeBankMax:    0,
		ResetHourLocal: 0,
		MessagesDays:   30,
		LogsDays:       30,
		MaxRunSeconds:  120,
		MaxOutputRate:  4096,
		AllowNetwork:   false,
		AllowFSWrite:   false,
	}); err != nil {
		t.Fatalf("set door config: %v", err)
	}

	env := map[string]string{
		"WOLFBBS_USER_ID":   "91",
		"WOLFBBS_HANDLE":    "oracleuser",
		"WOLFBBS_ROLE":      "user",
		"WOLFBBS_TERM_COLS": "80",
		"WOLFBBS_TERM_ROWS": "25",
		"WOLFBBS_ANSI":      "true",
	}
	out := bytes.Buffer{}
	in := bytes.NewBufferString("Awhere should i start?\nXQ")
	if err := reg.StartDoorByID(context.Background(), "oracle-door", in, &out, &out, env); err != nil {
		t.Fatalf("oracle door launch failed: %v", err)
	}
	if !strings.Contains(out.String(), "[AI-LABEL]") {
		t.Fatalf("expected AI label in oracle output, got: %s", out.String())
	}
}
