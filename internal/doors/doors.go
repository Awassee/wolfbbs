package doors

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	r.doors[door.Hotkey] = door
}

func (r *Registry) Doors() []Door {
	out := make([]Door, 0, len(r.doors))
	for _, d := range r.doors {
		out = append(out, d)
	}
	return out
}

func (r *Registry) Launch(ctx context.Context, hotkey string, stdin io.Reader, stdout, stderr io.Writer, env map[string]string) error {
	door, ok := r.doors[hotkey]
	if !ok {
		return fmt.Errorf("unknown door")
	}
	cmd := exec.CommandContext(ctx, door.Command, door.Args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "WOLFBBS_MODE=door")
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Run(); err != nil {
		return err
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

func DoorTimeout(ctx context.Context, seconds time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, seconds)
}
