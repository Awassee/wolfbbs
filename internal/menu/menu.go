package menu

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	ErrInvalidMenu    = errors.New("invalid menu")
	ErrModuleNotFound = errors.New("menu module not found")
)

type Entry struct {
	Hotkey string `json:"hotkey"`
	Label  string `json:"label"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	ACS    string `json:"acs,omitempty"`
}

type Screen struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Help    string  `json:"help,omitempty"`
	Footer  string  `json:"footer,omitempty"`
	Entries []Entry `json:"entries"`
}

func (s Screen) EntryByHotkey(key string) (Entry, bool) {
	key = strings.ToUpper(strings.TrimSpace(key))
	for _, entry := range s.Entries {
		if strings.ToUpper(strings.TrimSpace(entry.Hotkey)) == key {
			return entry, true
		}
	}
	return Entry{}, false
}

func LoadHJSON(path string) (Screen, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Screen{}, err
	}
	return ParseHJSON(data)
}

type ModuleContext struct {
	User      string
	SessionID string
	Args      map[string]string
}

type Module interface {
	Run(ctx ModuleContext, entry Entry) error
}

type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module
}

func NewRegistry() *Registry {
	return &Registry{
		modules: map[string]Module{},
	}
}

func (r *Registry) Register(name string, mod Module) error {
	name = normalizeAction(name)
	if name == "" || mod == nil {
		return fmt.Errorf("%w: module name and implementation are required", ErrInvalidMenu)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("%w: duplicate module %q", ErrInvalidMenu, name)
	}
	r.modules[name] = mod
	return nil
}

func (r *Registry) Execute(action string, ctx ModuleContext, entry Entry) error {
	action = normalizeAction(action)
	r.mu.RLock()
	mod, ok := r.modules[action]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrModuleNotFound, action)
	}
	return mod.Run(ctx, entry)
}

func normalizeAction(action string) string {
	return strings.ToLower(strings.TrimSpace(action))
}
