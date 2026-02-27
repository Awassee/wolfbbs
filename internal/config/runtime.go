package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	hjson "github.com/hjson/hjson-go/v4"
)

type Runtime struct {
	Menu        MenuConfig        `json:"menu"`
	ACS         ACSConfig         `json:"acs"`
	Content     ContentConfig     `json:"content"`
	ActivityPub ActivityPubConfig `json:"activitypub"`
	Connectors  ConnectorConfig   `json:"connectors"`
	Login       LoginConfig       `json:"login"`
}

type MenuConfig struct {
	Enabled bool   `json:"enabled"`
	File    string `json:"file"`
}

type ACSConfig struct {
	Strict bool `json:"strict"`
}

type ContentConfig struct {
	Host         string `json:"host"`
	GopherListen string `json:"gopher_listen"`
	NNTPListen   string `json:"nntp_listen"`
	NNTPSListen  string `json:"nntps_listen"`
	NNTPSCert    string `json:"nntps_cert"`
	NNTPSKey     string `json:"nntps_key"`
}

type ActivityPubConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
}

type ConnectorProgram struct {
	Enabled bool   `json:"enabled"`
	Command string `json:"command"`
	Args    string `json:"args"`
}

type ConnectorConfig struct {
	DoorParty ConnectorProgram `json:"doorparty"`
	BBSLink   ConnectorProgram `json:"bbslink"`
	Telnet    ConnectorProgram `json:"telnet_bridge"`
}

type ListenerConfig struct {
	Enabled bool   `json:"enabled"`
	Listen  string `json:"listen"`
}

type WebSocketLoginConfig struct {
	Enabled bool   `json:"enabled"`
	Listen  string `json:"listen"`
	Path    string `json:"path"`
}

type LoginConfig struct {
	Telnet         ListenerConfig       `json:"telnet"`
	WebSocket      WebSocketLoginConfig `json:"websocket"`
	WebSocketTLS   WebSocketTLSConfig   `json:"websocket_tls"`
	TrustedProxies string               `json:"trusted_proxies"`
}

type WebSocketTLSConfig struct {
	Enabled bool   `json:"enabled"`
	Listen  string `json:"listen"`
	Path    string `json:"path"`
	Cert    string `json:"cert"`
	Key     string `json:"key"`
}

func DefaultRuntime() Runtime {
	return Runtime{
		Menu: MenuConfig{
			Enabled: true,
			File:    "menus/main.hjson",
		},
		ACS: ACSConfig{
			Strict: false,
		},
		Content: ContentConfig{
			Host: "localhost",
		},
		ActivityPub: ActivityPubConfig{
			Enabled: false,
		},
		Login: LoginConfig{
			Telnet: ListenerConfig{
				Enabled: false,
				Listen:  ":2323",
			},
			WebSocket: WebSocketLoginConfig{
				Enabled: false,
				Listen:  ":6080",
				Path:    "/ws-login",
			},
			WebSocketTLS: WebSocketTLSConfig{
				Enabled: false,
				Listen:  ":6443",
				Path:    "/ws-login",
			},
			TrustedProxies: "",
		},
	}
}

