package doors

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wolfbbs/internal/config"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/term"
	"wolfbbs/internal/ui"
)

type doorTemplateProfile struct {
	Label         string
	PrimaryAction string
	PrimaryPast   string
	Secondary     string
}

type templateDoorState struct {
	Missions int64 `json:"missions"`
	Mastery  int64 `json:"mastery"`
	Credits  int64 `json:"credits"`
	Rank     int64 `json:"rank"`
}

type spaceTraderState struct {
	Sector   int            `json:"sector"`
	Credits  int64          `json:"credits"`
	CargoCap int            `json:"cargo_cap"`
	Hull     int            `json:"hull"`
	Weapon   int            `json:"weapon"`
	Cargo    map[string]int `json:"cargo"`
	Trades   int64          `json:"trades"`
	Wins     int64          `json:"wins"`
	Losses   int64          `json:"losses"`
}

type dragonTavernState struct {
	Level      int    `json:"level"`
	XP         int64  `json:"xp"`
	Gold       int64  `json:"gold"`
	HP         int    `json:"hp"`
	MaxHP      int    `json:"max_hp"`
	Attack     int    `json:"attack"`
	Defense    int    `json:"defense"`
	ForestRuns int64  `json:"forest_runs"`
	TavernRuns int64  `json:"tavern_runs"`
	DuelsWon   int64  `json:"duels_won"`
	DuelsLost  int64  `json:"duels_lost"`
	DuelDay    string `json:"duel_day"`
	DuelsUsed  int    `json:"duels_used"`
}

type votingPoll struct {
	ID        int      `json:"id"`
	Question  string   `json:"question"`
	Options   []string `json:"options"`
	Counts    []int64  `json:"counts"`
	Open      bool     `json:"open"`
	CreatedBy int64    `json:"created_by"`
	CreatedAt int64    `json:"created_at"`
}

type votingGlobalState struct {
	NextID int          `json:"next_id"`
	Polls  []votingPoll `json:"polls"`
}

type votingUserState struct {
	Votes map[string]int `json:"votes"`
}

