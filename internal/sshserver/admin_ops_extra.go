package sshserver

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/ui"
)

const (
	adminSettingSiteHostname        = "site.hostname"
	adminSettingSiteMOTD            = "site.motd"
	adminSettingSiteAnnouncement    = "site.announcement"
	adminSettingSiteReadOnly        = "site.read_only"
	adminSettingSiteSecureCookie    = "site.secure_cookie"
	adminSettingSiteWebOnRamp       = "site.web_onramp_enable"
	adminSettingSiteGuestTour       = "site.guest_tour_enable"
	adminSettingSiteDiscover        = "site.discover_enable"
	adminSettingSiteQuickJump       = "site.quick_jump_enable"
	adminSettingSiteClassicSearch   = "site.classic_search_enable"
	adminSettingMailRequireVerified = "mail.require_verified"
	adminSettingLockedChannels      = "chat.locked_channels"
	defaultInstallPrefixMacOS       = ".local/share/wolfbbs"
	defaultAdminArtifactListLimit   = 6
)

type adminArtifactRow struct {
	Name      string
	Path      string
	UpdatedAt time.Time
}

func (s *Server) siteHostname() string {
	return s.textFromConfig(adminSettingSiteHostname, "WOLFBBS_HOSTNAME", "localhost")
}

func (s *Server) readOnlyModeEnabled() bool {
	return s.flagFromConfig(adminSettingSiteReadOnly, "WOLFBBS_READ_ONLY", false)
}

func (s *Server) secureCookieEnabled() bool {
	return s.flagFromConfig(adminSettingSiteSecureCookie, "WOLFBBS_SECURE_COOKIE", false)
}

func (s *Server) webOnRampEnabled() bool {
	return s.flagFromConfig(adminSettingSiteWebOnRamp, "WOLFBBS_WEB_ONRAMP", true)
}

func (s *Server) motdText() string {
	return s.textFromConfig(adminSettingSiteMOTD, "WOLFBBS_MOTD", "")
}

func (s *Server) announcementText() string {
	return s.textFromConfig(adminSettingSiteAnnouncement, "WOLFBBS_ANNOUNCEMENT", "")
}

func (s *Server) persistAdminSetting(key, value string) error {
	if s == nil || s.admin == nil {
		return fmt.Errorf("admin repository unavailable")
	}
	return s.admin.UpsertSystemSetting(strings.TrimSpace(key), strings.TrimSpace(value))
}

func (s *Server) toggleAdminSetting(key string, current bool) (bool, error) {
	next := !current
	if err := s.persistAdminSetting(key, strconv.FormatBool(next)); err != nil {
		return current, err
	}
	return next, nil
}

func ensureMailbotServiceUser(authSvc *auth.Service) {
	const serviceHandle = "mailbot"
	if authSvc == nil {
		return
	}
	existing, err := authSvc.GetUser(serviceHandle)
	if err == nil && existing != nil {
		_ = authSvc.SetEnabled(serviceHandle, false)
		_ = authSvc.SetVerified(serviceHandle, true)
		_ = authSvc.SetRole(serviceHandle, rbac.RoleUser)
		return
	}
	password, err := randomTokenHex(20)
	if err != nil {
		password = fmt.Sprintf("mailbot-%d", time.Now().UnixNano())
	}
	if _, err := authSvc.Register(serviceHandle, password); err != nil {
		return
	}
	_ = authSvc.SetEnabled(serviceHandle, false)
	_ = authSvc.SetVerified(serviceHandle, true)
	_ = authSvc.SetRole(serviceHandle, rbac.RoleUser)
}

func seedDefaultBoardsSSH(repo interface {
	Create(board *domain.Board) error
	List() ([]domain.Board, error)
}) (int, error) {
	if repo == nil {
		return 0, fmt.Errorf("board repository is required")
	}
	boards, err := repo.List()
	if err != nil {
		return 0, err
	}
	existing := make(map[string]struct{}, len(boards))
	for _, board := range boards {
		name := strings.ToLower(strings.TrimSpace(board.Name))
		if name == "" {
			continue
		}
		existing[name] = struct{}{}
	}
	seed := []domain.Board{
		{Name: "General", Description: "General system discussion", CreatedBy: 1},
		{Name: "Node Talk", Description: "Node status and operator chat", CreatedBy: 1},
		{Name: "Tooling", Description: "Build scripts and deployment", CreatedBy: 1},
	}
	created := 0
	for i := range seed {
		name := strings.ToLower(strings.TrimSpace(seed[i].Name))
		if _, ok := existing[name]; ok {
			continue
		}
		if err := repo.Create(&seed[i]); err != nil {
			return created, err
		}
		existing[name] = struct{}{}
		created++
	}
	return created, nil
}

