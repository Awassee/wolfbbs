package session

import (
	"testing"
	"time"
)

func TestSessionLifecycleAndNodeReuse(t *testing.T) {
	mgr := NewManager(2, 4)

	first, err := mgr.Start("s1", "alice", "127.0.0.1")
	if err != nil {
		t.Fatalf("start first: %v", err)
	}
	if first.NodeID != 1 {
		t.Fatalf("expected node 1, got %d", first.NodeID)
	}

	second, err := mgr.Start("s2", "bob", "127.0.0.2")
	if err != nil {
		t.Fatalf("start second: %v", err)
	}
	if second.NodeID != 2 {
		t.Fatalf("expected node 2, got %d", second.NodeID)
	}

	if _, err := mgr.Start("s3", "carol", "127.0.0.3"); err == nil {
		t.Fatal("expected no free nodes error")
	}

	mgr.End("s1")
	third, err := mgr.Start("s3", "carol", "127.0.0.3")
	if err != nil {
		t.Fatalf("start third after end: %v", err)
	}
	if third.NodeID != 1 {
		t.Fatalf("expected node reuse to 1, got %d", third.NodeID)
	}
}

func TestSettersAndSnapshots(t *testing.T) {
	mgr := NewManager(3, 8)
	_, err := mgr.Start("s1", "alice", "10.0.0.1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	mgr.SetArea("s1", "Doors")
	mgr.SetUser("s1", "alice2")
	time.Sleep(10 * time.Millisecond)
	online := mgr.Online()
	if len(online) != 1 {
		t.Fatalf("expected 1 online, got %d", len(online))
	}
	row := online[0]
	if row.Username != "alice2" {
		t.Fatalf("expected updated user, got %+v", row)
	}
	if row.Area != "Doors" {
		t.Fatalf("expected updated area, got %+v", row)
	}
	if row.IdleSeconds < 0 {
		t.Fatalf("expected non-negative idle, got %+v", row)
	}
	if snap, ok := mgr.Get("s1"); !ok || snap.Username != "alice2" || snap.Area != "Doors" {
		t.Fatalf("unexpected snapshot from Get: %+v ok=%t", snap, ok)
	}
}

func TestLastCallersLimit(t *testing.T) {
	mgr := NewManager(1, 2)
	_, _ = mgr.Start("s1", "a", "1")
	mgr.End("s1")
	_, _ = mgr.Start("s2", "b", "2")
	mgr.End("s2")
	_, _ = mgr.Start("s3", "c", "3")
	mgr.End("s3")

	callers := mgr.LastCallers(10)
	if len(callers) != 2 {
		t.Fatalf("expected max callers cap 2, got %d", len(callers))
	}
	if callers[0].Username != "c" || callers[1].Username != "b" {
		t.Fatalf("unexpected caller order: %+v", callers)
	}
}

func TestReconnectNoticeCapturedOnlyForUnexpectedDisconnects(t *testing.T) {
	mgr := NewManager(2, 4)
	if _, err := mgr.Start("s1", "alice", "127.0.0.1"); err != nil {
		t.Fatalf("start alice: %v", err)
	}
	mgr.SetArea("s1", "Chat Rooms")
	mgr.End("s1")

	notice, ok := mgr.TakeReconnectNotice("alice")
	if !ok {
		t.Fatal("expected reconnect notice for unexpected disconnect")
	}
	if notice.Area != "Chat Rooms" {
		t.Fatalf("expected area Chat Rooms, got %+v", notice)
	}
	if _, ok := mgr.TakeReconnectNotice("alice"); ok {
		t.Fatal("expected reconnect notice to be consumed once")
	}

	if _, err := mgr.Start("s2", "bob", "127.0.0.2"); err != nil {
		t.Fatalf("start bob: %v", err)
	}
	mgr.MarkCleanExit("s2")
	mgr.End("s2")
	if _, ok := mgr.TakeReconnectNotice("bob"); ok {
		t.Fatal("did not expect reconnect notice for clean sign-off")
	}
}