type fileRecord struct {
	ID          int      `json:"id"`
	Area        string   `json:"area"`
	Filename    string   `json:"filename"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Uploader    int64    `json:"uploader"`
	SHA256      string   `json:"sha256"`
	UploadedAt  int64    `json:"uploaded_at"`
	Downloads   int64    `json:"downloads"`
}

type filebaseGlobalState struct {
	NextID int          `json:"next_id"`
	Files  []fileRecord `json:"files"`
}

type filebaseUserState struct {
	LastScanUnix int64 `json:"last_scan_unix"`
	Uploads      int64 `json:"uploads"`
	Downloads    int64 `json:"downloads"`
}

type oracleDoorState struct {
	Day     string `json:"day"`
	Prompts int    `json:"prompts"`
}

var templateDoorProfiles = map[string]doorTemplateProfile{
	"barren-realms-commander":         {Label: "Realm Command", PrimaryAction: "Deploy", PrimaryPast: "deployed", Secondary: "Fortify"},
	"solar-realms-dominion":           {Label: "Solar Dominion", PrimaryAction: "Colonize", PrimaryPast: "colonized", Secondary: "Refit"},
	"yankee-trader-syndicate":         {Label: "Trader Syndicate", PrimaryAction: "Contract", PrimaryPast: "completed", Secondary: "Reinvest"},
	"assassins-guild":                 {Label: "Guild Ops", PrimaryAction: "Mission", PrimaryPast: "completed", Secondary: "Sharpen"},
	"arrowbridge-quest":               {Label: "Quest Routes", PrimaryAction: "Explore", PrimaryPast: "explored", Secondary: "Camp"},
	"exitilus-realms":                 {Label: "Kingdom War", PrimaryAction: "Raid", PrimaryPast: "raided", Secondary: "Rebuild"},
	"falcon-relic-wars":               {Label: "Relic Hunt", PrimaryAction: "Contest", PrimaryPast: "contested", Secondary: "Study"},
	"global-war-command":              {Label: "Global War", PrimaryAction: "Advance", PrimaryPast: "advanced", Secondary: "Entrench"},
	"pit-arena":                       {Label: "Arena Circuit", PrimaryAction: "Fight", PrimaryPast: "fought", Secondary: "Train"},
	"overkill-ops":                    {Label: "Ops Board", PrimaryAction: "Sortie", PrimaryPast: "finished", Secondary: "Tune"},
	"siege-engines":                   {Label: "Siege Range", PrimaryAction: "Duel", PrimaryPast: "dueled", Secondary: "Calibrate"},
	"fishing-derby":                   {Label: "Derby Waters", PrimaryAction: "Trip", PrimaryPast: "fished", Secondary: "Bait"},
	"word-duel-arena":                 {Label: "Word Arena", PrimaryAction: "Round", PrimaryPast: "played", Secondary: "Practice"},
	"casino-royale-suite":             {Label: "Casino Floor", PrimaryAction: "Session", PrimaryPast: "played", Secondary: "Bankroll"},
	"door-hub":                        {Label: "Door Hub", PrimaryAction: "Browse", PrimaryPast: "browsed", Secondary: "Pin"},
	"doorparty-connector":             {Label: "DoorParty", PrimaryAction: "Bridge", PrimaryPast: "bridged", Secondary: "Sync"},
	"bbslink-connector":               {Label: "BBSLink", PrimaryAction: "Relay", PrimaryPast: "relayed", Secondary: "Refresh"},
	"telnet-bridge":                   {Label: "Telnet Bridge", PrimaryAction: "Dial", PrimaryPast: "dialed", Secondary: "Bookmark"},
	"bulletin-news-center":            {Label: "News Center", PrimaryAction: "Digest", PrimaryPast: "read", Secondary: "Archive"},
	"ansi-art-gallery":                {Label: "ANSI Gallery", PrimaryAction: "Exhibit", PrimaryPast: "viewed", Secondary: "Feature"},
	"tournaments-achievements-center": {Label: "Trophy Hall", PrimaryAction: "Bracket", PrimaryPast: "entered", Secondary: "Review"},
}

func (r *Registry) runAdvancedNativeDoor(ctx context.Context, door Door, cfg domain.DoorConfig, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) (bool, error) {
	switch door.ID {
	case "space-trader-wars":
		return true, r.runSpaceTraderWars(ctx, door, cfg, dctx, reader, stdout)
	case "dragon-tavern-legends":
		return true, r.runDragonTavernLegends(ctx, door, cfg, dctx, reader, stdout)
	case "voting-booth":
		return true, r.runVotingBooth(ctx, door, dctx, reader, stdout)
	case "filebase-pro":
		return true, r.runFileBasePro(ctx, door, dctx, reader, stdout)
	case "oracle-door":
		return true, r.runOracleDoor(ctx, door, dctx, reader, stdout)
	case "ansi-art-gallery":
		return true, r.runANSIArtGallery(ctx, door, dctx, reader, stdout)
	case "doorparty-connector":
		return true, r.runConnectorDoor(ctx, door, cfg, dctx, reader, stdout, "doorparty")
	case "bbslink-connector":
		return true, r.runConnectorDoor(ctx, door, cfg, dctx, reader, stdout, "bbslink")
	case "telnet-bridge":
		return true, r.runConnectorDoor(ctx, door, cfg, dctx, reader, stdout, "telnet_bridge")
	default:
		if profile, ok := templateDoorProfiles[door.ID]; ok {
			return true, r.runTemplatePatternDoor(ctx, door, dctx, profile, reader, stdout)
		}
		return false, nil
	}
}

type connectorProgram struct {
	Name    string
	Enabled bool
	Command string
	Args    []string
}

func (r *Registry) runConnectorDoor(ctx context.Context, door Door, cfg domain.DoorConfig, dctx DoorContext, reader *bufio.Reader, stdout io.Writer, kind string) error {
	program := loadConnectorProgram(kind)
	for {
		networkStatus := "allowed"
		if !cfg.AllowNetwork {
			networkStatus = "blocked by door policy"
		}
		commandStatus := "not configured"
		if strings.TrimSpace(program.Command) != "" {
			commandStatus = program.Command
		}
		enableStatus := "disabled"
		if program.Enabled {
			enableStatus = "enabled"
		}
		lines := []string{
			fmt.Sprintf("%s adapter status: %s", program.Name, enableStatus),
			fmt.Sprintf("Command: %s", commandStatus),
			fmt.Sprintf("Policy network: %s", networkStatus),
			fmt.Sprintf("Node:%s Session:%s User:%s", fallbackName(dctx.NodeID, "-"), fallbackName(dctx.SessionID, "-"), fallbackName(dctx.Username, "-")),
			"",
			"(C)onnect  (S)tatus  (H)elp  (R)ules  (Q)uit",
			"",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgMagenta)
		key, err := readDoorKey(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door))
			pauseDoor(reader, stdout)
		case "S":
			r.writeTopScores(stdout, door.ID, "points", "Connector Sessions")
			pauseDoor(reader, stdout)
		case "C":
			program = loadConnectorProgram(kind)
			if !program.Enabled {
				_ = r.repo.AddEvent(&domain.DoorEvent{
					DoorID:      door.ID,
					UserID:      dctx.UserID,
					EventType:   "connector_denied",
					PayloadJSON: fmt.Sprintf(`{"reason":"disabled","connector":%q}`, kind),
				})
				io.WriteString(stdout, "\r\nConnector disabled by sysop policy.")
				pauseDoor(reader, stdout)
				break
			}
			if !cfg.AllowNetwork {
				_ = r.repo.AddEvent(&domain.DoorEvent{
					DoorID:      door.ID,
					UserID:      dctx.UserID,
					EventType:   "connector_denied",
					PayloadJSON: fmt.Sprintf(`{"reason":"network_blocked","connector":%q}`, kind),
				})
				io.WriteString(stdout, "\r\nThis door currently blocks network access.")
				pauseDoor(reader, stdout)
				break
			}
			if strings.TrimSpace(program.Command) == "" {
				_ = r.repo.AddEvent(&domain.DoorEvent{
					DoorID:      door.ID,
					UserID:      dctx.UserID,
					EventType:   "connector_denied",
					PayloadJSON: fmt.Sprintf(`{"reason":"command_missing","connector":%q}`, kind),
				})
				io.WriteString(stdout, "\r\nConnector command is not configured.")
				pauseDoor(reader, stdout)
				break
			}
			_ = r.repo.AddEvent(&domain.DoorEvent{
				DoorID:      door.ID,
				UserID:      dctx.UserID,
				EventType:   "connector_launch",
				PayloadJSON: fmt.Sprintf(`{"connector":%q,"command":%q}`, kind, program.Command),
			})
			err := r.execConnectorProgram(ctx, door, cfg, dctx, reader, stdout, program, kind)
			if err != nil {
				_ = r.repo.AddEvent(&domain.DoorEvent{
					DoorID:      door.ID,
					UserID:      dctx.UserID,
					EventType:   "connector_error",
					PayloadJSON: fmt.Sprintf(`{"connector":%q,"error":%q}`, kind, truncateJSONValue(err.Error(), 280)),
				})
				io.WriteString(stdout, "\r\nConnector failed: "+err.Error())
				pauseDoor(reader, stdout)
				break
			}
			_ = r.repo.AddEvent(&domain.DoorEvent{
				DoorID:      door.ID,
				UserID:      dctx.UserID,
				EventType:   "connector_stop",
				PayloadJSON: fmt.Sprintf(`{"connector":%q}`, kind),
			})
			io.WriteString(stdout, "\r\nRemote session returned cleanly.")
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) execConnectorProgram(ctx context.Context, door Door, cfg domain.DoorConfig, dctx DoorContext, stdin io.Reader, stdout io.Writer, program connectorProgram, kind string) error {
	commandPath, err := resolveDoorCommand(program.Command)
	if err != nil {
		return err
	}
	if err := validateDoorPath(commandPath); err != nil {
		return err
	}
	dataDir := perDoorDataDir(door.ID, dctx.UserID)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, commandPath, append([]string{}, program.Args...)...)
	cmd.Stdin = stdin
	cmd.Stdout = newRateWriter(stdout, cfg.MaxOutputRate)
	cmd.Stderr = newRateWriter(stdout, cfg.MaxOutputRate)
	cmd.Dir = dataDir
	cmd.Env = append(os.Environ(),
		"WOLFBBS_MODE=connector",
		"WOLFBBS_CONNECTOR="+kind,
		"WOLFBBS_DOOR_ID="+door.ID,
		"WOLFBBS_DOOR_DATA_DIR="+dataDir,
		"WOLFBBS_USER_ID="+strconv.FormatInt(dctx.UserID, 10),
		"WOLFBBS_HANDLE="+dctx.Username,
		"WOLFBBS_ROLE="+rbac.NormalizeRole(dctx.Role),
		"WOLFBBS_NODE="+dctx.NodeID,
		"WOLFBBS_SESSION_ID="+dctx.SessionID,
		"WOLFBBS_TERM_COLS="+strconv.Itoa(dctx.Cols),
		"WOLFBBS_TERM_ROWS="+strconv.Itoa(dctx.Rows),
		"WOLFBBS_ANSI="+boolText(dctx.ANSI),
		"WOLFBBS_ENCODING="+fallbackName(dctx.Encoding, "utf-8"),
		"WOLFBBS_TZ="+fallbackName(dctx.Timezone, "UTC"),
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s command failed: %w", program.Name, err)
	}
	return nil
}

func loadConnectorProgram(kind string) connectorProgram {
	kind = strings.ToLower(strings.TrimSpace(kind))
	program := connectorProgram{
		Name: strings.ToUpper(kind),
	}
	cfg, err := config.LoadRuntimeFromEnv()
	if err == nil {
		switch kind {
		case "doorparty":
			program = connectorProgram{
				Name:    "DoorParty",
				Enabled: cfg.Connectors.DoorParty.Enabled,
				Command: strings.TrimSpace(cfg.Connectors.DoorParty.Command),
				Args:    parseConnectorArgs(cfg.Connectors.DoorParty.Args),
			}
		case "bbslink":
			program = connectorProgram{
				Name:    "BBSLink",
				Enabled: cfg.Connectors.BBSLink.Enabled,
				Command: strings.TrimSpace(cfg.Connectors.BBSLink.Command),
				Args:    parseConnectorArgs(cfg.Connectors.BBSLink.Args),
			}
		case "telnet_bridge", "telnet-bridge":
			program = connectorProgram{
				Name:    "Telnet Bridge",
				Enabled: cfg.Connectors.Telnet.Enabled,
				Command: strings.TrimSpace(cfg.Connectors.Telnet.Command),
				Args:    parseConnectorArgs(cfg.Connectors.Telnet.Args),
			}
		}
	}

	enabledVar := ""
	commandVar := ""
	argsVar := ""
	switch kind {
	case "doorparty":
		enabledVar = "WOLFBBS_DOORPARTY_ENABLE"
		commandVar = "WOLFBBS_DOORPARTY_COMMAND"
		argsVar = "WOLFBBS_DOORPARTY_ARGS"
		if program.Name == "" {
			program.Name = "DoorParty"
		}
	case "bbslink":
		enabledVar = "WOLFBBS_BBSLINK_ENABLE"
		commandVar = "WOLFBBS_BBSLINK_COMMAND"
		argsVar = "WOLFBBS_BBSLINK_ARGS"
		if program.Name == "" {
			program.Name = "BBSLink"
		}
	case "telnet_bridge", "telnet-bridge":
		enabledVar = "WOLFBBS_TELNET_BRIDGE_ENABLE"
		commandVar = "WOLFBBS_TELNET_BRIDGE_COMMAND"
		argsVar = "WOLFBBS_TELNET_BRIDGE_ARGS"
		if program.Name == "" {
			program.Name = "Telnet Bridge"
		}
	}
	if raw, ok := lookupEnvBool(enabledVar); ok {
		program.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv(commandVar)); raw != "" {
		program.Command = raw
	}
	if raw := strings.TrimSpace(os.Getenv(argsVar)); raw != "" {
		program.Args = parseConnectorArgs(raw)
	}
	return program
}

func parseConnectorArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func lookupEnvBool(key string) (bool, bool) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func (r *Registry) runTemplatePatternDoor(ctx context.Context, door Door, dctx DoorContext, profile doorTemplateProfile, reader *bufio.Reader, stdout io.Writer) error {
	state, err := loadUserStateJSON(r.repo, dctx.UserID, door.ID, templateDoorState{
		Credits: 120,
		Rank:    1,
	})
	if err != nil {
		return err
	}
	for {
		lines := []string{
			fmt.Sprintf("%s node online. %s with classic quick commands.", profile.Label, door.Name),
			fmt.Sprintf("Missions:%d  Rank:%d  Mastery:%d  Credits:%d", state.Missions, state.Rank, state.Mastery, state.Credits),
			"",
			fmt.Sprintf("(%s) %s  (%s) %s  (S)tatus  (L)eaderboard", string(profile.PrimaryAction[0]), profile.PrimaryAction, string(profile.Secondary[0]), profile.Secondary),
			"(H)elp  (R)ules  (Q)uit",
			"",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgYellow)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		changed := false
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door))
			pauseDoor(reader, stdout)
		case "S":
			io.WriteString(stdout, fmt.Sprintf("\r\nRank %d | Mastery %d | Credits %d | Runs %d\r\n", state.Rank, state.Mastery, state.Credits, state.Missions))
			pauseDoor(reader, stdout)
		case "L":
			r.writeTopScores(stdout, door.ID, "points", "Top Operators")
			pauseDoor(reader, stdout)
		case strings.ToUpper(string(profile.PrimaryAction[0])):
			seed := int64((state.Missions+1)*17 + state.Mastery*13 + dctx.UserID + int64(len(door.ID)))
			points := 40 + (seed % 90)
			creditGain := points + (state.Rank * 8)
			state.Missions++
			state.Credits += creditGain
			if state.Missions%4 == 0 {
				state.Rank++
			}
			if state.Missions == 1 {
				_ = r.AwardAchievement(door.ID, dctx.UserID, "first_run")
			}
			if state.Rank >= 5 {
				_ = r.AwardAchievement(door.ID, dctx.UserID, "rank_5")
			}
			io.WriteString(stdout, fmt.Sprintf("\r\n%s %s. +%d credits\r\n", profile.PrimaryAction, profile.PrimaryPast, creditGain))
			changed = true
			pauseDoor(reader, stdout)
		case strings.ToUpper(string(profile.Secondary[0])):
			cost := int64(35 + (state.Mastery * 6))
			if state.Credits < cost {
				io.WriteString(stdout, fmt.Sprintf("\r\nNeed %d credits to %s.\r\n", cost, strings.ToLower(profile.Secondary)))
				pauseDoor(reader, stdout)
				break
			}
			state.Credits -= cost
			state.Mastery++
			if state.Mastery >= 8 {
				_ = r.AwardAchievement(door.ID, dctx.UserID, "master_8")
			}
			io.WriteString(stdout, fmt.Sprintf("\r\n%s complete. Mastery is now %d.\r\n", profile.Secondary, state.Mastery))
			changed = true
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		if changed {
			if err := saveUserStateJSON(r.repo, dctx.UserID, door.ID, state); err != nil {
				return err
			}
			_ = r.SubmitScore(door.ID, dctx.UserID, "points", templateDoorScoreValue(state), fmt.Sprintf(`{"missions":%d,"mastery":%d}`, state.Missions, state.Mastery))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) runSpaceTraderWars(ctx context.Context, door Door, _ domain.DoorConfig, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) error {
	state, err := loadUserStateJSON(r.repo, dctx.UserID, door.ID, defaultSpaceTraderState())
	if err != nil {
		return err
	}
	for {
		lines := []string{
			fmt.Sprintf("Captain: %s  Sector: %d", fallbackName(dctx.Username, "Pilot"), state.Sector),
			fmt.Sprintf("Credits:%d  Hull:%d  Weapon:%d  Cargo:%d/%d", state.Credits, state.Hull, state.Weapon, stateCargoUsed(state), state.CargoCap),
			fmt.Sprintf("Wins:%d Losses:%d Trades:%d", state.Wins, state.Losses, state.Trades),
			"",
			"(T)rade  (W)arp  (U)pgrade  (F)ight",
			"(S)tatus (L)eaderboard  (H)elp  (R)ules  (Q)uit",
			"",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgCyan)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		changed := false
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door))
			pauseDoor(reader, stdout)
		case "S":
			r.writeSpaceTraderHoldings(stdout, state)
			pauseDoor(reader, stdout)
		case "L":
			r.writeTopScores(stdout, door.ID, "points", "Space Trader Net Worth")
			pauseDoor(reader, stdout)
		case "T":
			mutated, tradeMsg, err := r.spaceTraderTrade(reader, stdout, &state)
			if err != nil {
				return err
			}
			if tradeMsg != "" {
				io.WriteString(stdout, tradeMsg)
			}
			if mutated {
				state.Trades++
				changed = true
				if state.Trades == 1 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "first_trade")
				}
			}
			pauseDoor(reader, stdout)
		case "W":
			mutated, msg, err := r.spaceTraderWarp(reader, stdout, &state)
			if err != nil {
				return err
			}
			if msg != "" {
				io.WriteString(stdout, msg)
			}
			if mutated {
				changed = true
				if state.Sector >= 20 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "deep_space")
				}
			}
			pauseDoor(reader, stdout)
		case "U":
			mutated, msg, err := r.spaceTraderUpgrade(reader, stdout, &state)
			if err != nil {
				return err
			}
			if msg != "" {
				io.WriteString(stdout, msg)
			}
			if mutated {
				changed = true
			}
			pauseDoor(reader, stdout)
		case "F":
			mutated, msg := r.spaceTraderFight(&state)
			if msg != "" {
				io.WriteString(stdout, msg)
			}
			if mutated {
				changed = true
				if state.Wins == 1 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "first_win")
				}
				if state.Wins >= 10 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "ace_pilot")
				}
			}
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		if changed {
			if err := saveUserStateJSON(r.repo, dctx.UserID, door.ID, state); err != nil {
				return err
			}
			_ = r.SubmitScore(door.ID, dctx.UserID, "points", spaceTraderScoreValue(state), fmt.Sprintf(`{"wins":%d,"trades":%d,"sector":%d}`, state.Wins, state.Trades, state.Sector))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) runDragonTavernLegends(ctx context.Context, door Door, _ domain.DoorConfig, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) error {
	state, err := loadUserStateJSON(r.repo, dctx.UserID, door.ID, defaultDragonTavernState())
	if err != nil {
		return err
	}
	for {
		resetDragonDuelWindow(&state, time.Now().UTC())
		lines := []string{
			fmt.Sprintf("Hero: %s  Level:%d  XP:%d", fallbackName(dctx.Username, "Adventurer"), state.Level, state.XP),
			fmt.Sprintf("HP:%d/%d  ATK:%d  DEF:%d  Gold:%d", state.HP, state.MaxHP, state.Attack, state.Defense, state.Gold),
			fmt.Sprintf("Forest:%d  Tavern:%d  Duels:%d/%d", state.ForestRuns, state.TavernRuns, state.DuelsUsed, 3),
			"",
			"(F)orest  (T)avern  (S)mithy  (D)uel",
			"(C)haracter  (L)eaderboard  (H)elp  (R)ules  (Q)uit",
			"",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgMagenta)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		changed := false
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door))
			pauseDoor(reader, stdout)
		case "L":
			r.writeTopScores(stdout, door.ID, "points", "Dragon Tavern Champions")
			pauseDoor(reader, stdout)
		case "C":
			io.WriteString(stdout, fmt.Sprintf("\r\nLevel %d  XP %d  Gold %d\r\nHP %d/%d  ATK %d  DEF %d\r\nForest %d  Tavern %d  Duels W/L %d/%d\r\n", state.Level, state.XP, state.Gold, state.HP, state.MaxHP, state.Attack, state.Defense, state.ForestRuns, state.TavernRuns, state.DuelsWon, state.DuelsLost))
			pauseDoor(reader, stdout)
		case "F":
			mutated, msg := dragonForestRun(&state)
			io.WriteString(stdout, msg)
			if mutated {
				changed = true
				if state.ForestRuns == 1 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "first_quest")
				}
			}
			pauseDoor(reader, stdout)
		case "T":
			mutated, msg := dragonTavernRun(&state)
			io.WriteString(stdout, msg)
			if mutated {
				changed = true
				if state.TavernRuns >= 5 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "tavern_regular")
				}
			}
			pauseDoor(reader, stdout)
		case "S":
			mutated, msg, err := dragonSmithyRun(reader, stdout, &state)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			if mutated {
				changed = true
			}
			pauseDoor(reader, stdout)
		case "D":
			mutated, msg := dragonDuelRun(&state)
			io.WriteString(stdout, msg)
			if mutated {
				changed = true
				if state.DuelsWon >= 3 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "duelist_3")
				}
			}
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		if changed {
			dragonNormalizeStats(&state)
			if err := saveUserStateJSON(r.repo, dctx.UserID, door.ID, state); err != nil {
				return err
			}
			if state.Level >= 5 {
				_ = r.AwardAchievement(door.ID, dctx.UserID, "level_5")
			}
			_ = r.SubmitScore(door.ID, dctx.UserID, "points", dragonScoreValue(state), fmt.Sprintf(`{"level":%d,"duels_won":%d}`, state.Level, state.DuelsWon))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) runVotingBooth(ctx context.Context, door Door, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) error {
	global, err := loadGlobalStateJSON(r.repo, door.ID, votingGlobalState{NextID: 1})
	if err != nil {
		return err
	}
	userState, err := loadUserStateJSON(r.repo, dctx.UserID, door.ID, votingUserState{Votes: map[string]int{}})
	if err != nil {
		return err
	}
	if userState.Votes == nil {
		userState.Votes = map[string]int{}
	}
	for {
		totalPolls, openPolls := votingCounts(global)
		lines := []string{
			fmt.Sprintf("Polls:%d  Open:%d  Your Votes:%d", totalPolls, openPolls, len(userState.Votes)),
			"",
			"(L)ist polls  (V)ote  (R)esults",
			"(C)reate poll  (O)pen/Close poll  (H)elp  (Q)uit",
			"",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgGreen)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		changedGlobal := false
		changedUser := false
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "L":
			r.writePollList(stdout, global)
			pauseDoor(reader, stdout)
		case "R":
			r.writePollResults(stdout, global)
			pauseDoor(reader, stdout)
		case "C":
			if !isModOrAdmin(dctx.Role) {
				io.WriteString(stdout, "\r\nOnly moderators/admins can create polls from this door.\r\n")
				pauseDoor(reader, stdout)
				break
			}
			mutated, msg, err := r.votingCreatePoll(reader, stdout, dctx, &global)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			changedGlobal = mutated
			pauseDoor(reader, stdout)
		case "V":
			gChanged, uChanged, msg, err := r.votingCastVote(reader, stdout, dctx, &global, &userState)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			changedGlobal = gChanged
			changedUser = uChanged
			if uChanged {
				voteCount := int64(len(userState.Votes))
				_ = r.SubmitScore(door.ID, dctx.UserID, "points", voteCount, fmt.Sprintf(`{"votes":%d}`, voteCount))
				if voteCount == 1 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "first_vote")
				}
				if voteCount >= 5 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "civic_voice")
				}
			}
			pauseDoor(reader, stdout)
		case "O":
			if !isModOrAdmin(dctx.Role) {
				io.WriteString(stdout, "\r\nOnly moderators/admins can open or close polls.\r\n")
				pauseDoor(reader, stdout)
				break
			}
			mutated, msg, err := r.votingTogglePoll(reader, stdout, &global)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			changedGlobal = mutated
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		if changedGlobal {
			if err := saveGlobalStateJSON(r.repo, door.ID, global); err != nil {
				return err
			}
		}
		if changedUser {
			if err := saveUserStateJSON(r.repo, dctx.UserID, door.ID, userState); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) runFileBasePro(ctx context.Context, door Door, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) error {
	global, err := loadGlobalStateJSON(r.repo, door.ID, filebaseGlobalState{NextID: 1})
	if err != nil {
		return err
	}
	userState, err := loadUserStateJSON(r.repo, dctx.UserID, door.ID, filebaseUserState{})
	if err != nil {
		return err
	}
	for {
		lines := []string{
			fmt.Sprintf("Files:%d  Uploads:%d  Downloads:%d", len(global.Files), userState.Uploads, userState.Downloads),
			fmt.Sprintf("Last new-scan: %s", formatUnix(userState.LastScanUnix)),
			"",
			"(L)ist  (A)dd metadata  (S)earch  (N)ew scan  (D)ownload",
			"(H)elp  (R)ules  (Q)uit",
			"",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgBlue)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		changedGlobal := false
		changedUser := false
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door))
			pauseDoor(reader, stdout)
		case "L":
			writeFileList(stdout, global.Files, "File Catalog")
			pauseDoor(reader, stdout)
		case "S":
			msg, err := filebaseSearch(reader, stdout, global.Files)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			pauseDoor(reader, stdout)
		case "A":
			gChanged, uChanged, msg, err := filebaseAdd(reader, stdout, dctx, &global, &userState)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			changedGlobal = gChanged
			changedUser = uChanged
			if uChanged {
				if userState.Uploads == 1 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "first_upload")
				}
				if userState.Uploads >= 10 {
					_ = r.AwardAchievement(door.ID, dctx.UserID, "catalog_curator")
				}
			}
			pauseDoor(reader, stdout)
		case "N":
			msg := filebaseNewScan(&userState, global.Files)
			io.WriteString(stdout, msg)
			changedUser = true
			pauseDoor(reader, stdout)
		case "D":
			gChanged, uChanged, msg, err := filebaseDownload(reader, stdout, &global, &userState)
			if err != nil {
				return err
			}
			io.WriteString(stdout, msg)
			changedGlobal = gChanged
			changedUser = uChanged
			if userState.Downloads >= 10 {
				_ = r.AwardAchievement(door.ID, dctx.UserID, "downloader_10")
			}
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		if changedGlobal {
			if err := saveGlobalStateJSON(r.repo, door.ID, global); err != nil {
				return err
			}
		}
		if changedUser {
			if err := saveUserStateJSON(r.repo, dctx.UserID, door.ID, userState); err != nil {
				return err
			}
			_ = r.SubmitScore(door.ID, dctx.UserID, "points", filebaseScoreValue(userState), fmt.Sprintf(`{"uploads":%d,"downloads":%d}`, userState.Uploads, userState.Downloads))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) runOracleDoor(ctx context.Context, door Door, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) error {
	state, err := loadUserStateJSON(r.repo, dctx.UserID, door.ID, oracleDoorState{})
	if err != nil {
		return err
	}
	day := time.Now().UTC().Format("2006-01-02")
	if !dctx.Now.IsZero() {
		day = dctx.Now.UTC().Format("2006-01-02")
	}
	if state.Day != day {
		state.Day = day
		state.Prompts = 0
	}
	limit := oraclePromptLimit()

	for {
		lines := []string{
			"Contained AI door. Human-first board culture stays primary.",
			fmt.Sprintf("Prompts today: %d/%d", state.Prompts, limit),
			"",
			"(A)sk  (H)elp  (R)ules  (Q)uit",
			"",
			"Every response is labeled [AI-LABEL].",
			"Enter selection:",
		}
		renderDoorPanel(stdout, door.Name, lines, ui.FgCyan)
		key, err := readDoorKey(reader)
		if err != nil {
			return err
		}
		switch key {
		case "Q", "ESC":
			return nil
		case "H", "?":
			io.WriteString(stdout, "\r\n"+doorHelp(door))
			pauseDoor(reader, stdout)
		case "R":
			io.WriteString(stdout, "\r\n"+doorRules(door))
			pauseDoor(reader, stdout)
		case "A":
			if state.Prompts >= limit {
				io.WriteString(stdout, "\r\nDaily Oracle quota reached. Come back tomorrow.")
				pauseDoor(reader, stdout)
				break
			}
			question, err := promptDoorLine(reader, stdout, "Question:", false)
			if err != nil {
				return err
			}
			question = strings.TrimSpace(question)
			if question == "" {
				io.WriteString(stdout, "\r\nQuestion required.")
				pauseDoor(reader, stdout)
				break
			}
			state.Prompts++
			_ = saveUserStateJSON(r.repo, dctx.UserID, door.ID, state)
			_ = r.repo.AddEvent(&domain.DoorEvent{
				DoorID:      door.ID,
				UserID:      dctx.UserID,
				EventType:   "oracle_prompt",
				PayloadJSON: fmt.Sprintf(`{"question":%q}`, truncateJSONValue(question, 200)),
			})
			reply := oracleReply(question, dctx.Username)
			io.WriteString(stdout, "\r\n[AI-LABEL] "+reply)
			pauseDoor(reader, stdout)
		default:
			io.WriteString(stdout, "\r\nUnknown key.")
			pauseDoor(reader, stdout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) runANSIArtGallery(ctx context.Context, door Door, dctx DoorContext, reader *bufio.Reader, stdout io.Writer) error {
	artRoot := strings.TrimSpace(os.Getenv("WOLFBBS_ANSI_ART_DIR"))
	if artRoot == "" {
		artRoot = filepath.Join("doors", "ansi-art-gallery", "art")
	}
	_ = os.MkdirAll(artRoot, 0o755)
	for {
		entries, err := os.ReadDir(artRoot)
		if err != nil {
			return err
		}
		type artItem struct {
			Name string
			Path string
		}
		items := make([]artItem, 0, len(entries))
		for _, row := range entries {
			if row.IsDir() {
				continue
			}
			name := strings.TrimSpace(row.Name())
			if name == "" {
				continue
			}
			lower := strings.ToLower(name)
			if !strings.HasSuffix(lower, ".ans") && !strings.HasSuffix(lower, ".asc") && !strings.HasSuffix(lower, ".txt") {
				continue
			}
			items = append(items, artItem{Name: name, Path: filepath.Join(artRoot, name)})
		}
		sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })

		lines := []string{
			"ANSI/ASCII art packs with SAUCE metadata support.",
			"Art path: " + artRoot,
			"",
		}
		if len(items) == 0 {
			lines = append(lines, "No .ans/.asc/.txt files found.")
		} else {
			for idx, item := range items {
				lines = append(lines, fmt.Sprintf("%2d) %s", idx+1, item.Name))
			}
		}
		lines = append(lines, "", "(R)efresh  [#] view file  (Q)uit  (?)Help", "", "Enter selection:")
		renderDoorPanel(stdout, door.Name, lines, ui.FgYellow)
		key, err := readDoorKey(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch key {
		case "Q", "ESC":
			return nil
		case "R":
			continue
		case "H", "?":
			io.WriteString(stdout, "\r\nSelect a file number to render it. SAUCE metadata appears in the header.")
			pauseDoor(reader, stdout)
			continue
		default:
			idx, err := strconv.Atoi(strings.TrimSpace(key))
			if err != nil || idx <= 0 || idx > len(items) {
				io.WriteString(stdout, "\r\nUnknown selection.")
				pauseDoor(reader, stdout)
				continue
			}
			data, err := os.ReadFile(items[idx-1].Path)
			if err != nil {
				io.WriteString(stdout, "\r\nCould not read file: "+err.Error())
				pauseDoor(reader, stdout)
				continue
			}
			meta, hasSAUCE := term.ParseSAUCE(data)
			body := term.StripSAUCE(data)
			header := items[idx-1].Name
			if hasSAUCE {
				metaBits := []string{}
				if strings.TrimSpace(meta.Title) != "" {
					metaBits = append(metaBits, "Title:"+meta.Title)
				}
				if strings.TrimSpace(meta.Author) != "" {
					metaBits = append(metaBits, "Author:"+meta.Author)
				}
				if strings.TrimSpace(meta.Group) != "" {
					metaBits = append(metaBits, "Group:"+meta.Group)
				}
				if len(metaBits) > 0 {
					header += " (" + strings.Join(metaBits, " | ") + ")"
				}
			}
			io.WriteString(stdout, "\x1b[2J\x1b[H")
			io.WriteString(stdout, ui.Color(ui.FgYellow, "", header)+"\r\n")
			art := string(body)
			art = strings.ReplaceAll(art, "\r\n", "\n")
			art = strings.ReplaceAll(art, "\n", "\r\n")
			io.WriteString(stdout, ui.ApplyOutputProfile(art, dctx.ANSI, dctx.Encoding))
			io.WriteString(stdout, "\r\n\r\nPress any key to return to gallery.")
			pauseDoor(reader, stdout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Registry) spaceTraderTrade(reader *bufio.Reader, stdout io.Writer, state *spaceTraderState) (bool, string, error) {
	if state == nil {
		return false, "", nil
	}
	prices := traderPrices(state.Sector)
	io.WriteString(stdout, fmt.Sprintf("\r\nSector %d port prices: Ore:%d  Food:%d  Tech:%d\r\n", state.Sector, prices["ORE"], prices["FOOD"], prices["TECH"]))
	io.WriteString(stdout, "Trade action (B)uy, (S)ell, (Q)uit: ")
	action, err := readDoorKey(reader)
	if err != nil {
		return false, "", err
	}
	switch action {
	case "Q", "ESC":
		return false, "\r\nNo trade executed.\r\n", nil
	case "B":
		good, ok, err := promptCommodity(reader, stdout, "Buy commodity")
		if err != nil {
			return false, "", err
		}
		if !ok {
			return false, "\r\nCancelled.\r\n", nil
		}
		price := prices[good]
		space := state.CargoCap - stateCargoUsed(*state)
		if space <= 0 {
			return false, "\r\nCargo hold is full.\r\n", nil
		}
		maxAffordable := int(state.Credits / price)
		maxQty := minInt(space, maxAffordable)
		if maxQty <= 0 {
			return false, "\r\nInsufficient credits.\r\n", nil
		}
		qty, err := promptDoorInt(reader, stdout, fmt.Sprintf("Quantity (1-%d): ", maxQty), 1, 1, maxQty)
		if err != nil {
			return false, "", err
		}
		cost := int64(qty) * price
		state.Credits -= cost
		if state.Cargo == nil {
			state.Cargo = map[string]int{}
		}
		state.Cargo[good] += qty
		return true, fmt.Sprintf("\r\nBought %d %s for %d credits.\r\n", qty, good, cost), nil
	case "S":
		good, ok, err := promptCommodity(reader, stdout, "Sell commodity")
		if err != nil {
			return false, "", err
		}
		if !ok {
			return false, "\r\nCancelled.\r\n", nil
		}
		owned := state.Cargo[good]
		if owned <= 0 {
			return false, "\r\nNo cargo to sell.\r\n", nil
		}
		price := prices[good]
		qty, err := promptDoorInt(reader, stdout, fmt.Sprintf("Quantity (1-%d): ", owned), 1, 1, owned)
		if err != nil {
			return false, "", err
		}
		state.Cargo[good] -= qty
		revenue := int64(qty) * price
		state.Credits += revenue
		return true, fmt.Sprintf("\r\nSold %d %s for %d credits.\r\n", qty, good, revenue), nil
	default:
		return false, "\r\nUnknown trade selection.\r\n", nil
	}
}

func (r *Registry) spaceTraderWarp(reader *bufio.Reader, stdout io.Writer, state *spaceTraderState) (bool, string, error) {
	target, err := promptDoorInt(reader, stdout, "Warp to sector (1-40): ", state.Sector, 1, 40)
	if err != nil {
		return false, "", err
	}
	if target == state.Sector {
		return false, "\r\nHolding position.\r\n", nil
	}
	delta := absInt(target - state.Sector)
	if delta > 5 {
		return false, "\r\nWarp lanes limited to 5 sectors per jump.\r\n", nil
	}
	fuel := int64(delta * 6)
	if state.Credits < fuel {
		return false, fmt.Sprintf("\r\nNeed %d credits for fuel.\r\n", fuel), nil
	}
	state.Credits -= fuel
	state.Sector = target
	return true, fmt.Sprintf("\r\nWarp complete. Arrived sector %d (fuel %d).\r\n", target, fuel), nil
}

func (r *Registry) spaceTraderUpgrade(reader *bufio.Reader, stdout io.Writer, state *spaceTraderState) (bool, string, error) {
	io.WriteString(stdout, "\r\nUpgrade options: (H)ull +20 [250cr], (C)argo +5 [180cr], (W)eapon +1 [300cr], (Q)uit: ")
	key, err := readDoorKey(reader)
	if err != nil {
		return false, "", err
	}
	switch key {
	case "Q", "ESC":
		return false, "\r\nUpgrade canceled.\r\n", nil
	case "H":
		if state.Credits < 250 {
			return false, "\r\nNeed 250 credits.\r\n", nil
		}
		state.Credits -= 250
		state.Hull += 20
		return true, "\r\nHull reinforced.\r\n", nil
	case "C":
		if state.Credits < 180 {
			return false, "\r\nNeed 180 credits.\r\n", nil
		}
		state.Credits -= 180
		state.CargoCap += 5
		return true, "\r\nCargo hold expanded.\r\n", nil
	case "W":
		if state.Credits < 300 {
			return false, "\r\nNeed 300 credits.\r\n", nil
		}
		state.Credits -= 300
		state.Weapon++
		return true, "\r\nWeapon systems upgraded.\r\n", nil
	default:
		return false, "\r\nUnknown upgrade key.\r\n", nil
	}
}

func (r *Registry) spaceTraderFight(state *spaceTraderState) (bool, string) {
	if state == nil {
		return false, ""
	}
	enemy := int64(25 + ((state.Sector*13 + int(state.Wins*7) + int(state.Losses*5)) % 45))
	player := int64(state.Hull/2 + state.Weapon*30 + int(state.Credits/120))
	if player >= enemy {
		reward := 60 + enemy/2
		state.Credits += reward
		state.Wins++
		if state.Hull > 25 {
			state.Hull -= 4
		}
		return true, fmt.Sprintf("\r\nVictory! Enemy power %d. Salvage +%d credits.\r\n", enemy, reward)
	}
	penalty := minInt64(state.Credits, enemy/2+18)
	state.Credits -= penalty
	state.Losses++
	state.Hull = maxInt(20, state.Hull-8)
	return true, fmt.Sprintf("\r\nDefeat. Enemy power %d. Repairs cost %d credits.\r\n", enemy, penalty)
}

func (r *Registry) writeSpaceTraderHoldings(stdout io.Writer, state spaceTraderState) {
	prices := traderPrices(state.Sector)
	net := state.Credits
	io.WriteString(stdout, "\r\nCargo manifest:\r\n")
	order := []string{"ORE", "FOOD", "TECH"}
	for _, good := range order {
		qty := state.Cargo[good]
		value := int64(qty) * prices[good]
		net += value
		io.WriteString(stdout, fmt.Sprintf("  %-4s qty:%d  est:%d\r\n", good, qty, value))
	}
	io.WriteString(stdout, fmt.Sprintf("Net worth estimate: %d\r\n", net))
}

func dragonForestRun(state *dragonTavernState) (bool, string) {
	if state == nil {
		return false, ""
	}
	seed := int64(state.ForestRuns*11 + int64(state.Level*7) + state.XP + state.Gold)
	enemyPower := int64(state.Level*8 + 20 + int(seed%17))
	playerPower := int64(state.Attack*4 + state.Defense*3 + state.HP/2 + state.Level*6)
	state.ForestRuns++
	if playerPower >= enemyPower {
		xpGain := enemyPower + int64(8+state.Level*2)
		goldGain := int64(20 + int(seed%16))
		state.XP += xpGain
		state.Gold += goldGain
		levelUps := dragonApplyLevelUps(state)
		return true, fmt.Sprintf("\r\nForest clear. +%d XP +%d gold. Level ups:%d\r\n", xpGain, goldGain, levelUps)
	}
	damage := maxInt(6, int(enemyPower/12))
	state.HP -= damage
	if state.HP <= 0 {
		state.HP = 1
		loss := minInt64(state.Gold, 20+enemyPower/4)
		state.Gold -= loss
		return true, fmt.Sprintf("\r\nAmbushed. You were revived at 1 HP. Lost %d gold.\r\n", loss)
	}
	return true, fmt.Sprintf("\r\nHard fight. Lost %d HP.\r\n", damage)
}

func dragonTavernRun(state *dragonTavernState) (bool, string) {
	if state == nil {
		return false, ""
	}
	state.TavernRuns++
	event := (state.TavernRuns + int64(state.Level) + state.XP) % 4
	switch event {
	case 0:
		heal := minInt(state.MaxHP-state.HP, 12)
		if heal < 0 {
			heal = 0
		}
		cost := int64(10)
		if state.Gold >= cost && heal > 0 {
			state.Gold -= cost
			state.HP += heal
			return true, fmt.Sprintf("\r\nTavern healer restored %d HP for %d gold.\r\n", heal, cost)
		}
		return true, "\r\nNo healing needed tonight.\r\n"
	case 1:
		tip := int64(18 + (state.TavernRuns % 10))
		state.Gold += tip
		return true, fmt.Sprintf("\r\nA bard paid you %d gold for a story.\r\n", tip)
	case 2:
		xp := int64(20 + state.Level*3)
		state.XP += xp
		levelUps := dragonApplyLevelUps(state)
		return true, fmt.Sprintf("\r\nYou learned old battle lore. +%d XP (level ups:%d)\r\n", xp, levelUps)
	default:
		damage := 5 + int(state.TavernRuns%6)
		state.HP = maxInt(1, state.HP-damage)
		state.Gold += 12
		return true, fmt.Sprintf("\r\nMinor tavern brawl. -%d HP, +12 gold.\r\n", damage)
	}
}

func dragonSmithyRun(reader *bufio.Reader, stdout io.Writer, state *dragonTavernState) (bool, string, error) {
	if state == nil {
		return false, "", nil
	}
	io.WriteString(stdout, "\r\nSmithy: (P)otion 15g  (B)lade +1 atk 45g  (A)rmor +1 def 45g  (Q)uit: ")
	key, err := readDoorKey(reader)
	if err != nil {
		return false, "", err
	}
	switch key {
	case "Q", "ESC":
		return false, "\r\nLeaving smithy.\r\n", nil
	case "P":
		if state.Gold < 15 {
			return false, "\r\nNeed 15 gold.\r\n", nil
		}
		state.Gold -= 15
		state.HP = minInt(state.MaxHP, state.HP+16)
		return true, "\r\nPotion used.\r\n", nil
	case "B":
		if state.Gold < 45 {
			return false, "\r\nNeed 45 gold.\r\n", nil
		}
		state.Gold -= 45
		state.Attack++
		return true, "\r\nBlade sharpened.\r\n", nil
	case "A":
		if state.Gold < 45 {
			return false, "\r\nNeed 45 gold.\r\n", nil
		}
		state.Gold -= 45
		state.Defense++
		state.MaxHP++
		return true, "\r\nArmor reinforced.\r\n", nil
	default:
		return false, "\r\nUnknown smithy option.\r\n", nil
	}
}

func dragonDuelRun(state *dragonTavernState) (bool, string) {
	if state == nil {
		return false, ""
	}
	resetDragonDuelWindow(state, time.Now().UTC())
	if state.DuelsUsed >= 3 {
		return false, "\r\nNo duels remaining today.\r\n"
	}
	state.DuelsUsed++
	enemy := int64(state.Level*10 + 25 + int(state.DuelsUsed*7))
	player := int64(state.Attack*5 + state.Defense*3 + state.Level*7 + state.HP/2)
	if player >= enemy {
		state.DuelsWon++
		state.Gold += 30 + int64(state.Level*4)
		state.XP += 28 + int64(state.Level*5)
		levelUps := dragonApplyLevelUps(state)
		return true, fmt.Sprintf("\r\nDuel victory. +gold +XP. Level ups:%d\r\n", levelUps)
	}
	state.DuelsLost++
	damage := 6 + int(enemy/18)
	state.HP = maxInt(1, state.HP-damage)
	return true, fmt.Sprintf("\r\nDuel loss. Took %d damage.\r\n", damage)
}

func dragonNormalizeStats(state *dragonTavernState) {
	if state.Level < 1 {
		state.Level = 1
	}
	if state.MaxHP < 20 {
		state.MaxHP = 20
	}
	state.HP = minInt(state.HP, state.MaxHP)
	if state.HP < 1 {
		state.HP = 1
	}
	if state.Attack < 4 {
		state.Attack = 4
	}
	if state.Defense < 2 {
		state.Defense = 2
	}
}

func dragonApplyLevelUps(state *dragonTavernState) int {
	if state == nil {
		return 0
	}
	levelUps := 0
	for {
		threshold := int64(state.Level * 120)
		if state.XP < threshold {
			break
		}
		state.XP -= threshold
		state.Level++
		state.MaxHP += 8
		state.Attack += 2
		state.Defense++
		state.HP = state.MaxHP
		levelUps++
	}
	return levelUps
}

func resetDragonDuelWindow(state *dragonTavernState, now time.Time) {
	if state == nil {
		return
	}
	day := now.UTC().Format("2006-01-02")
	if state.DuelDay != day {
		state.DuelDay = day
		state.DuelsUsed = 0
	}
}

func (r *Registry) votingCreatePoll(reader *bufio.Reader, stdout io.Writer, dctx DoorContext, global *votingGlobalState) (bool, string, error) {
	if global == nil {
		return false, "", nil
	}
	question, err := promptDoorLine(reader, stdout, "Question: ", false)
	if err != nil {
		return false, "", err
	}
	if question == "" {
		return false, "\r\nQuestion is required.\r\n", nil
	}
	rawOptions, err := promptDoorLine(reader, stdout, "Options (comma-separated): ", false)
	if err != nil {
		return false, "", err
	}
	options := splitCSV(rawOptions)
	if len(options) < 2 {
		return false, "\r\nNeed at least two options.\r\n", nil
	}
	if global.NextID <= 0 {
		global.NextID = 1
	}
	p := votingPoll{
		ID:        global.NextID,
		Question:  truncate(question, 120),
		Options:   options,
		Counts:    make([]int64, len(options)),
		Open:      true,
		CreatedBy: dctx.UserID,
		CreatedAt: time.Now().UTC().Unix(),
	}
	global.NextID++
	global.Polls = append(global.Polls, p)
	sort.Slice(global.Polls, func(i, j int) bool { return global.Polls[i].ID < global.Polls[j].ID })
	_ = r.repo.AddEvent(&domain.DoorEvent{
		DoorID:      "voting-booth",
		UserID:      dctx.UserID,
		EventType:   "poll_create",
		PayloadJSON: fmt.Sprintf(`{"poll_id":%d}`, p.ID),
	})
	return true, fmt.Sprintf("\r\nPoll #%d created.\r\n", p.ID), nil
}

func (r *Registry) votingCastVote(reader *bufio.Reader, stdout io.Writer, dctx DoorContext, global *votingGlobalState, userState *votingUserState) (bool, bool, string, error) {
	if global == nil || userState == nil {
		return false, false, "", nil
	}
	if len(global.Polls) == 0 {
		return false, false, "\r\nNo polls available.\r\n", nil
	}
	pollID, err := promptDoorInt(reader, stdout, "Poll ID: ", 0, 1, 1000000)
	if err != nil {
		return false, false, "", err
	}
	idx := findPollIndex(global.Polls, pollID)
	if idx < 0 {
		return false, false, "\r\nUnknown poll.\r\n", nil
	}
	poll := &global.Polls[idx]
	if !poll.Open {
		return false, false, "\r\nPoll is closed.\r\n", nil
	}
	key := strconv.Itoa(poll.ID)
	if _, voted := userState.Votes[key]; voted {
		return false, false, "\r\nYou already voted on this poll.\r\n", nil
	}
	io.WriteString(stdout, "\r\n")
	for i, option := range poll.Options {
		io.WriteString(stdout, fmt.Sprintf("  %d) %s\r\n", i+1, option))
	}
	choice, err := promptDoorInt(reader, stdout, fmt.Sprintf("Choice (1-%d): ", len(poll.Options)), 1, 1, len(poll.Options))
	if err != nil {
		return false, false, "", err
	}
	poll.Counts[choice-1]++
	userState.Votes[key] = choice
	_ = r.repo.AddEvent(&domain.DoorEvent{
		DoorID:      "voting-booth",
		UserID:      dctx.UserID,
		EventType:   "poll_vote",
		PayloadJSON: fmt.Sprintf(`{"poll_id":%d,"choice":%d}`, poll.ID, choice),
	})
	return true, true, "\r\nVote recorded.\r\n", nil
}

func (r *Registry) votingTogglePoll(reader *bufio.Reader, stdout io.Writer, global *votingGlobalState) (bool, string, error) {
	if global == nil || len(global.Polls) == 0 {
		return false, "\r\nNo polls available.\r\n", nil
	}
	id, err := promptDoorInt(reader, stdout, "Poll ID: ", 0, 1, 1000000)
	if err != nil {
		return false, "", err
	}
	idx := findPollIndex(global.Polls, id)
	if idx < 0 {
		return false, "\r\nUnknown poll.\r\n", nil
	}
	global.Polls[idx].Open = !global.Polls[idx].Open
	state := "closed"
	if global.Polls[idx].Open {
		state = "opened"
	}
	return true, fmt.Sprintf("\r\nPoll #%d %s.\r\n", id, state), nil
}

func (r *Registry) writePollList(stdout io.Writer, global votingGlobalState) {
	io.WriteString(stdout, "\r\nPoll list:\r\n")
	if len(global.Polls) == 0 {
		io.WriteString(stdout, "  no polls yet\r\n")
		return
	}
	sort.Slice(global.Polls, func(i, j int) bool { return global.Polls[i].ID < global.Polls[j].ID })
	for _, poll := range global.Polls {
		status := "open"
		if !poll.Open {
			status = "closed"
		}
		io.WriteString(stdout, fmt.Sprintf("  #%d [%s] %s\r\n", poll.ID, status, poll.Question))
	}
}

func (r *Registry) writePollResults(stdout io.Writer, global votingGlobalState) {
	io.WriteString(stdout, "\r\nPoll results:\r\n")
	if len(global.Polls) == 0 {
		io.WriteString(stdout, "  no polls yet\r\n")
		return
	}
	sort.Slice(global.Polls, func(i, j int) bool { return global.Polls[i].ID < global.Polls[j].ID })
	for _, poll := range global.Polls {
		total := int64(0)
		for _, c := range poll.Counts {
			total += c
		}
		io.WriteString(stdout, fmt.Sprintf("\r\n#%d %s (votes:%d)\r\n", poll.ID, poll.Question, total))
		for i, option := range poll.Options {
			count := int64(0)
			if i < len(poll.Counts) {
				count = poll.Counts[i]
			}
			bar := votingBar(count, total, 30)
			io.WriteString(stdout, fmt.Sprintf("  %d) %-24s [%s] %d\r\n", i+1, truncate(option, 24), bar, count))
		}
	}
}

func votingBar(count, total int64, width int) string {
	if width <= 0 {
		width = 20
	}
	if total <= 0 || count <= 0 {
		return strings.Repeat(".", width)
	}
	fill := int((count * int64(width)) / total)
	if fill < 1 {
		fill = 1
	}
	if fill > width {
		fill = width
	}
	return strings.Repeat("#", fill) + strings.Repeat(".", width-fill)
}

func votingCounts(global votingGlobalState) (int, int) {
	total := len(global.Polls)
	open := 0
	for _, poll := range global.Polls {
		if poll.Open {
			open++
		}
	}
	return total, open
}

func findPollIndex(polls []votingPoll, id int) int {
	for i := range polls {
		if polls[i].ID == id {
			return i
		}
	}
	return -1
}

func filebaseAdd(reader *bufio.Reader, stdout io.Writer, dctx DoorContext, global *filebaseGlobalState, userState *filebaseUserState) (bool, bool, string, error) {
	if global == nil || userState == nil {
		return false, false, "", nil
	}
	area, err := promptDoorLine(reader, stdout, "File area [main]: ", true)
	if err != nil {
		return false, false, "", err
	}
	if area == "" {
		area = "main"
	}
	filename, err := promptDoorLine(reader, stdout, "Filename: ", false)
	if err != nil {
		return false, false, "", err
	}
	if filename == "" {
		return false, false, "\r\nFilename required.\r\n", nil
	}
	description, err := promptDoorLine(reader, stdout, "Description: ", false)
	if err != nil {
		return false, false, "", err
	}
	if description == "" {
		return false, false, "\r\nDescription required.\r\n", nil
	}
	tagsRaw, err := promptDoorLine(reader, stdout, "Tags (comma-separated): ", true)
	if err != nil {
		return false, false, "", err
	}
	fingerprint, err := promptDoorLine(reader, stdout, "Fingerprint text for checksum: ", false)
	if err != nil {
		return false, false, "", err
	}
	if fingerprint == "" {
		return false, false, "\r\nFingerprint text required for checksum.\r\n", nil
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(filename)) + "|" + strings.TrimSpace(description) + "|" + strings.TrimSpace(fingerprint)))
	hash := hex.EncodeToString(sum[:])
	for _, row := range global.Files {
		if row.SHA256 == hash {
			return false, false, fmt.Sprintf("\r\nDuplicate detected. Matches file #%d.\r\n", row.ID), nil
		}
	}
	if global.NextID <= 0 {
		global.NextID = 1
	}
	nowUnix := time.Now().UTC().Unix()
	record := fileRecord{
		ID:          global.NextID,
		Area:        truncate(strings.ToLower(area), 24),
		Filename:    truncate(filename, 64),
		Description: truncate(description, 120),
		Tags:        splitCSV(tagsRaw),
		Uploader:    dctx.UserID,
		SHA256:      hash,
		UploadedAt:  nowUnix,
		Downloads:   0,
	}
	global.NextID++
	global.Files = append(global.Files, record)
	sort.Slice(global.Files, func(i, j int) bool {
		if global.Files[i].UploadedAt == global.Files[j].UploadedAt {
			return global.Files[i].ID < global.Files[j].ID
		}
		return global.Files[i].UploadedAt > global.Files[j].UploadedAt
	})
	userState.Uploads++
	return true, true, fmt.Sprintf("\r\nFile #%d indexed with checksum %s.\r\n", record.ID, record.SHA256[:12]), nil
}

func filebaseSearch(reader *bufio.Reader, stdout io.Writer, files []fileRecord) (string, error) {
	term, err := promptDoorLine(reader, stdout, "Search term: ", false)
	if err != nil {
		return "", err
	}
	term = strings.ToLower(term)
	if term == "" {
		return "\r\nNo term entered.\r\n", nil
	}
	matches := []fileRecord{}
	for _, row := range files {
		if strings.Contains(strings.ToLower(row.Filename), term) || strings.Contains(strings.ToLower(row.Description), term) {
			matches = append(matches, row)
			continue
		}
		for _, tag := range row.Tags {
			if strings.Contains(strings.ToLower(tag), term) {
				matches = append(matches, row)
				break
			}
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\r\nSearch results for %q:\r\n", term))
	if len(matches) == 0 {
		b.WriteString("  no matches\r\n")
		return b.String(), nil
	}
	writeFileRows(&b, matches)
	return b.String(), nil
}

func filebaseNewScan(userState *filebaseUserState, files []fileRecord) string {
	if userState == nil {
		return "\r\n"
	}
	var b strings.Builder
	b.WriteString("\r\nNew files since last scan:\r\n")
	count := 0
	for _, row := range files {
		if row.UploadedAt > userState.LastScanUnix {
			count++
			b.WriteString(fmt.Sprintf("  #%d %-24s %-30s %s\r\n", row.ID, truncate(row.Area, 24), truncate(row.Filename, 30), formatUnix(row.UploadedAt)))
		}
	}
	if count == 0 {
		b.WriteString("  none\r\n")
	}
	userState.LastScanUnix = time.Now().UTC().Unix()
	return b.String()
}

func filebaseDownload(reader *bufio.Reader, stdout io.Writer, global *filebaseGlobalState, userState *filebaseUserState) (bool, bool, string, error) {
	if global == nil || userState == nil {
		return false, false, "", nil
	}
	if len(global.Files) == 0 {
		return false, false, "\r\nNo files indexed.\r\n", nil
	}
	id, err := promptDoorInt(reader, stdout, "Download file ID: ", 0, 1, 1000000)
	if err != nil {
		return false, false, "", err
	}
	for i := range global.Files {
		if global.Files[i].ID == id {
			global.Files[i].Downloads++
			userState.Downloads++
			return true, true, fmt.Sprintf("\r\nQueued %s for transfer. Total downloads: %d\r\n", global.Files[i].Filename, global.Files[i].Downloads), nil
		}
	}
	return false, false, "\r\nUnknown file id.\r\n", nil
}

func writeFileList(stdout io.Writer, files []fileRecord, title string) {
	io.WriteString(stdout, "\r\n"+title+"\r\n")
	if len(files) == 0 {
		io.WriteString(stdout, "  no files indexed\r\n")
		return
	}
	copyRows := append([]fileRecord{}, files...)
	sort.Slice(copyRows, func(i, j int) bool {
		if copyRows[i].UploadedAt == copyRows[j].UploadedAt {
			return copyRows[i].ID < copyRows[j].ID
		}
		return copyRows[i].UploadedAt > copyRows[j].UploadedAt
	})
	var b strings.Builder
	writeFileRows(&b, copyRows)
	io.WriteString(stdout, b.String())
}

func writeFileRows(b *strings.Builder, rows []fileRecord) {
	for _, row := range rows {
		tags := strings.Join(row.Tags, ",")
		if tags == "" {
			tags = "-"
		}
		b.WriteString(fmt.Sprintf("  #%d %-18s %-24s dl:%d %s\r\n", row.ID, truncate(row.Area, 18), truncate(row.Filename, 24), row.Downloads, formatUnix(row.UploadedAt)))
		b.WriteString(fmt.Sprintf("      %-52s tags:%s\r\n", truncate(row.Description, 52), truncate(tags, 24)))
	}
}

func (r *Registry) writeTopScores(stdout io.Writer, doorID, scoreType, label string) {
	rows, _ := r.repo.ListScores(doorID, scoreType, 10)
	io.WriteString(stdout, "\r\n"+label+"\r\n")
	if len(rows) == 0 {
		io.WriteString(stdout, "  no scores yet\r\n")
		return
	}
	for idx, row := range rows {
		io.WriteString(stdout, fmt.Sprintf("  %2d) uid:%d  %d\r\n", idx+1, row.UserID, row.Value))
	}
}

func renderDoorPanel(stdout io.Writer, title string, lines []string, fg string) {
	if stdout == nil {
		return
	}
	height := len(lines) + 2
	if height < 8 {
		height = 8
	}
	if height > 22 {
		height = 22
		lines = lines[:height-2]
	}
	panel := ui.DrawBox(80, height, title, lines, ui.CP437Box, fg, ui.BgBlack)
	io.WriteString(stdout, ui.ClearScreen())
	io.WriteString(stdout, panel)
}

func pauseDoor(reader *bufio.Reader, stdout io.Writer) {
	io.WriteString(stdout, "\r\nPress any key.")
	_, _ = readDoorKey(reader)
}

func promptCommodity(reader *bufio.Reader, stdout io.Writer, prompt string) (string, bool, error) {
	io.WriteString(stdout, "\r\n"+prompt+" (O)re (F)ood (T)ech (Q)uit: ")
	key, err := readDoorKey(reader)
	if err != nil {
		return "", false, err
	}
	switch key {
	case "O":
		return "ORE", true, nil
	case "F":
		return "FOOD", true, nil
	case "T":
		return "TECH", true, nil
	default:
		return "", false, nil
	}
}

func promptDoorInt(reader *bufio.Reader, stdout io.Writer, prompt string, def, minValue, maxValue int) (int, error) {
	if maxValue < minValue {
		maxValue = minValue
	}
	line, err := promptDoorLine(reader, stdout, prompt, true)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return def, nil
		}
		return def, err
	}
	if line == "" {
		return clampInt(def, minValue, maxValue), nil
	}
	n, convErr := strconv.Atoi(line)
	if convErr != nil {
		return clampInt(def, minValue, maxValue), nil
	}
	return clampInt(n, minValue, maxValue), nil
}

func promptDoorLine(reader *bufio.Reader, stdout io.Writer, prompt string, allowEmpty bool) (string, error) {
	if prompt != "" {
		io.WriteString(stdout, "\r\n"+prompt)
	}
	skippedLeadingEmpty := false
	for {
		line, err := readDoorLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				line = strings.TrimSpace(line)
				if line == "" && !allowEmpty {
					return "", nil
				}
				return line, nil
			}
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" && !allowEmpty {
			if !skippedLeadingEmpty {
				skippedLeadingEmpty = true
				continue
			}
			io.WriteString(stdout, "required: ")
			continue
		}
		return line, nil
	}
}

func readDoorLine(reader *bufio.Reader) (string, error) {
	if reader == nil {
		return "", io.EOF
	}
	line, err := reader.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil {
		if errors.Is(err, io.EOF) {
			return line, io.EOF
		}
		return line, err
	}
	return line, nil
}

func loadUserStateJSON[T any](repo repository.DoorRepository, userID int64, doorID string, defaults T) (T, error) {
	out := defaults
	row, err := repo.GetUserState(userID, doorID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return out, nil
		}
		return out, err
	}
	if row == nil || strings.TrimSpace(row.StateJSON) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(row.StateJSON), &out); err != nil {
		return defaults, nil
	}
	return out, nil
}

func saveUserStateJSON[T any](repo repository.DoorRepository, userID int64, doorID string, state T) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return repo.UpsertUserState(&domain.DoorUserState{
		UserID:    userID,
		DoorID:    doorID,
		StateJSON: string(body),
		UpdatedAt: time.Now().UTC(),
	})
}

func loadGlobalStateJSON[T any](repo repository.DoorRepository, doorID string, defaults T) (T, error) {
	out := defaults
	row, err := repo.GetGlobalState(doorID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return out, nil
		}
		return out, err
	}
	if row == nil || strings.TrimSpace(row.StateJSON) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(row.StateJSON), &out); err != nil {
		return defaults, nil
	}
	return out, nil
}

func saveGlobalStateJSON[T any](repo repository.DoorRepository, doorID string, state T) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return repo.UpsertGlobalState(&domain.DoorGlobalState{
		DoorID:    doorID,
		StateJSON: string(body),
		UpdatedAt: time.Now().UTC(),
	})
}

func defaultSpaceTraderState() spaceTraderState {
	return spaceTraderState{
		Sector:   1,
		Credits:  600,
		CargoCap: 20,
		Hull:     100,
		Weapon:   1,
		Cargo: map[string]int{
			"ORE":  0,
			"FOOD": 0,
			"TECH": 0,
		},
	}
}

func defaultDragonTavernState() dragonTavernState {
	return dragonTavernState{
		Level:   1,
		XP:      0,
		Gold:    140,
		HP:      40,
		MaxHP:   40,
		Attack:  6,
		Defense: 3,
	}
}

func oraclePromptLimit() int {
	raw := strings.TrimSpace(os.Getenv("WOLFBBS_ORACLE_DAILY_PROMPTS"))
	if raw == "" {
		return 10
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 10
	}
	if v > 100 {
		return 100
	}
	return v
}

func oracleReply(question, handle string) string {
	q := strings.ToLower(strings.TrimSpace(question))
	switch {
	case strings.Contains(q, "where"):
		return "Start with (N)ewscan, then check Boards and local rules."
	case strings.Contains(q, "door"):
		return "Door Night tip: pick one door, post results, and challenge one caller."
	case strings.Contains(q, "ansi"), strings.Contains(q, "art"):
		return "ANSI craft wins here. Keep lines tight and palette intentional."
	case strings.Contains(q, "help"):
		return "Use ? in every menu; WolfBBS keeps commands single-letter and direct."
	default:
		name := strings.TrimSpace(handle)
		if name == "" {
			name = "caller"
		}
		return "Noted, " + name + ". Keep it concise, cite sources, and keep board culture human-first."
	}
}

func traderPrices(sector int) map[string]int64 {
	if sector <= 0 {
		sector = 1
	}
	return map[string]int64{
		"ORE":  10 + int64((sector*3)%9),
		"FOOD": 7 + int64((sector*5)%8),
		"TECH": 24 + int64((sector*7)%10),
	}
}

func stateCargoUsed(state spaceTraderState) int {
	total := 0
	for _, qty := range state.Cargo {
		total += qty
	}
	return total
}

func spaceTraderScoreValue(state spaceTraderState) int64 {
	prices := traderPrices(state.Sector)
	value := state.Credits + int64(state.Hull*4) + int64(state.Weapon*70)
	for good, qty := range state.Cargo {
		value += prices[good] * int64(qty)
	}
	value += state.Wins * 55
	return value
}

func templateDoorScoreValue(state templateDoorState) int64 {
	return (state.Missions * 30) + (state.Mastery * 50) + (state.Rank * 100) + state.Credits
}

func dragonScoreValue(state dragonTavernState) int64 {
	return int64(state.Level*1000) + state.Gold + (state.DuelsWon * 140) + (state.ForestRuns * 24)
}

func filebaseScoreValue(state filebaseUserState) int64 {
	return (state.Uploads * 250) + (state.Downloads * 35)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, truncate(part, 32))
	}
	return out
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return "never"
	}
	return time.Unix(ts, 0).UTC().Format("2006-01-02 15:04")
}

func fallbackName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fallback
	}
	return name
}

func isModOrAdmin(role string) bool {
	return rbac.AtLeast(role, rbac.RoleModerator)
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
