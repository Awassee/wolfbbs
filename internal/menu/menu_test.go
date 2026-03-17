package menu

import (
	"errors"
	"testing"
)

func TestParseHJSONValid(t *testing.T) {
	menuData := []byte(`
{
  id: "main"
  title: "Main Menu"
  entries: [
    { hotkey: "M", label: "Message Boards", action: "boards.open", target: "general" }
    { hotkey: "Q", label: "Quit", action: "session.quit" }
  ]
}
`)
	screen, err := ParseHJSON(menuData)
	if err != nil {
		t.Fatalf("parse menu: %v", err)
	}
	if screen.Title != "Main Menu" {
		t.Fatalf("unexpected title: %q", screen.Title)
	}
	if len(screen.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(screen.Entries))
	}
	if entry, ok := screen.EntryByHotkey("m"); !ok || entry.Action != "boards.open" {
		t.Fatalf("hotkey lookup failed: %+v %v", entry, ok)
	}
}

func TestParseHJSONRejectsDuplicateHotkey(t *testing.T) {
	menuData := []byte(`
{
  title: "Main Menu"
  entries: [
    { hotkey: "M", label: "Messages", action: "boards.open" }
    { hotkey: "m", label: "Mail", action: "mail.open" }
  ]
}
`)
	_, err := ParseHJSON(menuData)
	if err == nil || !errors.Is(err, ErrInvalidMenu) {
		t.Fatalf("expected invalid menu error, got %v", err)
	}
}

type testModule struct {
	called bool
}

func (m *testModule) Run(ctx ModuleContext, entry Entry) error {
	m.called = ctx.User == "tester" && entry.Action == "demo.run"
	return nil
}

func TestRegistryExecute(t *testing.T) {
	reg := NewRegistry()
	mod := &testModule{}
	if err := reg.Register("demo.run", mod); err != nil {
		t.Fatalf("register module: %v", err)
	}
	entry := Entry{Hotkey: "D", Label: "Demo", Action: "demo.run"}
	if err := reg.Execute("demo.run", ModuleContext{User: "tester"}, entry); err != nil {
		t.Fatalf("execute module: %v", err)
	}
	if !mod.called {
		t.Fatal("expected module run to be called")
	}
	if err := reg.Execute("missing.action", ModuleContext{}, entry); err == nil || !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected missing module error, got %v", err)
	}
}