func (s *Server) runAdminConfigSetup(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Admin settings are unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		users := []domain.User{}
		if s.auth != nil {
			users, _ = s.auth.ListUsers()
		}
		mailbotReady := false
		for _, row := range users {
			if strings.EqualFold(strings.TrimSpace(row.Handle), "mailbot") && !row.Enabled && row.Verified {
				mailbotReady = true
				break
			}
		}
		boardCount := 0
		if s.boards != nil {
			if boards, err := s.boards.List(); err == nil {
				boardCount = len(boards)
			}
		}
		lines := []string{
			fmt.Sprintf("Site: %s @ %s", s.siteName(), s.siteHostname()),
			"MOTD: " + defaultIfBlank(clampForTTY(s.motdText(), renderWidth-8), "(blank)"),
			"Announcement: " + defaultIfBlank(clampForTTY(s.announcementText(), renderWidth-15), "(blank)"),
			"",
			fmt.Sprintf("read_only=%s secure_cookie=%s require_verified=%s", boolText(s.readOnlyModeEnabled()), boolText(s.secureCookieEnabled()), boolText(s.requireVerifiedEmail())),
			fmt.Sprintf("web_onramp=%s guest_tour=%s discover=%s quick_jump=%s classic_search=%s", boolText(s.webOnRampEnabled()), boolText(s.guestTourEnabled()), boolText(s.discoverEnabled()), boolText(s.quickJumpEnabled()), boolText(s.classicSearchEnabled())),
			fmt.Sprintf("Boards seeded: %d   Mailbot ready: %s", boardCount, boolText(mailbotReady)),
			"",
			"SITE <name>|<hostname>",
			"TEXT <motd>|<announcement>",
			"TOGGLE <read_only|secure_cookie|require_verified|web_onramp|guest_tour|discover|quick_jump|classic_search>",
			"SEED BOARDS | ENSURE MAILBOT | Q",
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Config + Setup", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Config + Setup", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 512)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if isBackCommand(cmd) {
			return
		}
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "SITE "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("SITE "):]), 2)
			name := defaultIfBlank(strings.TrimSpace(parts[0]), "WolfBBS")
			host := defaultIfBlank(strings.TrimSpace(parts[1]), "localhost")
			if err := s.persistAdminSetting("site.name", name); err != nil {
				adminPause(sess, reader, touch, "Could not save site name: "+err.Error())
				continue
			}
			if err := s.persistAdminSetting(adminSettingSiteHostname, host); err != nil {
				adminPause(sess, reader, touch, "Could not save site hostname: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "config", "save_identity", "site="+name+" host="+host)
			adminPause(sess, reader, touch, "Updated site identity.")
		case strings.HasPrefix(upper, "TEXT "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("TEXT "):]), 2)
			if err := s.persistAdminSetting(adminSettingSiteMOTD, strings.TrimSpace(parts[0])); err != nil {
				adminPause(sess, reader, touch, "Could not save MOTD: "+err.Error())
				continue
			}
			if err := s.persistAdminSetting(adminSettingSiteAnnouncement, strings.TrimSpace(parts[1])); err != nil {
				adminPause(sess, reader, touch, "Could not save announcement: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "config", "save_site_text", "")
			adminPause(sess, reader, touch, "Updated MOTD and announcement.")
		case strings.HasPrefix(upper, "TOGGLE "):
			target := strings.ToLower(strings.TrimSpace(cmd[len("TOGGLE "):]))
			var (
				current bool
				key     string
			)
			switch target {
			case "read_only":
				current, key = s.readOnlyModeEnabled(), adminSettingSiteReadOnly
			case "secure_cookie":
				current, key = s.secureCookieEnabled(), adminSettingSiteSecureCookie
			case "require_verified":
				current, key = s.requireVerifiedEmail(), adminSettingMailRequireVerified
			case "web_onramp":
				current, key = s.webOnRampEnabled(), adminSettingSiteWebOnRamp
			case "guest_tour":
				current, key = s.guestTourEnabled(), adminSettingSiteGuestTour
			case "discover":
				current, key = s.discoverEnabled(), adminSettingSiteDiscover
			case "quick_jump":
				current, key = s.quickJumpEnabled(), adminSettingSiteQuickJump
			case "classic_search":
				current, key = s.classicSearchEnabled(), adminSettingSiteClassicSearch
			default:
				adminPause(sess, reader, touch, "Unknown toggle target.")
				continue
			}
			next, err := s.toggleAdminSetting(key, current)
			if err != nil {
				adminPause(sess, reader, touch, "Toggle failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "config", "toggle_setting", target+"="+boolText(next))
			adminPause(sess, reader, touch, "Updated "+target+" to "+boolText(next)+".")
		case strings.EqualFold(cmd, "SEED BOARDS"):
			if s.boards == nil {
				adminPause(sess, reader, touch, "Board repository is unavailable.")
				continue
			}
			created, err := seedDefaultBoardsSSH(s.boards)
			if err != nil {
				adminPause(sess, reader, touch, "Board seed failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "setup", "seed_default_boards", fmt.Sprintf("seeded=%d", created))
			adminPause(sess, reader, touch, fmt.Sprintf("Seeded %d default board(s).", created))
		case strings.EqualFold(cmd, "ENSURE MAILBOT"):
			if s.auth == nil {
				adminPause(sess, reader, touch, "Auth service is unavailable.")
				continue
			}
			ensureMailbotServiceUser(s.auth)
			recordAudit(s.admin, actor, "setup", "ensure_mailbot", "")
			adminPause(sess, reader, touch, "Mailbot service account checked.")
		default:
			adminPause(sess, reader, touch, "Use SITE, TEXT, TOGGLE, SEED BOARDS, ENSURE MAILBOT, or Q.")
		}
	}
}

func (s *Server) effectiveGatewayConfig() gateway.EmailConfig {
	cfg := gateway.LoadEmailConfigFromEnv()
	if s != nil && s.admin != nil {
		if settings, err := s.admin.GetGatewaySettings(); err == nil && settings != nil {
			if host := strings.TrimSpace(settings.SMTPHost); host != "" {
				cfg.Host = host
			}
			if settings.SMTPPort > 0 {
				cfg.Port = settings.SMTPPort
			}
			if user := strings.TrimSpace(settings.SMTPUser); user != "" {
				cfg.User = user
			}
			if pass := strings.TrimSpace(settings.SMTPPass); pass != "" {
				cfg.Pass = pass
			}
			if fromDomain := strings.TrimSpace(settings.FromDomain); fromDomain != "" {
				cfg.FromDomain = fromDomain
			}
			if settings.MaxRecipients > 0 {
				cfg.MaxRecipients = settings.MaxRecipients
			}
			if settings.MaxMessageBytes > 0 {
				cfg.MaxMessageBytes = settings.MaxMessageBytes
			}
		}
	}
	cfg.MaxRecipients = clampInt(cfg.MaxRecipients, 1, 25)
	cfg.MaxMessageBytes = clampInt(cfg.MaxMessageBytes, 1024, 2*1024*1024)
	return cfg
}

func (s *Server) runAdminGatewayDeck(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.admin == nil {
		adminPause(sess, reader, touch, "Gateway settings are unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		cfg := s.effectiveGatewayConfig()
		webCfg := s.activeWebFetchConfig()
		aiCfg := s.loadAIGatewaySettings()
		lines := []string{
			fmt.Sprintf("SMTP: %s:%d user=%s from=%s", defaultIfBlank(cfg.Host, "(unset)"), cfg.Port, defaultIfBlank(cfg.User, "(unset)"), defaultIfBlank(cfg.FromDomain, "(unset)")),
			fmt.Sprintf("Mail limits: recipients=%d bytes=%d require_verified=%s", cfg.MaxRecipients, cfg.MaxMessageBytes, boolText(s.requireVerifiedEmail())),
			fmt.Sprintf("Web gateway: timeout=%ss max_bytes=%d", strconv.Itoa(int(webCfg.Timeout.Seconds())), webCfg.MaxBodyBytes),
			fmt.Sprintf("AI: enabled=%s base=%s model=%s timeout=%ds max_tokens=%d key=%s", boolText(aiCfg.Enabled), clampForTTY(aiCfg.BaseURL, 16), clampForTTY(aiCfg.Model, 16), aiCfg.TimeoutSec, aiCfg.MaxTokens, boolText(strings.TrimSpace(aiCfg.APIKey) != "")),
			"",
			"SMTP <host>|<port>|<user>|<from_domain>|<max_recipients>|<max_bytes>",
			"SMTPPASS <password>",
			"WEB <timeout_sec>|<max_bytes>",
			"AI <on|off>|<base_url>|<model>|<timeout_sec>|<max_tokens>",
			"AIPASS <api_key> | AIPROMPT <system prompt> | Q",
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Gateways", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Gateway Controls", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 1024)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if isBackCommand(cmd) {
			return
		}
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "SMTPPASS "):
			settings, _ := s.admin.GetGatewaySettings()
			if settings == nil {
				settings = &domain.GatewaySettings{}
			}
			settings.SMTPHost = strings.TrimSpace(cfg.Host)
			settings.SMTPPort = cfg.Port
			settings.SMTPUser = strings.TrimSpace(cfg.User)
			settings.FromDomain = strings.TrimSpace(cfg.FromDomain)
			settings.MaxRecipients = cfg.MaxRecipients
			settings.MaxMessageBytes = cfg.MaxMessageBytes
			settings.WebTimeoutSec = int(webCfg.Timeout.Seconds())
			settings.WebMaxBytes = int(webCfg.MaxBodyBytes)
			settings.SMTPPass = strings.TrimSpace(cmd[len("SMTPPASS "):])
			if err := s.admin.UpsertGatewaySettings(settings); err != nil {
				adminPause(sess, reader, touch, "SMTP password save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "gateway_settings", "update_gateway_settings", "smtp_pass=updated")
			adminPause(sess, reader, touch, "SMTP password updated.")
		case strings.HasPrefix(upper, "SMTP "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("SMTP "):]), 6)
			settings, _ := s.admin.GetGatewaySettings()
			if settings == nil {
				settings = &domain.GatewaySettings{}
			}
			settings.SMTPHost = strings.TrimSpace(parts[0])
			settings.SMTPPort = clampInt(parseIntWithDefault(parts[1], 587), 1, 65535)
			settings.SMTPUser = strings.TrimSpace(parts[2])
			settings.FromDomain = strings.TrimSpace(parts[3])
			settings.MaxRecipients = clampInt(parseIntWithDefault(parts[4], 3), 1, 25)
			settings.MaxMessageBytes = clampInt(parseIntWithDefault(parts[5], 65536), 1024, 2*1024*1024)
			settings.WebTimeoutSec = int(webCfg.Timeout.Seconds())
			settings.WebMaxBytes = int(webCfg.MaxBodyBytes)
			if existing, err := s.admin.GetGatewaySettings(); err == nil && existing != nil && settings.SMTPPass == "" {
				settings.SMTPPass = existing.SMTPPass
			}
			if err := s.admin.UpsertGatewaySettings(settings); err != nil {
				adminPause(sess, reader, touch, "SMTP save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "gateway_settings", "update_gateway_settings", "smtp=updated")
			adminPause(sess, reader, touch, "SMTP gateway settings updated.")
		case strings.HasPrefix(upper, "WEB "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("WEB "):]), 2)
			settings, _ := s.admin.GetGatewaySettings()
			if settings == nil {
				settings = &domain.GatewaySettings{}
			}
			emailCfg := s.effectiveGatewayConfig()
			settings.SMTPHost = emailCfg.Host
			settings.SMTPPort = emailCfg.Port
			settings.SMTPUser = emailCfg.User
			settings.SMTPPass = emailCfg.Pass
			settings.FromDomain = emailCfg.FromDomain
			settings.MaxRecipients = emailCfg.MaxRecipients
			settings.MaxMessageBytes = emailCfg.MaxMessageBytes
			settings.WebTimeoutSec = clampInt(parseIntWithDefault(parts[0], 10), 1, 120)
			settings.WebMaxBytes = clampInt(parseIntWithDefault(parts[1], 2*1024*1024), 1024, 8*1024*1024)
			if err := s.admin.UpsertGatewaySettings(settings); err != nil {
				adminPause(sess, reader, touch, "Web gateway save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "gateway_settings", "update_gateway_settings", "web=updated")
			adminPause(sess, reader, touch, "Web gateway settings updated.")
		case strings.HasPrefix(upper, "AI "):
			parts := splitFixedParts(strings.TrimSpace(cmd[len("AI "):]), 5)
			enabled := strings.EqualFold(strings.TrimSpace(parts[0]), "on") || strings.EqualFold(strings.TrimSpace(parts[0]), "true") || strings.EqualFold(strings.TrimSpace(parts[0]), "1")
			updates := map[string]string{
				sysSettingGatewayAIEnabled:    strconv.FormatBool(enabled),
				sysSettingGatewayAIBaseURL:    defaultIfBlank(strings.TrimSpace(parts[1]), "https://api.openai.com"),
				sysSettingGatewayAIModel:      defaultIfBlank(strings.TrimSpace(parts[2]), "gpt-4.1-mini"),
				sysSettingGatewayAITimeoutSec: strconv.Itoa(clampInt(parseIntWithDefault(parts[3], aiCfg.TimeoutSec), 1, 120)),
				sysSettingGatewayAIMaxTokens:  strconv.Itoa(clampInt(parseIntWithDefault(parts[4], aiCfg.MaxTokens), 1, 4000)),
			}
			saveErr := ""
			for key, value := range updates {
				if err := s.persistAdminSetting(key, value); err != nil {
					saveErr = err.Error()
					break
				}
			}
			if saveErr != "" {
				adminPause(sess, reader, touch, "AI save failed: "+saveErr)
				continue
			}
			recordAudit(s.admin, actor, "gateway.ai", "update_ai_settings", "enabled="+boolText(enabled))
			adminPause(sess, reader, touch, "AI gateway settings updated.")
		case strings.HasPrefix(upper, "AIPASS "):
			if err := s.persistAdminSetting(sysSettingGatewayAIAPIKey, strings.TrimSpace(cmd[len("AIPASS "):])); err != nil {
				adminPause(sess, reader, touch, "AI API key save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "gateway.ai", "update_ai_key", "")
			adminPause(sess, reader, touch, "AI API key updated.")
		case strings.HasPrefix(upper, "AIPROMPT "):
			if err := s.persistAdminSetting(sysSettingGatewayAISystemPrompt, strings.TrimSpace(cmd[len("AIPROMPT "):])); err != nil {
				adminPause(sess, reader, touch, "AI prompt save failed: "+err.Error())
				continue
			}
			recordAudit(s.admin, actor, "gateway.ai", "update_ai_prompt", "")
			adminPause(sess, reader, touch, "AI system prompt updated.")
		default:
			adminPause(sess, reader, touch, "Use SMTP, SMTPPASS, WEB, AI, AIPASS, AIPROMPT, or Q.")
		}
	}
}

func (s *Server) loadLockedChannelsSSH() map[string]bool {
	out := map[string]bool{}
	if s == nil || s.admin == nil {
		return out
	}
	raw, err := s.admin.GetSystemSetting(adminSettingLockedChannels)
	if err != nil {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		channel := chat.NormalizeChannel(strings.TrimSpace(part))
		if channel == "" {
			continue
		}
		out[channel] = true
	}
	return out
}

func (s *Server) persistLockedChannelsSSH(channels map[string]bool) error {
	keys := make([]string, 0, len(channels))
	for channel, locked := range channels {
		if !locked {
			continue
		}
		keys = append(keys, chat.NormalizeChannel(channel))
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) })
	return s.persistAdminSetting(adminSettingLockedChannels, strings.Join(keys, ","))
}

func (s *Server) setChannelLockSSH(channel string, locked bool) error {
	channel = chat.NormalizeChannel(channel)
	if channel == "" {
		return fmt.Errorf("channel is required")
	}
	channels := s.loadLockedChannelsSSH()
	if locked {
		channels[channel] = true
	} else {
		delete(channels, channel)
	}
	return s.persistLockedChannelsSSH(channels)
}

func (s *Server) isChannelLockedSSH(channel string) bool {
	channel = chat.NormalizeChannel(channel)
	if channel == "" {
		return false
	}
	return s.loadLockedChannelsSSH()[channel]
}

func (s *Server) chatLockBypass(handle string) bool {
	if s == nil || s.auth == nil {
		return false
	}
	user, err := s.auth.GetUser(handle)
	if err != nil || user == nil {
		return false
	}
	role := rbac.NormalizeRole(user.Role)
	return role == rbac.RoleModerator || role == rbac.RoleSysop
}

func (s *Server) runAdminChatOps(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s == nil || s.chatSvc == nil {
		adminPause(sess, reader, touch, "Chat service is unavailable.")
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		channels := s.chatSvc.ListChannels()
		if len(channels) == 0 {
			channels = []string{"#lobby"}
		}
		onlineByChannel := map[string]int{}
		for _, row := range s.chatSvc.Online() {
			channel := chat.NormalizeChannel(row.Area)
			if channel == "" {
				channel = "#lobby"
			}
			onlineByChannel[channel]++
		}
		lines := []string{
			"CREATE <#channel> | LOCK <#channel> | UNLOCK <#channel>",
			"BAN <#channel> <nick> [reason] | UNBAN <#channel> <nick>",
			"MUTE <#channel> <nick> [reason] | UNMUTE <#channel> <nick>",
			"KICK <#channel> <nick> [reason] | Q",
			"",
			fmt.Sprintf("%-16s %-6s %-6s", "Channel", "Online", "Locked"),
		}
		for _, channel := range channels {
			lines = append(lines, fmt.Sprintf("%-16s %-6d %-6s", clampForTTY(channel, 16), onlineByChannel[channel], boolText(s.isChannelLockedSSH(channel))))
		}
		lines = append(lines, "", "Recent moderation:")
		events := s.chatSvc.ModerationLog(6)
		if len(events) == 0 {
			lines = append(lines, "(no moderation events)")
		}
		for _, event := range events {
			lines = append(lines, clampForTTY(fmt.Sprintf("%s %s %s -> %s (%s)", formatClock(event.CreatedAt.UTC(), time24h), event.Type, defaultIfBlank(event.Channel, "#lobby"), defaultIfBlank(event.Target, "-"), defaultIfBlank(event.Reason, "n/a")), renderWidth-4))
		}
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Chat Ops", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Chat Moderation", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Command: ")
		raw, err := readLine(reader, 512)
		if err != nil {
			return
		}
		touch()
		cmd := strings.TrimSpace(raw)
		if cmd == "" {
			continue
		}
		if isBackCommand(cmd) {
			return
		}
		fields := strings.Fields(cmd)
		if len(fields) == 0 {
			continue
		}
		verb := strings.ToUpper(fields[0])
		switch verb {
		case "CREATE":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: CREATE <#channel>")
				continue
			}
			channel := chat.NormalizeChannel(fields[1])
			s.chatSvc.JoinChannel(actor, channel)
			s.chatSvc.LeaveChannel(actor, channel)
			recordAudit(s.admin, actor, channel, "chat_create_channel", "")
			adminPause(sess, reader, touch, "Created/registered channel "+channel+".")
		case "LOCK", "UNLOCK":
			if len(fields) < 2 {
				adminPause(sess, reader, touch, "Usage: LOCK/UNLOCK <#channel>")
				continue
			}
			channel := chat.NormalizeChannel(fields[1])
			locked := verb == "LOCK"
			if err := s.setChannelLockSSH(channel, locked); err != nil {
				adminPause(sess, reader, touch, "Lock update failed: "+err.Error())
				continue
			}
			action := "chat_unlock_channel"
			if locked {
				action = "chat_lock_channel"
			}
			recordAudit(s.admin, actor, channel, action, "")
			adminPause(sess, reader, touch, "Updated channel lock for "+channel+".")
		case "BAN", "MUTE", "KICK":
			if len(fields) < 3 {
				adminPause(sess, reader, touch, "Usage: "+verb+" <#channel> <nick> [reason]")
				continue
			}
			channel := chat.NormalizeChannel(fields[1])
			target := strings.TrimSpace(fields[2])
			reason := ""
			if len(fields) > 3 {
				reason = strings.TrimSpace(strings.Join(fields[3:], " "))
			}
			switch verb {
			case "BAN":
				s.chatSvc.Ban(channel, target, actor, reason, "")
				recordAudit(s.admin, actor, target, "chat_ban", "channel="+channel)
			case "MUTE":
				s.chatSvc.Mute(channel, target, actor, reason, "")
				recordAudit(s.admin, actor, target, "chat_mute", "channel="+channel)
			case "KICK":
				s.chatSvc.Kick(channel, actor, target, reason)
				recordAudit(s.admin, actor, target, "chat_kick", "channel="+channel)
			}
			adminPause(sess, reader, touch, verb+" applied to "+target+".")
		case "UNBAN", "UNMUTE":
			if len(fields) < 3 {
				adminPause(sess, reader, touch, "Usage: "+verb+" <#channel> <nick>")
				continue
			}
			channel := chat.NormalizeChannel(fields[1])
			target := strings.TrimSpace(fields[2])
			if verb == "UNBAN" {
				s.chatSvc.Unban(channel, target)
				recordAudit(s.admin, actor, target, "chat_unban", "channel="+channel)
			} else {
				s.chatSvc.Unmute(channel, target)
				recordAudit(s.admin, actor, target, "chat_unmute", "channel="+channel)
			}
			adminPause(sess, reader, touch, verb+" applied to "+target+".")
		default:
			adminPause(sess, reader, touch, "Use CREATE, LOCK, UNLOCK, BAN, UNBAN, MUTE, UNMUTE, KICK, or Q.")
		}
	}
}

func installPrefixPathSSH() string {
	if prefix := strings.TrimSpace(os.Getenv("WOLFBBS_INSTALL_PREFIX")); prefix != "" {
		return filepath.Clean(prefix)
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Clean(".wolfbbs")
	}
	return filepath.Join(home, defaultInstallPrefixMacOS)
}

func adminPathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func collectAdminArtifacts(glob string, limit int) []adminArtifactRow {
	paths, err := filepath.Glob(glob)
	if err != nil {
		return nil
	}
	rows := make([]adminArtifactRow, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		rows = append(rows, adminArtifactRow{
			Name:      filepath.Base(path),
			Path:      filepath.Clean(path),
			UpdatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].Path < rows[j].Path
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func recentBackupArtifacts(limit int) []adminArtifactRow {
	roots := []string{
		filepath.Join(installPrefixPathSSH(), "SERVICE_STATUS.txt"),
		filepath.Join(installPrefixPathSSH(), "FIRST_STEPS.txt"),
		filepath.Join(installPrefixPathSSH(), "install.log"),
	}
	rows := make([]adminArtifactRow, 0, len(roots)+8)
	for _, path := range roots {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		rows = append(rows, adminArtifactRow{Name: filepath.Base(path), Path: filepath.Clean(path), UpdatedAt: info.ModTime().UTC()})
	}
	rows = append(rows, collectAdminArtifacts(filepath.Join("menus", "*.bak"), limit)...)
	rows = append(rows, collectAdminArtifacts(filepath.Join(profileExportRootDir(), "*", "*.json"), limit)...)
	rows = append(rows, collectAdminArtifacts(filepath.Join(profileExportRootDir(), "*", "*.txt"), limit)...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].Path < rows[j].Path
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func manualAcceptanceSummaryLines(limit int) []string {
	raw, err := os.ReadFile(filepath.Join("docs", "manual-acceptance-latest.md"))
	if err != nil {
		return []string{"manual report missing"}
	}
	lines := make([]string, 0, 4)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "PASS=") || strings.Contains(line, "FAIL=") || strings.HasPrefix(line, "# ") {
			lines = append(lines, line)
		}
		if limit > 0 && len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		return []string{"manual report present but summary not found"}
	}
	return lines
}

func (s *Server) launchReadinessLines() []string {
	lines := []string{
		fmt.Sprintf("Site identity: %s @ %s", s.siteName(), s.siteHostname()),
		fmt.Sprintf("Safety: secure_cookie=%s require_verified=%s read_only=%s", boolText(s.secureCookieEnabled()), boolText(s.requireVerifiedEmail()), boolText(s.readOnlyModeEnabled())),
		fmt.Sprintf("Experience: web_onramp=%s guest_tour=%s discover=%s quick_jump=%s", boolText(s.webOnRampEnabled()), boolText(s.guestTourEnabled()), boolText(s.discoverEnabled()), boolText(s.quickJumpEnabled())),
	}
	userCount := 0
	mailbotReady := false
	if s != nil && s.auth != nil {
		if users, err := s.auth.ListUsers(); err == nil {
			userCount = len(users)
			for _, row := range users {
				if strings.EqualFold(strings.TrimSpace(row.Handle), "mailbot") && !row.Enabled && row.Verified {
					mailbotReady = true
					break
				}
			}
		}
	}
	boardCount := 0
	if s != nil && s.boards != nil {
		if boards, err := s.boards.List(); err == nil {
			boardCount = len(boards)
		}
	}
	gatewayCfg := s.activeEmailGateway().Enabled()
	lines = append(lines,
		fmt.Sprintf("Users: %d   Boards: %d", userCount, boardCount),
		fmt.Sprintf("Mailbot ready: %s   Gateway configured: %s", boolText(mailbotReady), boolText(gatewayCfg)),
	)
	return lines
}

func (s *Server) runAdminReleaseTooling(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, actor string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	for {
		lines := []string{
			"Launch + upgrade + release cockpit parity for /admin/launch, /admin/upgrade-safety, /admin/backups, and /admin/release",
			"",
		}
		lines = append(lines, s.launchReadinessLines()...)
		lines = append(lines, "", "Operator signals:")
		lines = append(lines, s.adminSignalLines(7)...)
		lines = append(lines, "", "Manual acceptance:")
		lines = append(lines, manualAcceptanceSummaryLines(3)...)
		lines = append(lines, "", "Release notes:")
		releaseRows := collectAdminArtifacts(filepath.Join("docs", "releases", "*.md"), defaultAdminArtifactListLimit)
		if len(releaseRows) == 0 {
			lines = append(lines, "(no docs/releases artifacts)")
		} else {
			for _, row := range releaseRows {
				lines = append(lines, clampForTTY(row.Name+" @ "+row.UpdatedAt.Local().Format("2006-01-02 15:04"), renderWidth-4))
			}
		}
		lines = append(lines, "", "Packages:")
		packageRows := collectAdminArtifacts(filepath.Join("dist", "releases", "*"), defaultAdminArtifactListLimit)
		if len(packageRows) == 0 {
			lines = append(lines, "(no dist/releases artifacts)")
		} else {
			for _, row := range packageRows {
				lines = append(lines, clampForTTY(row.Name+" @ "+row.UpdatedAt.Local().Format("2006-01-02 15:04"), renderWidth-4))
			}
		}
		lines = append(lines, "", "Backups:")
		backupRows := recentBackupArtifacts(defaultAdminArtifactListLimit)
		if len(backupRows) == 0 {
			lines = append(lines, "(no backup/offline artifacts)")
		} else {
			for _, row := range backupRows {
				lines = append(lines, clampForTTY(row.Name+" @ "+row.UpdatedAt.Local().Format("2006-01-02 15:04"), renderWidth-4))
			}
		}
		lines = append(lines, "", "Commands: X app upgrade  Q return")
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Admin / Release Tooling", actor, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Release Tooling", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), ansiEnabled, encoding)
		_, _ = io.WriteString(sess, "Selection: ")
		key, err := readKey(reader)
		if err != nil {
			return
		}
		touch()
		switch key {
		case "Q", "ESC":
			return
		case "X":
			s.runAppUpgrade(sess, reader, actor, account, touch)
		default:
			adminPause(sess, reader, touch, "Use X or Q.")
		}
	}
}
