package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultRuntimeLoginServersDisabled(t *testing.T) {
	cfg := DefaultRuntime()
	if cfg.Login.Telnet.Enabled {
		t.Fatal("telnet should be disabled by default")
	}
	if cfg.Login.WebSocket.Enabled {
		t.Fatal("websocket login should be disabled by default")
	}
	if cfg.Login.WebSocketTLS.Enabled {
		t.Fatal("websocket tls login should be disabled by default")
	}
	if cfg.Login.Telnet.Listen == "" {
		t.Fatal("telnet listen default should be set")
	}
	if cfg.Login.WebSocket.Listen == "" {
		t.Fatal("websocket listen default should be set")
	}
	if cfg.Login.WebSocket.Path == "" {
		t.Fatal("websocket path default should be set")
	}
	if cfg.Login.WebSocketTLS.Listen == "" {
		t.Fatal("websocket tls listen default should be set")
	}
	if cfg.Login.WebSocketTLS.Path == "" {
		t.Fatal("websocket tls path default should be set")
	}
}

func TestLoadRuntimeHJSONAndEnvOverride(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "wolfbbs.hjson")
	body := `
{
  menu: { enabled: false, file: "menus/custom.hjson" }
  acs: { strict: true }
  content: { host: "bbs.example", gopher_listen: "127.0.0.1:7070" }
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("WOLFBBS_MENU_ENABLE", "true")
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	if !cfg.Menu.Enabled {
		t.Fatal("expected env override for menu.enabled")
	}
	if cfg.Menu.File != "menus/custom.hjson" {
		t.Fatalf("unexpected menu file %q", cfg.Menu.File)
	}
	if !cfg.ACS.Strict {
		t.Fatal("expected acs strict from file")
	}
	if cfg.Content.Host != "bbs.example" {
		t.Fatalf("unexpected content host %q", cfg.Content.Host)
	}
}

func TestLoadRuntimeValidation(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "wolfbbs.hjson")
	body := `
{
  menu: { enabled: true, file: "" }
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadRuntime(path); err == nil {
		t.Fatal("expected validation error for empty menu file")
	}
}

func TestLoadRuntimeConnectorValidation(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "wolfbbs.hjson")
	body := `
{
  connectors: {
    doorparty: { enabled: true },
    telnet_bridge: { enabled: true }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadRuntime(path); err == nil {
		t.Fatal("expected validation error for enabled connector without command")
	}

	t.Setenv("WOLFBBS_DOORPARTY_COMMAND", "/usr/bin/true")
	if _, err := LoadRuntime(path); err == nil {
		t.Fatal("expected validation error for enabled telnet bridge without command")
	}
	t.Setenv("WOLFBBS_TELNET_BRIDGE_COMMAND", "/usr/bin/true")
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("expected env override to satisfy telnet bridge requirement: %v", err)
	}
	if cfg.Connectors.DoorParty.Command != "/usr/bin/true" {
		t.Fatalf("unexpected connector command: %q", cfg.Connectors.DoorParty.Command)
	}
	if cfg.Connectors.Telnet.Command != "/usr/bin/true" {
		t.Fatalf("unexpected telnet bridge command: %q", cfg.Connectors.Telnet.Command)
	}
}

func TestLoadRuntimeLoginValidation(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "wolfbbs.hjson")
	body := `
{
  login: {
    websocket: { enabled: true, listen: "127.0.0.1:6090", path: "bad" }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadRuntime(path); err == nil {
		t.Fatal("expected websocket path validation failure")
	}

	t.Setenv("WOLFBBS_WS_PATH", "/ws-login")
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("expected env override to fix websocket path: %v", err)
	}
	if cfg.Login.WebSocket.Path != "/ws-login" {
		t.Fatalf("unexpected websocket path: %q", cfg.Login.WebSocket.Path)
	}
}

func TestLoadRuntimeWSSValidation(t *testing.T) {
	ResetCacheForTest()
	dir := t.TempDir()
	path := filepath.Join(dir, "wolfbbs.hjson")
	body := `
{
  login: {
    websocket_tls: { enabled: true, listen: "127.0.0.1:6443", path: "/ws-login" }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadRuntime(path); err == nil {
		t.Fatal("expected websocket tls cert/key validation failure")
	}

	t.Setenv("WOLFBBS_WSS_CERT", "/tmp/cert.pem")
	t.Setenv("WOLFBBS_WSS_KEY", "/tmp/key.pem")
	cfg, err := LoadRuntime(path)
	if err != nil {
		t.Fatalf("expected env override to satisfy websocket tls cert/key requirement: %v", err)
	}
	if cfg.Login.WebSocketTLS.Cert != "/tmp/cert.pem" || cfg.Login.WebSocketTLS.Key != "/tmp/key.pem" {
		t.Fatalf("unexpected websocket tls cert/key: cert=%q key=%q", cfg.Login.WebSocketTLS.Cert, cfg.Login.WebSocketTLS.Key)
	}
}
