package sshserver

import (
	"testing"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestLegacyHotkeyToAction(t *testing.T) {
	if got := legacyHotkeyToAction("m"); got != "boards.open" {
		t.Fatalf("expected boards.open, got %q", got)
	}
	if got := legacyHotkeyToAction("n"); got != "system.newscan" {
		t.Fatalf("expected system.newscan, got %q", got)
	}
	if got := legacyHotkeyToAction("?"); got != "" {
		t.Fatalf("expected empty action for help key, got %q", got)
	}
	if got := legacyHotkeyToAction("x"); got != "system.config_center" {
		t.Fatalf("expected system.config_center, got %q", got)
	}
	if got := legacyHotkeyToAction("y"); got != "system.status_center" {
		t.Fatalf("expected system.status_center, got %q", got)
	}
	if got := legacyHotkeyToAction("/"); got != "system.quick_jump" {
		t.Fatalf("expected system.quick_jump, got %q", got)
	}
}

func TestQuickJumpToAction(t *testing.T) {
	if got := quickJumpToAction("boards"); got != "boards.open" {
		t.Fatalf("expected boards.open, got %q", got)
	}
	if got := quickJumpToAction("status"); got != "system.status_center" {
		t.Fatalf("expected system.status_center, got %q", got)
	}
	if got := quickJumpToAction("config"); got != "system.config_center" {
		t.Fatalf("expected system.config_center, got %q", got)
	}
	if got := quickJumpToAction("events"); got != "pulse.open" {
		t.Fatalf("expected pulse.open for events alias, got %q", got)
	}
	if got := quickJumpToAction("challenges"); got != "pulse.open" {
		t.Fatalf("expected pulse.open for challenges alias, got %q", got)
	}
	if got := quickJumpToAction("digest-prefs"); got != "pulse.open" {
		t.Fatalf("expected pulse.open for digest alias, got %q", got)
	}
	if got := quickJumpToAction("bookmarks"); got != "settings.open" {
		t.Fatalf("expected settings.open for bookmarks alias, got %q", got)
	}
	if got := quickJumpToAction("circles"); got != "settings.open" {
		t.Fatalf("expected settings.open for circles alias, got %q", got)
	}
	if got := quickJumpToAction("collections"); got != "files.open" {
		t.Fatalf("expected files.open for collections alias, got %q", got)
	}
	if got := quickJumpToAction("offline"); got != "files.open" {
		t.Fatalf("expected files.open for offline alias, got %q", got)
	}
	if got := quickJumpToAction("statusz"); got != "system.status_center" {
		t.Fatalf("expected system.status_center for statusz alias, got %q", got)
	}
	if got := quickJumpToAction("showcase"); got != "system.showcase" {
		t.Fatalf("expected system.showcase for showcase alias, got %q", got)
	}
	if got := quickJumpToAction("/app upgrade"); got != "system.app_upgrade" {
		t.Fatalf("expected system.app_upgrade, got %q", got)
	}
	if got := quickJumpToAction("unknown"); got != "" {
		t.Fatalf("expected empty action for unknown jump target, got %q", got)
	}
}

func TestEvaluateAccess(t *testing.T) {
	user := &domain.User{
		Handle:   "tester",
		Role:     "moderator",
		Verified: true,
	}
	if ok := evaluateAccess("role=moderator and verified", user, "", map[string]string{"area": "boards"}, true, nil); !ok {
		t.Fatal("expected moderator verified user to pass")
	}
	if ok := evaluateAccess("role=sysop", user, "", nil, true, nil); ok {
		t.Fatal("expected moderator to fail sysop-only rule")
	}
	if ok := evaluateAccess("role=admin", user, "", nil, true, nil); ok {
		t.Fatal("expected moderator to fail admin/sysop-only alias rule")
	}
	if ok := evaluateAccess("(role=admin", user, "", nil, false, nil); !ok {
		t.Fatal("expected invalid expression to allow when strict is false")
	}
	if ok := evaluateAccess("(role=admin", user, "", nil, true, nil); ok {
		t.Fatal("expected invalid expression to deny when strict is true")
	}
}

func TestActiveWebFetchConfigUsesAdminGatewaySettings(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	if err := admin.UpsertGatewaySettings(&domain.GatewaySettings{
		WebTimeoutSec: 27,
		WebMaxBytes:   256000,
	}); err != nil {
		t.Fatalf("seed gateway settings: %v", err)
	}
	srv := &Server{admin: admin}
	cfg := srv.activeWebFetchConfig()
	if got := int(cfg.Timeout.Seconds()); got != 27 {
		t.Fatalf("expected timeout 27s, got %ds", got)
	}
	if got := int(cfg.MaxBodyBytes); got != 256000 {
		t.Fatalf("expected max body 256000, got %d", got)
	}
}

func TestLoadAIGatewaySettingsUsesSystemSettings(t *testing.T) {
	admin := repository.NewInMemoryAdminRepository()
	_ = admin.UpsertSystemSetting(sysSettingGatewayAIEnabled, "true")
	_ = admin.UpsertSystemSetting(sysSettingGatewayAIBaseURL, "https://ai.example")
	_ = admin.UpsertSystemSetting(sysSettingGatewayAIModel, "gpt-test")
	_ = admin.UpsertSystemSetting(sysSettingGatewayAIAPIKey, "sk-test")
	_ = admin.UpsertSystemSetting(sysSettingGatewayAISystemPrompt, "Stay concise.")
	_ = admin.UpsertSystemSetting(sysSettingGatewayAITimeoutSec, "45")
	_ = admin.UpsertSystemSetting(sysSettingGatewayAIMaxTokens, "700")

	srv := &Server{admin: admin}
	cfg := srv.loadAIGatewaySettings()
	if !cfg.Enabled {
		t.Fatal("expected ai gateway enabled from settings")
	}
	if cfg.BaseURL != "https://ai.example" {
		t.Fatalf("expected ai base url override, got %q", cfg.BaseURL)
	}
	if cfg.Model != "gpt-test" {
		t.Fatalf("expected ai model override, got %q", cfg.Model)
	}
	if cfg.APIKey != "sk-test" {
		t.Fatalf("expected ai api key override, got %q", cfg.APIKey)
	}
	if cfg.SystemPrompt != "Stay concise." {
		t.Fatalf("expected ai system prompt override, got %q", cfg.SystemPrompt)
	}
	if cfg.TimeoutSec != 45 {
		t.Fatalf("expected ai timeout 45, got %d", cfg.TimeoutSec)
	}
	if cfg.MaxTokens != 700 {
		t.Fatalf("expected ai max tokens 700, got %d", cfg.MaxTokens)
	}
}
