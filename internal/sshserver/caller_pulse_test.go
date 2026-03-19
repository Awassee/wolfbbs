package sshserver

import (
	"encoding/json"
	"testing"
	"time"

	"wolfbbs/internal/repository"
)

func TestPulseDigestMaxItemsPreservesExistingFields(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: admin}
	handle := "Caller"
	key := pulseDigestPrefsSettingKey(handle)
	if key == "" {
		t.Fatal("expected digest prefs key for handle")
	}
	if err := admin.UpsertSystemSetting(key, `{"enabled":true,"attention_cadence":"daily","max_items":12}`); err != nil {
		t.Fatalf("seed digest prefs: %v", err)
	}

	if got := srv.loadPulseDigestMaxItems(handle); got != 12 {
		t.Fatalf("expected max_items=12, got %d", got)
	}
	if err := srv.persistPulseDigestMaxItems(handle, 18); err != nil {
		t.Fatalf("persist digest max items: %v", err)
	}

	raw, err := admin.GetSystemSetting(key)
	if err != nil {
		t.Fatalf("read digest prefs: %v", err)
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode digest prefs: %v", err)
	}
	if got, ok := pulseIntFromAny(payload["max_items"]); !ok || got != 18 {
		t.Fatalf("expected max_items=18, got %v", payload["max_items"])
	}
	if got, ok := payload["enabled"].(bool); !ok || !got {
		t.Fatalf("expected enabled field preserved as true, got %#v", payload["enabled"])
	}
	if got, ok := payload["attention_cadence"].(string); !ok || got != "daily" {
		t.Fatalf("expected attention_cadence preserved, got %#v", payload["attention_cadence"])
	}
}

func TestPulseWeekdayDigestPrefsRoundTrip(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: admin}
	handle := "caller"

	defaults := srv.loadPulseWeekdayDigestPrefs(handle, 12)
	if defaults.Monday != 12 || defaults.Friday != 12 {
		t.Fatalf("expected default weekday prefs to follow fallback, got %+v", defaults)
	}

	defaults.Monday = 14
	defaults.Friday = 8
	if err := srv.persistPulseWeekdayDigestPrefs(handle, defaults, 12); err != nil {
		t.Fatalf("persist weekday prefs: %v", err)
	}
	loaded := srv.loadPulseWeekdayDigestPrefs(handle, 12)
	if loaded.Monday != 14 || loaded.Friday != 8 {
		t.Fatalf("expected persisted weekday prefs, got %+v", loaded)
	}
}

func TestRecentPulseEventRecapsAcceptsMapPayload(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: admin}
	now := time.Now().UTC().Add(-2 * time.Hour)
	payload := map[string]pulseEventRecap{
		"evt-1": {
			EventID:         "evt-1",
			Title:           "Retro Net Night",
			StartsAt:        now,
			AttendanceCount: 17,
			Summary:         "Great turnout.",
			UpdatedBy:       "sysop",
			UpdatedAt:       now.Add(30 * time.Minute),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := admin.UpsertSystemSetting(pulseSettingEventRecaps, string(raw)); err != nil {
		t.Fatalf("seed recaps setting: %v", err)
	}

	rows := srv.recentPulseEventRecaps(5)
	if len(rows) != 1 {
		t.Fatalf("expected one recap row, got %d", len(rows))
	}
	if rows[0].EventID != "evt-1" || rows[0].AttendanceCount != 17 {
		t.Fatalf("unexpected recap row: %+v", rows[0])
	}
}
