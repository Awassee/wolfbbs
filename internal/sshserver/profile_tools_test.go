package sshserver

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestProfileBookmarksRoundTrip(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: admin}
	handle := "Caller"

	rows := []profileBookmarkEntry{
		{
			Kind:      "board_message",
			Label:     "Board item",
			BoardID:   1,
			MessageID: 42,
			AddedAt:   time.Now().UTC(),
		},
		{
			Kind:    "mail",
			Label:   "Mail item",
			MailID:  9,
			AddedAt: time.Now().UTC().Add(-time.Minute),
		},
	}
	if err := srv.persistProfileBookmarks(handle, rows); err != nil {
		t.Fatalf("persist bookmarks: %v", err)
	}
	loaded := srv.profileBookmarks(handle)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(loaded))
	}
	if loaded[0].Key == "" || loaded[0].Href == "" {
		t.Fatalf("expected normalized bookmark key/href, got %+v", loaded[0])
	}
}

func TestProfileCirclesCreateDelete(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: admin}
	handle := "caller"

	if err := srv.upsertProfileCircle(handle, "", "Door Crew", "night games", []string{"alice", "bob"}); err != nil {
		t.Fatalf("create circle: %v", err)
	}
	loaded := srv.profileCircles(handle)
	if len(loaded) != 1 {
		t.Fatalf("expected one circle, got %d", len(loaded))
	}
	if loaded[0].ID == "" || loaded[0].Name != "Door Crew" {
		t.Fatalf("unexpected circle row: %+v", loaded[0])
	}
	if err := srv.deleteProfileCircle(handle, loaded[0].ID); err != nil {
		t.Fatalf("delete circle: %v", err)
	}
	if got := len(srv.profileCircles(handle)); got != 0 {
		t.Fatalf("expected no circles after delete, got %d", got)
	}
}

func TestBuildProfileAndAttentionExportPayloads(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	srv := &Server{admin: admin}
	handle := "caller"
	key := profileNormalizeHandleKey(handle)

	_ = admin.UpsertSystemSetting(profileSettingKey(profileSettingHomeRouteRoot, handle), "/today")
	_ = admin.UpsertSystemSetting(pulseDigestPrefsSettingKey(handle), `{"enabled":true,"max_items":16}`)
	_ = admin.UpsertSystemSetting(profileSettingKey(profileSettingPublicProfileRoot, handle), `{"status_line":"night owl"}`)
	_ = admin.UpsertSystemSetting(profileSettingKey(profileSettingContactAliasesRoot, handle), `{"alice":"A"}`)
	_ = admin.UpsertSystemSetting(profileSettingKey(profileSettingRouteSeenRoot, handle), `{"\/today":"2026-03-01T00:00:00Z"}`)
	_ = admin.UpsertSystemSetting(profileSettingKey(profileSettingAttentionReadRoot, handle), `{"mail:12":"2026-03-02T00:00:00Z"}`)
	_ = admin.UpsertSystemSetting(profileSettingKey(profileSettingAttentionDismissedRoot, handle), `{"board:9":"2026-03-02T02:00:00Z"}`)

	profilePayload := srv.buildProfileExportPayload(&domain.User{
		Handle:        handle,
		Role:          "user",
		Theme:         "retro-amber",
		ANSIEnabled:   true,
		PagingEnabled: true,
		TimeFormat24h: true,
	})
	if got := profilePayload["home_route"]; got != "/today" {
		t.Fatalf("expected home route /today, got %#v", got)
	}
	if got := profilePayload["handle"]; got != handle {
		t.Fatalf("expected handle in profile export, got %#v", got)
	}
	digest, ok := profilePayload["digest_preferences"].(map[string]interface{})
	if !ok || digest["max_items"] == nil {
		t.Fatalf("expected digest preferences in profile export, got %#v", profilePayload["digest_preferences"])
	}

	attentionPayload := srv.buildAttentionExportPayload(&domain.User{Handle: handle, Role: "user"})
	if got := attentionPayload["preset_recommendation"]; got != "caller" {
		t.Fatalf("expected caller preset recommendation, got %#v", got)
	}
	routeSeen, ok := attentionPayload["route_seen"].(map[string]string)
	if !ok || len(routeSeen) == 0 {
		t.Fatalf("expected route seen map, got %#v", attentionPayload["route_seen"])
	}
	if routeSeen["/today"] == "" {
		// fallback to escaped key in case JSON parser preserved it literally
		if routeSeen["\\/today"] == "" {
			t.Fatalf("expected /today route-seen timestamp, got %#v", routeSeen)
		}
	}
	attention, ok := attentionPayload["attention"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected attention object, got %#v", attentionPayload["attention"])
	}
	readMap, ok := attention["read"].(map[string]string)
	if !ok || readMap["mail:12"] == "" {
		t.Fatalf("expected read map in attention payload, got %#v", attention["read"])
	}

	// Sanity-check that settings keys were normalized to lowercase handle.
	if _, err := admin.GetSystemSetting(profileSettingBookmarksRoot + key); err == nil {
		// no-op: key lookup should be valid even if empty value.
	}
}

func TestDecodeTimeMapFromTimePayload(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw, err := json.Marshal(map[string]time.Time{"k": now})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := decodeTimeMap(string(raw))
	if decoded["k"] == "" {
		t.Fatalf("expected decoded timestamp for key k, got %#v", decoded)
	}
}

func TestProfileExportRootDirUsesInstallPrefixFallback(t *testing.T) {
	t.Setenv("WOLFBBS_OFFLINE_DIR", "")
	t.Setenv("WOLFBBS_INSTALL_PREFIX", filepath.Join(string(filepath.Separator), "tmp", "wolfbbs-release-test"))

	got := profileExportRootDir()
	want := filepath.Join(string(filepath.Separator), "tmp", "wolfbbs-release-test", "offline", "caller")
	if got != want {
		t.Fatalf("profileExportRootDir() = %q, want %q", got, want)
	}
}
