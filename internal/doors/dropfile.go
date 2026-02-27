package doors

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type generatedDropfiles struct {
	Door32Path  string
	DoorSysPath string
	DorInfoPath string
}

func writeConfiguredDropfiles(dataDir string, ctx DoorContext) (generatedDropfiles, error) {
	raw := strings.TrimSpace(os.Getenv("WOLFBBS_DOOR_DROPFILES"))
	if raw == "" {
		return generatedDropfiles{}, nil
	}
	parts := strings.Split(strings.ToLower(raw), ",")
	wantDoor32 := false
	wantDoorSys := false
	wantDorInfo := false
	for _, part := range parts {
		switch strings.TrimSpace(part) {
		case "door32", "door32.sys":
			wantDoor32 = true
		case "door.sys", "doorsys":
			wantDoorSys = true
		case "dorinfo", "dorinfox", "dorinfo1", "dorinfo1.def":
			wantDorInfo = true
		}
	}
	out := generatedDropfiles{}
	if wantDoor32 {
		path := filepath.Join(dataDir, "DOOR32.SYS")
		if err := os.WriteFile(path, []byte(buildDoor32(ctx)), 0o600); err != nil {
			return out, err
		}
		out.Door32Path = path
	}
	if wantDoorSys {
		path := filepath.Join(dataDir, "DOOR.SYS")
		if err := os.WriteFile(path, []byte(buildDoorSys(ctx)), 0o600); err != nil {
			return out, err
		}
		out.DoorSysPath = path
	}
	if wantDorInfo {
		node := parseNodeNumber(ctx.NodeID)
		if node < 1 || node > 9 {
			node = 1
		}
		path := filepath.Join(dataDir, "DORINFO"+strconv.Itoa(node)+".DEF")
		if err := os.WriteFile(path, []byte(buildDorInfo(ctx)), 0o600); err != nil {
			return out, err
		}
		out.DorInfoPath = path
	}
	return out, nil
}

func buildDoor32(ctx DoorContext) string {
	lines := []string{
		"0",
		"0",
		"57600",
		"WolfBBS",
		strconv.FormatInt(ctx.UserID, 10),
		fallback(ctx.Username, "Unknown"),
		fallback(ctx.Username, "Unknown"),
		"30",
		"256",
		strconv.Itoa(parseNodeNumber(ctx.NodeID)),
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func buildDoorSys(ctx DoorContext) string {
	lines := []string{
		"COM1:",
		"57600",
		"8",
		strconv.Itoa(parseNodeNumber(ctx.NodeID)),
		"57600",
		"Y",
		"Y",
		"Y",
		"Y",
		fallback(ctx.Username, "Unknown"),
		fallback(ctx.Username, "Unknown"),
		"Unknown",
		"1",
		"100",
		"25",
		"N",
		"1",
		"1",
		"01/01/99",
		"123",
		"00:00",
		"00:00",
		"9999",
		"0",
		"0",
		"0",
		"9999",
		"01/01/99",
		"C:\\WOLFBBS",
		"C:\\WOLFBBS",
		"Sysop",
		fallback(ctx.Username, "Unknown"),
		"00:00",
		"Y",
		"Y",
		"Y",
		"7",
		"256",
		"0",
		"1",
		"No",
		"No",
		"No",
		"0",
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func buildDorInfo(ctx DoorContext) string {
	lines := []string{
		"WolfBBS",
		"Sysop",
		"Wolf",
		"COM1",
		"57600",
		"0",
		fallback(ctx.Username, "Unknown"),
		fallback(ctx.Username, "Unknown"),
		"00:00",
		"1",
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

func parseNodeNumber(value string) int {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 1
	}
	digits := make([]rune, 0, len(value))
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return 1
	}
	n, err := strconv.Atoi(string(digits))
	if err != nil || n <= 0 {
		return 1
	}
	return n
}

func fallback(value, def string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return def
	}
	return value
}
