package events

import "testing"

func TestBusPublishSpecificAndWildcard(t *testing.T) {
	bus := NewBus()
	seenSpecific := 0
	seenWildcard := 0
	unsubSpecific := bus.Subscribe("auth.login", func(ev Event) {
		seenSpecific++
		if ev.Name != "auth.login" {
			t.Fatalf("unexpected event name: %s", ev.Name)
		}
		if ev.Fields["user"] != "sysop" {
			t.Fatalf("missing event field")
		}
	})
	_ = bus.Subscribe("*", func(_ Event) {
		seenWildcard++
	})
	bus.Publish("auth.login", map[string]string{"user": "sysop"})
	if seenSpecific != 1 || seenWildcard != 1 {
		t.Fatalf("unexpected counts specific=%d wildcard=%d", seenSpecific, seenWildcard)
	}
	unsubSpecific()
	bus.Publish("auth.login", map[string]string{"user": "sysop"})
	if seenSpecific != 1 || seenWildcard != 2 {
		t.Fatalf("unsubscribe failed specific=%d wildcard=%d", seenSpecific, seenWildcard)
	}
}
