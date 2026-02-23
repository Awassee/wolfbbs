package doors

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Door struct {
	Name        string
	Description string
	Command     string
	Args        []string
	Hotkey      string
}

type Registry struct {
	doors map[string]Door
}

func NewRegistry() *Registry {
	return &Registry{doors: map[string]Door{}}
}

func (r *Registry) Register(door Door) {
	door.Hotkey = strings.ToUpper(strings.TrimSpace(door.Hotkey))
	if door.Hotkey == "" || strings.TrimSpace(door.Command) == "" {
		return
	}
	r.doors[door.Hotkey] = door
}

func (r *Registry) Doors() []Door {
	out := make([]Door, 0, len(r.doors))
	for _, d := range r.doors {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Hotkey < out[j].Hotkey
	})
	return out
}

func (r *Registry) Launch(ctx context.Context, hotkey string, stdin io.Reader, stdout, stderr io.Writer, env map[string]string) error {
	door, ok := r.doors[strings.ToUpper(strings.TrimSpace(hotkey))]
	if !ok {
		return fmt.Errorf("unknown door")
	}
	commandPath, err := resolveDoorCommand(door.Command)
	if err != nil {
		return err
	}
	if err := validateDoorPath(commandPath); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, commandPath, door.Args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "WOLFBBS_MODE=door")
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("door %s failed: %w", door.Name, err)
	}
	return nil
}

func TriviaDoor(binary string) Door {
	if strings.TrimSpace(binary) == "" {
		binary = "wolfbbs-trivia"
	}
	return Door{
		Name:        "Trivia",
		Description: "Simple terminal trivia game",
		Command:     binary,
		Hotkey:      "T",
	}
}

func SeedTrivia(r *Registry, binary string) {
	r.Register(TriviaDoor(binary))
}

func SeedFromEnv(r *Registry) {
	raw := strings.TrimSpace(os.Getenv("WOLFBBS_DOORS"))
	if raw == "" {
		return
	}
	entries := strings.Split(raw, ";")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) < 3 {
			continue
		}
		door := Door{
			Hotkey:  strings.TrimSpace(parts[0]),
			Name:    strings.TrimSpace(parts[1]),
			Command: strings.TrimSpace(parts[2]),
		}
		if len(parts) >= 4 {
			args := []string{}
			for _, arg := range strings.Split(parts[3], ",") {
				clean := strings.TrimSpace(arg)
				if clean != "" {
					args = append(args, clean)
				}
			}
			door.Args = args
		}
		if door.Name == "" {
			door.Name = "door-" + strings.ToLower(door.Hotkey)
		}
		r.Register(door)
	}
}

func DoorTimeout(ctx context.Context, seconds time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, seconds)
}

func resolveDoorCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("door command is required")
	}
	if strings.Contains(command, "/") {
		abs, err := filepath.Abs(command)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	found, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}
	return found, nil
}

func validateDoorPath(commandPath string) error {
	allowed := strings.TrimSpace(os.Getenv("WOLFBBS_DOOR_ALLOW_DIR"))
	if allowed == "" {
		return nil
	}
	allowAbs, err := filepath.Abs(allowed)
	if err != nil {
		return err
	}
	cmdAbs, err := filepath.Abs(commandPath)
	if err != nil {
		return err
	}
	allowAbs = filepath.Clean(allowAbs)
	cmdAbs = filepath.Clean(cmdAbs)
	if cmdAbs == allowAbs {
		return nil
	}
	prefix := allowAbs + string(filepath.Separator)
	if !strings.HasPrefix(cmdAbs, prefix) {
		return fmt.Errorf("door command %q is outside allow directory %q", cmdAbs, allowAbs)
	}
	return nil
}