func ResolveConfigPath() string {
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_CONFIG_FILE")); raw != "" {
		return raw
	}
	for _, candidate := range []string{"wolfbbs.hjson", "config/wolfbbs.hjson"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func LoadRuntime(path string) (Runtime, error) {
	cfg := DefaultRuntime()
	path = strings.TrimSpace(path)
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := hjson.Unmarshal(body, &cfg); err != nil {
			return cfg, err
		}
	}
	applyEnvOverrides(&cfg)
	if err := validate(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func LoadRuntimeFromEnv() (Runtime, error) {
	return LoadRuntime(ResolveConfigPath())
}

var (
	cacheOnce sync.Once
	cacheCfg  Runtime
	cacheErr  error
)

func CachedRuntime() (Runtime, error) {
	cacheOnce.Do(func() {
		cacheCfg, cacheErr = LoadRuntimeFromEnv()
	})
	return cacheCfg, cacheErr
}

func ResetCacheForTest() {
	cacheOnce = sync.Once{}
	cacheCfg = Runtime{}
	cacheErr = nil
}

func applyEnvOverrides(cfg *Runtime) {
	if cfg == nil {
		return
	}
	if raw, ok := envBool("WOLFBBS_MENU_ENABLE"); ok {
		cfg.Menu.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_MENU_FILE")); raw != "" {
		cfg.Menu.File = raw
	}
	if raw, ok := envBool("WOLFBBS_ACS_STRICT"); ok {
		cfg.ACS.Strict = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_CONTENT_HOST")); raw != "" {
		cfg.Content.Host = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_GOPHER_LISTEN")); raw != "" {
		cfg.Content.GopherListen = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_NNTP_LISTEN")); raw != "" {
		cfg.Content.NNTPListen = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_NNTPS_LISTEN")); raw != "" {
		cfg.Content.NNTPSListen = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_NNTPS_CERT")); raw != "" {
		cfg.Content.NNTPSCert = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_NNTPS_KEY")); raw != "" {
		cfg.Content.NNTPSKey = raw
	}
	if raw, ok := envBool("WOLFBBS_ACTIVITYPUB_ENABLE"); ok {
		cfg.ActivityPub.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_ACTIVITYPUB_BASE_URL")); raw != "" {
		cfg.ActivityPub.BaseURL = raw
	}
	if raw, ok := envBool("WOLFBBS_DOORPARTY_ENABLE"); ok {
		cfg.Connectors.DoorParty.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_DOORPARTY_COMMAND")); raw != "" {
		cfg.Connectors.DoorParty.Command = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_DOORPARTY_ARGS")); raw != "" {
		cfg.Connectors.DoorParty.Args = raw
	}
	if raw, ok := envBool("WOLFBBS_BBSLINK_ENABLE"); ok {
		cfg.Connectors.BBSLink.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_BBSLINK_COMMAND")); raw != "" {
		cfg.Connectors.BBSLink.Command = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_BBSLINK_ARGS")); raw != "" {
		cfg.Connectors.BBSLink.Args = raw
	}
	if raw, ok := envBool("WOLFBBS_TELNET_BRIDGE_ENABLE"); ok {
		cfg.Connectors.Telnet.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_TELNET_BRIDGE_COMMAND")); raw != "" {
		cfg.Connectors.Telnet.Command = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_TELNET_BRIDGE_ARGS")); raw != "" {
		cfg.Connectors.Telnet.Args = raw
	}
	if raw, ok := envBool("WOLFBBS_TELNET_ENABLE"); ok {
		cfg.Login.Telnet.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_TELNET_LISTEN")); raw != "" {
		cfg.Login.Telnet.Listen = raw
	}
	if raw, ok := envBool("WOLFBBS_WS_ENABLE"); ok {
		cfg.Login.WebSocket.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_WS_LISTEN")); raw != "" {
		cfg.Login.WebSocket.Listen = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_WS_PATH")); raw != "" {
		cfg.Login.WebSocket.Path = raw
	}
	if raw, ok := envBool("WOLFBBS_WSS_ENABLE"); ok {
		cfg.Login.WebSocketTLS.Enabled = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_WSS_LISTEN")); raw != "" {
		cfg.Login.WebSocketTLS.Listen = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_WSS_PATH")); raw != "" {
		cfg.Login.WebSocketTLS.Path = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_WSS_CERT")); raw != "" {
		cfg.Login.WebSocketTLS.Cert = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_WSS_KEY")); raw != "" {
		cfg.Login.WebSocketTLS.Key = raw
	}
	if raw := strings.TrimSpace(os.Getenv("WOLFBBS_TRUSTED_PROXIES")); raw != "" {
		cfg.Login.TrustedProxies = raw
	}
}

func validate(cfg *Runtime) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	if cfg.Menu.Enabled && strings.TrimSpace(cfg.Menu.File) == "" {
		return errors.New("menu.file is required when menu.enabled is true")
	}
	if err := validateListen(cfg.Content.GopherListen); err != nil {
		return errors.New("content.gopher_listen: " + err.Error())
	}
	if err := validateListen(cfg.Content.NNTPListen); err != nil {
		return errors.New("content.nntp_listen: " + err.Error())
	}
	if err := validateListen(cfg.Content.NNTPSListen); err != nil {
		return errors.New("content.nntps_listen: " + err.Error())
	}
	if strings.TrimSpace(cfg.Content.NNTPSListen) != "" {
		if strings.TrimSpace(cfg.Content.NNTPSCert) == "" || strings.TrimSpace(cfg.Content.NNTPSKey) == "" {
			return errors.New("content.nntps_cert and content.nntps_key are required when nntps is enabled")
		}
	}
	if cfg.Connectors.DoorParty.Enabled && strings.TrimSpace(cfg.Connectors.DoorParty.Command) == "" {
		return errors.New("connectors.doorparty.command is required when enabled")
	}
	if cfg.Connectors.BBSLink.Enabled && strings.TrimSpace(cfg.Connectors.BBSLink.Command) == "" {
		return errors.New("connectors.bbslink.command is required when enabled")
	}
	if cfg.Connectors.Telnet.Enabled && strings.TrimSpace(cfg.Connectors.Telnet.Command) == "" {
		return errors.New("connectors.telnet_bridge.command is required when enabled")
	}
	if cfg.Login.Telnet.Enabled {
		if err := validateListen(cfg.Login.Telnet.Listen); err != nil {
			return errors.New("login.telnet.listen: " + err.Error())
		}
	}
	if cfg.Login.WebSocket.Enabled {
		if err := validateListen(cfg.Login.WebSocket.Listen); err != nil {
			return errors.New("login.websocket.listen: " + err.Error())
		}
		path := strings.TrimSpace(cfg.Login.WebSocket.Path)
		if path == "" {
			return errors.New("login.websocket.path is required when websocket login is enabled")
		}
		if !strings.HasPrefix(path, "/") {
			return errors.New("login.websocket.path must start with /")
		}
	}
	if cfg.Login.WebSocketTLS.Enabled {
		if err := validateListen(cfg.Login.WebSocketTLS.Listen); err != nil {
			return errors.New("login.websocket_tls.listen: " + err.Error())
		}
		path := strings.TrimSpace(cfg.Login.WebSocketTLS.Path)
		if path == "" {
			return errors.New("login.websocket_tls.path is required when websocket tls login is enabled")
		}
		if !strings.HasPrefix(path, "/") {
			return errors.New("login.websocket_tls.path must start with /")
		}
		if strings.TrimSpace(cfg.Login.WebSocketTLS.Cert) == "" || strings.TrimSpace(cfg.Login.WebSocketTLS.Key) == "" {
			return errors.New("login.websocket_tls.cert and login.websocket_tls.key are required when websocket tls login is enabled")
		}
	}
	cfg.Menu.File = filepath.Clean(cfg.Menu.File)
	cfg.Content.Host = strings.TrimSpace(cfg.Content.Host)
	if cfg.Content.Host == "" {
		cfg.Content.Host = "localhost"
	}
	cfg.Login.WebSocket.Path = strings.TrimSpace(cfg.Login.WebSocket.Path)
	cfg.Login.WebSocketTLS.Path = strings.TrimSpace(cfg.Login.WebSocketTLS.Path)
	return nil
}

func validateListen(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	_, _, err := net.SplitHostPort(value)
	if err != nil {
		return err
	}
	return nil
}

func envBool(name string) (bool, bool) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return false, false
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
