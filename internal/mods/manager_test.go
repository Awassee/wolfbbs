package mods

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testMod struct {
	id        string
	desc      string
	initErr   error
	tickErr   error
	initCount int
	tickCount int
	stopCount int
}

func (m *testMod) ID() string          { return m.id }
func (m *testMod) Description() string { return m.desc }
func (m *testMod) Init(context.Context) error {
	m.initCount++
	return m.initErr
}
func (m *testMod) Tick(context.Context, time.Time) error {
	m.tickCount++
	return m.tickErr
}
func (m *testMod) Shutdown(context.Context) error {
	m.stopCount++
	return nil
}
func (m *testMod) Snapshot() map[string]string {
	return map[string]string{"ticks": "ok"}
}

func TestManagerLifecycle(t *testing.T) {
	mgr := NewManager(20 * time.Millisecond)
	mod := &testMod{id: "onelinerz", desc: "test"}
	if err := mgr.Register(mod, true); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if mod.initCount != 1 {
		t.Fatalf("expected 1 init call, got %d", mod.initCount)
	}
	if mod.tickCount == 0 {
		t.Fatal("expected tick calls")
	}
	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if mod.stopCount != 1 {
		t.Fatalf("expected 1 shutdown call, got %d", mod.stopCount)
	}
	snap := mgr.Snapshot()
	if len(snap) != 1 || snap[0].ID != "onelinerz" {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}
}

func TestManagerCapturesTickError(t *testing.T) {
	mgr := NewManager(20 * time.Millisecond)
	mod := &testMod{id: "rumorz", desc: "test", tickErr: errors.New("boom")}
	if err := mgr.Register(mod, true); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	_ = mgr.Stop(context.Background())
	snap := mgr.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one snapshot row, got %d", len(snap))
	}
	if snap[0].LastError == "" {
		t.Fatal("expected last error to be recorded")
	}
}

func TestBuiltinsState(t *testing.T) {
	one := NewOneLinerzMod(2)
	one.Add("alice", "first")
	one.Add("bob", "second")
	one.Add("carol", "third")
	lines := one.List(10)
	if len(lines) != 2 {
		t.Fatalf("expected max rows enforced, got %d", len(lines))
	}
	if lines[0].Handle != "carol" {
		t.Fatalf("expected newest first, got %s", lines[0].Handle)
	}

	rumor := NewRumorzMod([]string{"a", "b"})
	if err := rumor.Init(context.Background()); err != nil {
		t.Fatalf("init rumor: %v", err)
	}
	first := rumor.Current()
	if err := rumor.Tick(context.Background(), time.Now().UTC()); err != nil {
		t.Fatalf("tick rumor: %v", err)
	}
	second := rumor.Current()
	if first == second {
		t.Fatal("expected rumor to rotate")
	}

	bbs := NewBBSListMod(2)
	bbs.Add("A", "a.example", 23)
	bbs.Add("B", "b.example", 2222)
	bbs.Add("C", "c.example", 23)
	rows := bbs.List(10)
	if len(rows) != 2 {
		t.Fatalf("expected capped bbs rows, got %d", len(rows))
	}

	who := NewWhoOnlineMod(func() int { return 7 })
	if who.Snapshot()["online"] != "7" {
		t.Fatalf("expected online snapshot of 7, got %q", who.Snapshot()["online"])
	}
}
