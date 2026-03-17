package sshserver

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/acs"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/config"
	"wolfbbs/internal/discovery"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/doors"
	"wolfbbs/internal/events"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/mci"
	"wolfbbs/internal/menu"
	"wolfbbs/internal/netutil"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/session"
	"wolfbbs/internal/term"
	"wolfbbs/internal/ui"
)

type Server struct {
	address string
	logger  *slog.Logger
	auth    *auth.Service
	users   repository.UserRepository
	boards  repository.BoardRepository
	msgs    repository.MessageRepository
	mail    repository.PrivateMailRepository
	admin   repository.AdminRepository
	doors   repository.DoorRepository
	chatSvc *chat.Service
	bus     *events.Bus
	nodes   *session.Manager
	menuMod *menu.Registry
	server  *gssh.Server
}

type screenState int

const (
	stateWelcome screenState = iota
	stateLogin
	stateGuestTour
	stateBulletins
	stateMainMenu
	stateGateway
	stateLastCallers
	stateWhoOnline
	stateDoors
	stateConfigCenter
	stateStatusCenter
	stateExit
)

const (
	appUpgradeCommandEnv = "WOLFBBS_APP_UPGRADE_COMMAND"
	appUpgradeWorkDirEnv = "WOLFBBS_APP_UPGRADE_WORKDIR"
	appUpgradeTimeoutEnv = "WOLFBBS_APP_UPGRADE_TIMEOUT_SECONDS"
)

var appUpgradeExec = func(ctx context.Context, command, workDir string, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append([]string{}, env...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func New(address string, logger *slog.Logger, authSvc *auth.Service) *Server {
	s := &Server{
		address: address,
		logger:  logger,
		auth:    authSvc,
		nodes:   session.NewManager(255, 256),
		menuMod: menu.NewRegistry(),
	}
	s.server = &gssh.Server{
		Addr:        address,
		Handler:     s.handleSession,
		IdleTimeout: 10 * time.Minute,
		MaxTimeout:  30 * time.Minute,
	}
	return s
}

func (s *Server) SetSessionManager(mgr *session.Manager) {
	if mgr == nil {
		return
	}
	s.nodes = mgr
}

func (s *Server) SetRepositories(users repository.UserRepository, boards repository.BoardRepository, messages repository.MessageRepository, mail repository.PrivateMailRepository, admin repository.AdminRepository, doorRepo repository.DoorRepository) {
	s.users = users
	s.boards = boards
	s.msgs = messages
	s.mail = mail
	s.admin = admin
	s.doors = doorRepo
}

func (s *Server) SetChatService(chatSvc *chat.Service) {
	s.chatSvc = chatSvc
}

func (s *Server) SetEventBus(bus *events.Bus) {
	s.bus = bus
}

func (s *Server) SetMenuRegistry(registry *menu.Registry) {
	if registry == nil {
		return
	}
	s.menuMod = registry
}

func (s *Server) ListenAndServe() error {
	s.logger.Info("starting ssh server", "addr", s.address)
	return s.server.ListenAndServe()
}

func (s *Server) Serve(listener net.Listener) error {
	s.logger.Info("starting ssh server", "addr", listener.Addr().String())
	return s.server.Serve(listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleSession(sess gssh.Session) {
	pty, _, ok := sess.Pty()
	if !ok {
		io.WriteString(sess, "PTY required. Reconnect with a terminal.\n")
		return
	}
	termWidth := pty.Window.Width
	h := pty.Window.Height
	if termWidth < 40 {
		termWidth = 40
	}
	if h < 20 {
		h = 20
	}
	renderWidth := termWidth
	if renderWidth > ui.DefaultWidth {
		renderWidth = ui.DefaultWidth
	}
	_ = h

	reader := bufio.NewReader(sess)
	doorRegistry := doors.NewRegistry()
	doorRegistry.SetRepository(s.doors)
	if triviaBinary := strings.TrimSpace(os.Getenv("WOLFBBS_TRIVIA_BINARY")); triviaBinary != "" {
		doors.SeedTrivia(doorRegistry, triviaBinary)
	}
	doors.SeedFromEnv(doorRegistry)
	offlineDir := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
	if offlineDir == "" {
		offlineDir = ".wolfbbs/offline"
	}
	emailGateway := gateway.NewEmailGateway(gateway.LoadEmailConfigFromEnv())
	th := ui.DefaultTheme()
	state := stateWelcome
	termProfile := term.DetectProfile(pty.Term, os.Getenv("LANG"), strings.TrimSpace(os.Getenv("WOLFBBS_TERM_ENCODING")), termWidth, h, true)
	sessionANSI := true
	sessionEncoding := string(termProfile.Encoding)
	sessionTime24h := true
	currentUser := "Guest"
	currentAccount := &domain.User{Handle: "Guest", Role: "user", ANSIEnabled: true, TimeFormat24h: true, Theme: "retro-amber"}
	doorCategoryIndex := 0
	doorFavoritesOnly := false
	doorRecentOnly := false
	sessionID := ""
	if ctx := sess.Context(); ctx != nil {
		sessionID = strings.TrimSpace(ctx.SessionID())
	}
	if sessionID == "" {
		sessionID = fmt.Sprintf("anon-%d", time.Now().UnixNano())
	}
	remoteAddr := ""
	if ra := sess.RemoteAddr(); ra != nil {
		remoteAddr = ra.String()
	}
	remoteHost := normalizeRemoteHost(remoteAddr)
	remoteOrigin := netutil.RemoteOrigin(remoteAddr)
	s.logger.Info("session connected", "session_id", sessionID, "remote_addr", remoteAddr, "remote_host", remoteHost, "remote_origin", remoteOrigin)
	if s.nodes == nil {
		s.nodes = session.NewManager(255, 256)
	}
	nodeState, err := s.nodes.Start(sessionID, currentUser, remoteAddr)
	if err != nil {
		io.WriteString(sess, "No free nodes. Try again later.\r\n")
		return
	}
	defer func() {
		snapshot, ok := s.nodes.Get(sessionID)
		s.nodes.End(sessionID)
		if s.admin == nil {
			return
		}
		_ = s.admin.DeleteNodeSession(sessionID)
		if !ok {
			return
		}
		now := time.Now().UTC()
		duration := now.Sub(snapshot.LoginAt)
		if duration < 0 {
			duration = 0
		}
		logoutHost := normalizeRemoteHost(snapshot.RemoteAddr)
		logoutOrigin := netutil.RemoteOrigin(snapshot.RemoteAddr)
		s.logger.Info("session disconnected", "session_id", sessionID, "node", snapshot.NodeID, "user", snapshot.Username, "remote_host", logoutHost, "remote_origin", logoutOrigin, "duration_seconds", int64(duration.Seconds()))
		recordAudit(s.admin, snapshot.Username, fmt.Sprintf("Node %d", snapshot.NodeID), "session_logout",
			fmt.Sprintf("origin=%s host=%s area=%s duration=%s", strings.ToUpper(logoutOrigin), logoutHost, snapshot.Area, formatDuration(duration)))
		_ = s.admin.AddCallerHistory(&domain.CallerHistory{
			SessionID:       sessionID,
			NodeID:          snapshot.NodeID,
			Username:        snapshot.Username,
			Area:            snapshot.Area,
			RemoteAddr:      snapshot.RemoteAddr,
			LoginAt:         snapshot.LoginAt,
			LogoutAt:        now,
			DurationSeconds: int64(duration.Seconds()),
			CreatedAt:       now,
		})
	}()
	nodeID := nodeState.NodeID
	nodeLabel := fmt.Sprintf("Node %d", nodeID)
	currentArea := nodeState.Area

	lastNodePersist := time.Time{}
	persistNode := func(force bool) {
		if s.admin == nil {
			return
		}
		now := time.Now().UTC()
		if !force && !lastNodePersist.IsZero() && now.Sub(lastNodePersist) < 15*time.Second {
			return
		}
		snapshot, ok := s.nodes.Get(sessionID)
		if !ok {
			return
		}
		lastNodePersist = now
		_ = s.admin.UpsertNodeSession(&domain.NodeSession{
			SessionID:    sessionID,
			NodeID:       snapshot.NodeID,
			Username:     snapshot.Username,
			Area:         snapshot.Area,
			RemoteAddr:   snapshot.RemoteAddr,
			LoginAt:      snapshot.LoginAt,
			LastActivity: snapshot.LastActivity,
			UpdatedAt:    now,
		})
	}
	persistNode(true)

	setArea := func(area string) {
		currentArea = strings.TrimSpace(area)
		s.nodes.SetArea(sessionID, area)
		persistNode(true)
	}
	touch := func() {
		s.nodes.Touch(sessionID)
		persistNode(false)
	}
	runtimeCfg, cfgErr := config.CachedRuntime()
	if cfgErr != nil {
		s.logger.Warn("runtime config load failed; using env fallback", "error", cfgErr)
	}
	acsStrict := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_STRICT")))
	menuEnabled := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_MENU_ENABLE")))
	menuFile := strings.TrimSpace(os.Getenv("WOLFBBS_MENU_FILE"))
	if cfgErr == nil {
		acsStrict = runtimeCfg.ACS.Strict
		menuEnabled = runtimeCfg.Menu.Enabled
		menuFile = strings.TrimSpace(runtimeCfg.Menu.File)
	}
	if menuEnabled && menuFile == "" {
		menuFile = "menus/main.hjson"
	}
	var configuredMenu *menu.Screen
	if menuEnabled && menuFile != "" {
		screen, loadErr := menu.LoadHJSON(menuFile)
		if loadErr != nil {
			s.logger.Warn("menu load failed; falling back to built-in menu", "path", menuFile, "error", loadErr)
		} else {
			configuredMenu = &screen
		}
	}
	if s.menuMod == nil {
		s.menuMod = menu.NewRegistry()
	}

	menuEntriesForUser := func() []menu.Entry {
		if configuredMenu == nil {
			return nil
		}
		visible := make([]menu.Entry, 0, len(configuredMenu.Entries))
		for _, entry := range configuredMenu.Entries {
			if evaluateAccess(entry.ACS, currentAccount, currentUser, map[string]string{
				"screen": configuredMenu.ID,
				"target": entry.Target,
			}, acsStrict, s.logger) {
				visible = append(visible, entry)
			}
		}
		return visible
	}

	for {
		switch state {
		case stateWelcome:
			setArea("Welcome")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			io.WriteString(sess, "\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderWelcome(renderWidth), sessionANSI, sessionEncoding)
			io.WriteString(sess, ui.FooterPrompt(renderWidth, "Press any key to continue")+"\r\n")
			if key, err := readKey(reader); err == nil {
				touch()
				if key == "ESC" || key == "CTRL-C" || key == "Q" {
					state = stateExit
				} else {
					state = stateLogin
				}
			} else {
				return
			}
		case stateLogin:
			setArea("Login")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), "Guest", time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderLoginPromptWithGuest(renderWidth, s.guestTourEnabled())+"\r\n", sessionANSI, sessionEncoding)
			io.WriteString(sess, "Handle: ")
			handle, err := readLine(reader, 32)
			if err != nil {
				return
			}
			touch()
			handle = strings.TrimSpace(handle)
			if handle == "?" {
				showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", "Guest", nodeLabel, th, sessionTime24h, sessionANSI, sessionEncoding, ui.RenderLoginHelp(renderWidth, s.guestTourEnabled()), touch)
				state = stateLogin
				continue
			}
			if isGuestTourHandle(handle) && s.guestTourEnabled() {
				state = stateGuestTour
				continue
			}
			io.WriteString(sess, "Password: ")
			pass, err := readLine(reader, 64)
			if err != nil {
				return
			}
			touch()
			pass = strings.TrimSpace(pass)
			if handle == "" || pass == "" {
				io.WriteString(sess, "\r\nMissing input. Press any key to retry.\r\n")
				_, _ = readKey(reader)
				touch()
				state = stateLogin
				continue
			}

			user, err := s.auth.GetUser(handle)
			created := false
			if err != nil {
				io.WriteString(sess, "\r\nInvalid login. Create account? (Y/N): ")
				choice, err := readLine(reader, 4)
				if err != nil {
					return
				}
				touch()
				if strings.EqualFold(strings.TrimSpace(choice), "Y") {
					createdAccount, createErr := s.auth.Register(handle, pass)
					if createErr != nil {
						io.WriteString(sess, fmt.Sprintf("\r\nCould not create account: %v\r\n", createErr))
						_, _ = readKey(reader)
						touch()
						state = stateLogin
						continue
					}
					user = createdAccount
					created = true
					err = nil
				} else {
					io.WriteString(sess, "\r\nReturning to login.\r\n")
					_, _ = readKey(reader)
					touch()
					state = stateLogin
					continue
				}
			}

			secondFactor := ""
			if user != nil && user.TOTPSecret != "" && !created {
				io.WriteString(sess, "\r\nTwo-factor code: ")
				secondFactor, err = readLine(reader, 16)
				if err != nil {
					return
				}
				touch()
			}
			user, err = s.auth.Authenticate(handle, pass, strings.TrimSpace(secondFactor))
			if err != nil {
				if err == auth.ErrMissingSecondFactor || err == auth.ErrInvalidSecondFactor {
					io.WriteString(sess, "\r\nInvalid two-factor code. Press any key.\r\n")
				} else {
					io.WriteString(sess, "\r\nLogin failed. Press any key.\r\n")
				}
				_, _ = readKey(reader)
				touch()
				state = stateLogin
				continue
			}
			if created {
				io.WriteString(sess, "\r\nWelcome to "+s.siteName()+"! Press any key to continue.\r\n")
				_, _ = readKey(reader)
				touch()
			}

			currentUser = user.Handle
			currentAccount = user
			sessionANSI = user.ANSIEnabled
			sessionTime24h = user.TimeFormat24h
			th = ui.ThemeByName(user.Theme)
			s.nodes.SetUser(sessionID, currentUser)
			persistNode(true)
			if !sessionANSI {
				sessionEncoding = string(term.EncodingASCII)
			} else {
				sessionEncoding = string(termProfile.Encoding)
			}
			s.logger.Info("user authenticated", "user", currentUser, "session_id", sessionID, "node", nodeID, "remote_host", remoteHost, "remote_origin", remoteOrigin)
			recordAudit(s.admin, currentUser, nodeLabel, "session_login",
				fmt.Sprintf("origin=%s host=%s transport=ssh", strings.ToUpper(remoteOrigin), remoteHost))
			state = stateBulletins
		case stateBulletins:
			setArea("Bulletins")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			since := time.Now().UTC().Add(-24 * time.Hour)
			digestLines := []string{
				"System maintenance window: Sundays 03:00 UTC.",
				"Gateways are rate-limited for abuse prevention.",
				"Use ? in menus for command help.",
			}
			if s.smartNewscanEnabled() && currentAccount != nil && s.boards != nil && s.msgs != nil {
				digest, err := discovery.BuildSinceLastCall(s.boards, s.msgs, s.mail, currentAccount, smartNewscanMaxItems())
				if err == nil {
					since = digest.Since
					digestLines = make([]string, 0, len(digest.Items)+2)
					if summary := discovery.BuildConferenceSummary(digest.Items, 4); len(summary) > 0 {
						digestLines = append(digestLines, summary...)
						digestLines = append(digestLines, "")
					}
					for _, row := range digest.Items {
						digestLines = append(digestLines, row.Line)
					}
					if s.aiAssistEnabled() {
						if line := discovery.BuildAICatchUpLine(digest.Items); line != "" {
							digestLines = append([]string{line}, digestLines...)
						}
					}
					if len(digestLines) == 0 {
						digestLines = append(digestLines, "No new activity since your last call.")
					}
				}
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderSinceLastCall(renderWidth, digestLines, since)+"\r\n", sessionANSI, sessionEncoding)
			_, _ = readKey(reader)
			touch()
			state = stateMainMenu
		case stateGuestTour:
			setArea("Guest Tour")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), "Guest", time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderGuestTour(renderWidth, s.guestTourLines(sessionTime24h))+"\r\n", sessionANSI, sessionEncoding)
			_, _ = readKey(reader)
			touch()
			state = stateLogin
		case stateMainMenu:
			setArea("Main Menu")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			menuEntries := menuEntriesForUser()
			menuMode := configuredMenu != nil
			if menuMode {
				title := strings.TrimSpace(configuredMenu.Title)
				if title == "" {
					title = "Main Menu"
				}
				lines := make([]string, 0, len(menuEntries)+3)
				for _, entry := range menuEntries {
					lines = append(lines, fmt.Sprintf("[%s] %-15s  %s", strings.ToUpper(entry.Hotkey), truncateText(entry.Label, 15), truncateText(entry.Target, 26)))
				}
				if len(lines) == 0 {
					lines = append(lines, "No menu options are currently available.")
				}
				if help := strings.TrimSpace(configuredMenu.Help); help != "" {
					lines = append(lines, "")
					lines = append(lines, truncateText(help, renderWidth-4))
				}
				renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, title, lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), sessionANSI, sessionEncoding)
				footer := strings.TrimSpace(configuredMenu.Footer)
				if footer == "" {
					footer = "Press Q to quit, ? for help"
				}
				io.WriteString(sess, ui.FooterPrompt(renderWidth, footer)+"\r\n")
			} else {
				renderFrame(sess, termWidth, renderWidth, ui.RenderMainMenu(renderWidth), sessionANSI, sessionEncoding)
				io.WriteString(sess, ui.FooterPrompt(renderWidth, "Press Q to quit, ? for help")+"\r\n")
			}
			io.WriteString(sess, "Enter selection: ")
			key, err := readKey(reader)
			if err != nil {
				return
			}
			touch()
			s.logger.Info("menu selection", "user", currentUser, "selection", key)
			action := ""
			target := ""
			if menuMode {
				if key == "?" {
					helpText := strings.TrimSpace(configuredMenu.Help)
					showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", currentUser, nodeLabel, th, sessionTime24h, sessionANSI, sessionEncoding, ui.RenderMainMenuHelp(renderWidth, helpText), touch)
					break
				}
				for _, entry := range menuEntries {
					if strings.EqualFold(strings.TrimSpace(entry.Hotkey), strings.TrimSpace(key)) {
						action = strings.ToLower(strings.TrimSpace(entry.Action))
						target = strings.TrimSpace(entry.Target)
						break
					}
				}
				if action == "" {
					action = legacyHotkeyToAction(key)
					if action == "" {
						io.WriteString(sess, "\r\nUse a single-letter hotkey listed in the menu.\r\n")
						_, _ = readKey(reader)
						touch()
						break
					}
				}
			} else {
				if key == "?" {
					showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", currentUser, nodeLabel, th, sessionTime24h, sessionANSI, sessionEncoding, ui.RenderMainMenuHelp(renderWidth, ""), touch)
					break
				}
				action = legacyHotkeyToAction(key)
			}

			switch action {
			case "session.quit":
				state = stateExit
			case "system.newscan":
				state = stateBulletins
			case "boards.open":
				setArea("Message Boards")
				s.runBoards(sess, reader, termWidth, renderWidth, currentUser, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
				setArea("Main Menu")
			case "mail.open":
				setArea("Private Mail")
				s.runMail(sess, reader, termWidth, renderWidth, currentUser, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
				setArea("Main Menu")
			case "chat.open":
				setArea("Chat")
				s.runChat(sess, reader, termWidth, renderWidth, currentUser, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
				setArea("Main Menu")
			case "gateway.open":
				state = stateGateway
			case "doors.open":
				state = stateDoors
			case "system.last_callers":
				state = stateLastCallers
			case "system.who_online":
				state = stateWhoOnline
			case "system.config_center":
				state = stateConfigCenter
			case "system.status_center":
				state = stateStatusCenter
			case "system.quick_jump":
				if !s.quickJumpEnabled() {
					io.WriteString(sess, "\r\nQuick Jump is disabled by sysop. Press any key.")
					_, _ = readKey(reader)
					touch()
					break
				}
				io.WriteString(sess, "\r\nJump target (boards/mail/chat/gateway/doors/settings/last/who/status/config/newscan/app-upgrade): ")
				targetRaw, readErr := readLine(reader, 32)
				if readErr != nil {
					return
				}
				touch()
				jumpAction := quickJumpToAction(targetRaw)
				if jumpAction == "" {
					io.WriteString(sess, "\r\nUnknown jump target. Press any key.")
					_, _ = readKey(reader)
					touch()
					break
				}
				switch jumpAction {
				case "system.newscan":
					state = stateBulletins
				case "boards.open":
					setArea("Message Boards")
					s.runBoards(sess, reader, termWidth, renderWidth, currentUser, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
					setArea("Main Menu")
				case "mail.open":
					setArea("Private Mail")
					s.runMail(sess, reader, termWidth, renderWidth, currentUser, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
					setArea("Main Menu")
				case "chat.open":
					setArea("Chat")
					s.runChat(sess, reader, termWidth, renderWidth, currentUser, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
					setArea("Main Menu")
				case "gateway.open":
					state = stateGateway
				case "doors.open":
					state = stateDoors
				case "system.last_callers":
					state = stateLastCallers
				case "system.who_online":
					state = stateWhoOnline
				case "settings.open":
					updated, updateErr := s.runSettingsMCI(sess, reader, termWidth, renderWidth, currentUser, currentAccount, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
					if updateErr != nil {
						io.WriteString(sess, "\r\nSettings update failed: "+updateErr.Error()+"\r\nPress any key.")
						_, _ = readKey(reader)
						touch()
						break
					}
					if updated != nil {
						currentAccount = updated
						sessionANSI = updated.ANSIEnabled
						sessionTime24h = updated.TimeFormat24h
						th = ui.ThemeByName(updated.Theme)
						if !sessionANSI {
							sessionEncoding = string(term.EncodingASCII)
						} else {
							sessionEncoding = string(termProfile.Encoding)
						}
					}
				case "system.config_center":
					state = stateConfigCenter
				case "system.status_center":
					state = stateStatusCenter
				case "system.app_upgrade":
					s.runAppUpgrade(sess, reader, currentUser, currentAccount, touch)
				}
			case "files.open":
				if !evaluateAccess(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_FILES_READ")), currentAccount, currentUser, map[string]string{"area": "files"}, acsStrict, s.logger) {
					io.WriteString(sess, "\r\nAccess denied by ACS rule. Press any key.")
					_, _ = readKey(reader)
					touch()
					break
				}
				setArea("Files")
				s.runFiles(sess, reader, termWidth, renderWidth, currentUser, currentAccount, th, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
				setArea("Main Menu")
			case "settings.open":
				updated, err := s.runSettingsMCI(sess, reader, termWidth, renderWidth, currentUser, currentAccount, sessionANSI, sessionEncoding, sessionTime24h, nodeLabel, touch)
				if err != nil {
					io.WriteString(sess, "\r\nSettings update failed: "+err.Error()+"\r\nPress any key.")
					_, _ = readKey(reader)
					touch()
					break
				}
				if updated != nil {
					currentAccount = updated
					sessionANSI = updated.ANSIEnabled
					sessionTime24h = updated.TimeFormat24h
					th = ui.ThemeByName(updated.Theme)
					if !sessionANSI {
						sessionEncoding = string(term.EncodingASCII)
					} else {
						sessionEncoding = string(termProfile.Encoding)
					}
				}
			case "admin.open":
				if !evaluateAccess("role=sysop", currentAccount, currentUser, map[string]string{"area": "admin"}, acsStrict, s.logger) {
					io.WriteString(sess, "\r\nAdmin access denied. Press any key.")
					_, _ = readKey(reader)
					touch()
					break
				}
				io.WriteString(sess, "\r\nUse web admin at /admin for full sysop controls. Press any key.")
				_, _ = readKey(reader)
				touch()
			case "system.app_upgrade":
				s.runAppUpgrade(sess, reader, currentUser, currentAccount, touch)
			case "":
				if len(key) == 1 {
					io.WriteString(sess, "\r\nUse a single-letter hotkey listed in the menu.\r\n")
				} else {
					io.WriteString(sess, "\r\nUnknown key sequence.\r\n")
				}
				_, _ = readKey(reader)
				touch()
			default:
				execErr := s.menuMod.Execute(action, menu.ModuleContext{
					User:      currentUser,
					SessionID: sessionID,
					Args: map[string]string{
						"target": target,
					},
				}, menu.Entry{
					Hotkey: key,
					Action: action,
					Target: target,
				})
				if execErr != nil {
					io.WriteString(sess, "\r\nUnknown menu action: "+action+"\r\nPress any key.")
				} else {
					io.WriteString(sess, "\r\nModule action complete. Press any key.")
				}
				_, _ = readKey(reader)
				touch()
			}
		case stateDoors:
			setArea("Doors")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Doors", currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			userID := int64(0)
			role := "user"
			createdAt := ""
			if currentAccount != nil {
				userID = currentAccount.ID
				if strings.TrimSpace(currentAccount.Role) != "" {
					role = strings.ToLower(strings.TrimSpace(currentAccount.Role))
				}
				if !currentAccount.CreatedAt.IsZero() {
					createdAt = currentAccount.CreatedAt.UTC().Format(time.RFC3339)
				}
			}
			favoriteRows, _ := doorRegistry.ListFavorites(userID, 5)
			recentRows, _ := doorRegistry.ListRecent(userID, 5)
			favorites := make([]string, 0, len(favoriteRows))
			recent := make([]string, 0, len(recentRows))
			favMap := map[string]bool{}
			recentMap := map[string]bool{}
			for _, row := range favoriteRows {
				if row.DoorID != "" {
					favorites = append(favorites, strings.ToUpper(row.DoorID))
					favMap[row.DoorID] = true
				}
			}
			for _, row := range recentRows {
				if row.DoorID != "" {
					recent = append(recent, strings.ToUpper(row.DoorID))
					recentMap[row.DoorID] = true
				}
			}
			allDoors := doorRegistry.Doors()
			categoryChoices := []string{""}
			categorySeen := map[string]struct{}{}
			for _, door := range allDoors {
				category := strings.ToLower(strings.TrimSpace(door.Category))
				if category == "" {
					continue
				}
				if _, ok := categorySeen[category]; ok {
					continue
				}
				categorySeen[category] = struct{}{}
				categoryChoices = append(categoryChoices, category)
			}
			sort.Strings(categoryChoices[1:])
			if doorCategoryIndex >= len(categoryChoices) {
				doorCategoryIndex = 0
			}
			currentCategory := categoryChoices[doorCategoryIndex]
			items := make([]ui.DoorMenuItem, 0)
			for _, door := range allDoors {
				if doorFavoritesOnly && !favMap[door.ID] {
					continue
				}
				if doorRecentOnly && !recentMap[door.ID] {
					continue
				}
				if currentCategory != "" && !strings.EqualFold(strings.TrimSpace(door.Category), currentCategory) {
					continue
				}
				turns, err := doorRegistry.TurnsRemaining(userID, door.ID, time.Now())
				if err != nil {
					turns = 0
				}
				items = append(items, ui.DoorMenuItem{
					Hotkey:         door.Hotkey,
					Name:           door.Name,
					Category:       door.Category,
					TurnsRemaining: turns,
					Favorite:       favMap[door.ID],
				})
			}
			summary := ui.DoorMenuSummary{
				Total:         len(allDoors),
				Visible:       len(items),
				Category:      currentCategory,
				FavoritesOnly: doorFavoritesOnly,
				RecentOnly:    doorRecentOnly,
				Spotlight:     doorSpotlightLabel(items),
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderDoorMenu(renderWidth, items, favorites, recent, summary)+"\r\n", sessionANSI, sessionEncoding)
			io.WriteString(sess, "Selection: ")
			choice, err := readKey(reader)
			if err != nil {
				return
			}
			touch()
			if choice == "R" || choice == "Q" || choice == "ESC" {
				state = stateMainMenu
				break
			}
			if choice == "?" {
				showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", currentUser, nodeLabel, th, sessionTime24h, sessionANSI, sessionEncoding, ui.RenderDoorsHelp(renderWidth), touch)
				state = stateDoors
				break
			}
			if choice == "F" {
				doorFavoritesOnly = !doorFavoritesOnly
				state = stateDoors
				break
			}
			if choice == "V" {
				doorRecentOnly = !doorRecentOnly
				state = stateDoors
				break
			}
			if choice == "C" {
				doorCategoryIndex = (doorCategoryIndex + 1) % len(categoryChoices)
				state = stateDoors
				break
			}
			if choice == "!" {
				io.WriteString(sess, "\r\nFavorite toggle door hotkey: ")
				toggleKey, err := readKey(reader)
				if err != nil {
					return
				}
				touch()
				selectedDoor, ok := doorRegistry.GetByHotkey(toggleKey)
				if !ok {
					io.WriteString(sess, "\r\nUnknown door hotkey. Press any key.")
					_, _ = readKey(reader)
					state = stateDoors
					break
				}
				favorited, favErr := doorRegistry.ToggleFavorite(userID, selectedDoor.ID)
				if favErr != nil {
					io.WriteString(sess, "\r\nFavorite toggle failed: "+favErr.Error()+"\r\nPress any key.")
					_, _ = readKey(reader)
					state = stateDoors
					break
				}
				flag := "removed"
				if favorited {
					flag = "added"
				}
				io.WriteString(sess, "\r\nFavorite "+flag+": "+selectedDoor.Name+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				state = stateDoors
				break
			}
			if choice == "T" {
				writeClear(sess, sessionANSI)
				renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Door Scores & Trophies", currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
				lines := []string{"Top Scores", "-----------------------------------------------"}
				for _, door := range doorRegistry.Doors() {
					scores, _ := doorRegistry.ListScores(door.ID, 1)
					if len(scores) == 0 {
						continue
					}
					lines = append(lines, fmt.Sprintf("%-28s %6d", truncateText(door.Name, 28), scores[0].Value))
				}
				if len(lines) == 2 {
					lines = append(lines, "No scores submitted yet.")
				}
				achievements, _ := doorRegistry.ListAchievements(userID, "", 20)
				lines = append(lines, "", "Recent Achievements", "-----------------------------------------------")
				if len(achievements) == 0 {
					lines = append(lines, "No achievements yet.")
				} else {
					for _, row := range achievements {
						lines = append(lines, fmt.Sprintf("%-24s %s", truncateText(strings.ToUpper(row.DoorID), 24), row.AchievementCode))
					}
				}
				renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Door Scores & Trophies", lines, ui.CP437Box, ui.FgYellow, ui.BgBlack), sessionANSI, sessionEncoding)
				io.WriteString(sess, "\r\nPress any key to return.")
				_, _ = readKey(reader)
				touch()
				state = stateDoors
				break
			}
			ctx, cancel := doors.DoorTimeout(context.Background(), 180*time.Second)
			err = doorRegistry.Launch(ctx, choice, sess, sess, sess, map[string]string{
				"WOLFBBS_HANDLE":          currentUser,
				"WOLFBBS_USER_ID":         strconv.FormatInt(userID, 10),
				"WOLFBBS_ROLE":            role,
				"WOLFBBS_USER_CREATED_AT": createdAt,
				"WOLFBBS_SESSION_ID":      sessionID,
				"WOLFBBS_NODE":            strconv.Itoa(nodeID),
				"WOLFBBS_AREA":            "doors",
				"WOLFBBS_TERM_COLS":       strconv.Itoa(termWidth),
				"WOLFBBS_TERM_ROWS":       strconv.Itoa(h),
				"WOLFBBS_ANSI":            boolText(sessionANSI),
				"WOLFBBS_ENCODING":        sessionEncoding,
				"WOLFBBS_COLOR_DEPTH":     "8",
				"WOLFBBS_TZ":              time.Now().Location().String(),
			})
			cancel()
			if err != nil {
				recordAudit(s.admin, currentUser, strings.ToUpper(choice), "door_failure", err.Error())
				io.WriteString(sess, "\r\nDoor launch failed: "+err.Error()+"\r\nPress any key to continue.")
				_, _ = readKey(reader)
				touch()
			} else {
				recordAudit(s.admin, currentUser, strings.ToUpper(choice), "door_launch", "ok")
			}
			state = stateDoors
		case stateGateway:
			setArea("Gateways")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Gateway", currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderGatewayMenu(renderWidth)+"\r\n", sessionANSI, sessionEncoding)
			io.WriteString(sess, "Selection: ")
			gw, err := readKey(reader)
			if err != nil {
				return
			}
			touch()
			switch gw {
			case "R", "Q", "ESC":
				state = stateMainMenu
			case "?":
				showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", currentUser, nodeLabel, th, sessionTime24h, sessionANSI, sessionEncoding, ui.RenderGatewayHelp(renderWidth), touch)
				state = stateGateway
			case "E":
				io.WriteString(sess, "\r\nTo external email: ")
				to, err := readLine(reader, 200)
				if err != nil {
					return
				}
				io.WriteString(sess, "Subject: ")
				subject, err := readLine(reader, 120)
				if err != nil {
					return
				}
				io.WriteString(sess, "Body (blank line then '.' to send):\r\n")
				body, err := readMessageBody(reader, 100, 65536)
				if err != nil {
					return
				}
				to = strings.TrimSpace(to)
				subject = strings.TrimSpace(subject)
				body = strings.TrimSpace(body)
				if to == "" || subject == "" || body == "" {
					io.WriteString(sess, "\r\nTo/subject/body required. Press any key.\r\n")
					_, _ = readKey(reader)
					touch()
					state = stateMainMenu
					continue
				}
				user, userErr := s.auth.GetUser(currentUser)
				if userErr != nil || user == nil {
					io.WriteString(sess, "\r\nCould not load current user. Press any key.\r\n")
					_, _ = readKey(reader)
					touch()
					state = stateMainMenu
					continue
				}
				if s.requireVerifiedEmail() && !user.Verified {
					io.WriteString(sess, "\r\nAccount must be verified before external email. Press any key.\r\n")
					_, _ = readKey(reader)
					touch()
					state = stateMainMenu
					continue
				}
				if s.admin != nil {
					policy, err := s.admin.GetMailOutboundPolicy(user.Handle)
					if err == nil && policy != nil && policy.OutboundDisabled {
						io.WriteString(sess, "\r\nOutbound email disabled for your account. Press any key.\r\n")
						_, _ = readKey(reader)
						touch()
						state = stateMainMenu
						continue
					}
				}
				if err := emailGateway.SendOutbound(user.Handle, []string{to}, subject, body); err != nil {
					io.WriteString(sess, "\r\nOutbound failed: "+err.Error()+"\r\nPress any key.\r\n")
					_, _ = readKey(reader)
					touch()
					recordAudit(s.admin, user.Handle, to, "email_send_failed", err.Error())
					state = stateMainMenu
					continue
				}
				if s.mail != nil {
					recipient := to
					_ = s.mail.CreateMail(&domain.PrivateMail{
						FromUserID: user.ID,
						Subject:    subject,
						Body:       body,
						ExternalTo: &recipient,
					})
				}
				recordAudit(s.admin, user.Handle, to, "email_send", "external")
				io.WriteString(sess, "\r\nEmail queued via SMTP relay. Press any key.\r\n")
				_, _ = readKey(reader)
				touch()
				state = stateMainMenu
			case "W":
				io.WriteString(sess, "\r\nURL: ")
				url, err := readLine(reader, 160)
				if err != nil {
					return
				}
				u := strings.TrimSpace(url)
				if u == "" {
					io.WriteString(sess, "\r\nNo URL entered. Press any key.\r\n")
				} else {
					io.WriteString(sess, "\r\nFetching and formatting...\r\n")
					text, err := gateway.FetchText(context.Background(), u, gateway.DefaultFetchConfig)
					if err != nil {
						io.WriteString(sess, "\r\nGateway blocked: "+err.Error()+"\r\n")
					} else {
						pagerWrite(sess, reader, text)
						io.WriteString(sess, "\r\nSave for offline reading? (Y/N): ")
						answer, err := readKey(reader)
						if err == nil && answer == "Y" {
							touch()
							path, saveErr := gateway.SaveOffline(offlineDir, currentUser, u, text)
							if saveErr != nil {
								io.WriteString(sess, "\r\nSave failed: "+saveErr.Error())
							} else {
								io.WriteString(sess, "\r\nSaved: "+path)
							}
						}
					}
				}
				state = stateMainMenu
			default:
				io.WriteString(sess, "\r\nUnknown gateway key. Press any key.\r\n")
				_, _ = readKey(reader)
				touch()
				state = stateMainMenu
			}
		case stateLastCallers:
			setArea("Last Callers")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			io.WriteString(sess, "\r\n")
			last := []string{}
			if s.admin != nil {
				if callers, err := s.admin.ListCallerHistory(25); err == nil {
					for _, caller := range callers {
						duration := time.Duration(caller.DurationSeconds) * time.Second
						last = append(last, formatCallerTTYRow(renderWidth,
							caller.NodeID,
							caller.Username,
							formatClock(caller.LoginAt.Local(), sessionTime24h),
							formatOriginTag(caller.RemoteAddr),
							normalizeRemoteHost(caller.RemoteAddr),
							caller.Area,
							formatDuration(duration)))
					}
				}
			}
			if len(last) == 0 && s.nodes != nil {
				for _, caller := range s.nodes.LastCallers(25) {
					last = append(last, formatCallerTTYRow(renderWidth,
						caller.NodeID,
						caller.Username,
						formatClock(caller.LoginAt.Local(), sessionTime24h),
						formatOriginTag(caller.RemoteAddr),
						normalizeRemoteHost(caller.RemoteAddr),
						caller.Area,
						formatDuration(caller.Duration)))
				}
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderLastCallers(renderWidth, last), sessionANSI, sessionEncoding)
			_, _ = readKey(reader)
			touch()
			state = stateMainMenu
		case stateWhoOnline:
			setArea("Who's Online")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName(), currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			onlineRows := []string{}
			if s.admin != nil {
				if sessions, err := s.admin.ListNodeSessions(64); err == nil {
					now := time.Now().UTC()
					for _, online := range sessions {
						idle := now.Sub(online.LastActivity)
						if idle < 0 {
							idle = 0
						}
						onlineRows = append(onlineRows, formatCallerTTYRow(renderWidth,
							online.NodeID,
							online.Username,
							formatClock(online.LoginAt.Local(), sessionTime24h),
							formatOriginTag(online.RemoteAddr),
							normalizeRemoteHost(online.RemoteAddr),
							online.Area,
							formatDuration(idle)))
					}
				}
			}
			if len(onlineRows) == 0 && s.nodes != nil {
				for _, online := range s.nodes.Online() {
					onlineRows = append(onlineRows, formatCallerTTYRow(renderWidth,
						online.NodeID,
						online.Username,
						formatClock(online.LoginAt.Local(), sessionTime24h),
						formatOriginTag(online.RemoteAddr),
						normalizeRemoteHost(online.RemoteAddr),
						online.Area,
						formatDuration(time.Duration(online.IdleSeconds)*time.Second)))
				}
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderWhoOnline(renderWidth, onlineRows), sessionANSI, sessionEncoding)
			_, _ = readKey(reader)
			touch()
			state = stateMainMenu
		case stateConfigCenter:
			setArea("Config Center")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Config Center", currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			cfgLines := []string{
				fmt.Sprintf("Theme: %s", currentAccount.Theme),
				fmt.Sprintf("ANSI enabled: %s", boolText(currentAccount.ANSIEnabled)),
				fmt.Sprintf("Paging enabled: %s", boolText(currentAccount.PagingEnabled)),
				fmt.Sprintf("24h clock: %s", boolText(currentAccount.TimeFormat24h)),
				fmt.Sprintf("ACS strict: %s", boolText(s.flagFromConfig("runtime.acs.strict", "WOLFBBS_ACS_STRICT", false))),
				fmt.Sprintf("Guest tour enabled: %s", boolText(s.guestTourEnabled())),
				fmt.Sprintf("Smart newscan enabled: %s", boolText(s.smartNewscanEnabled())),
				fmt.Sprintf("Quick jump enabled: %s", boolText(s.quickJumpEnabled())),
				fmt.Sprintf("Classic search enabled: %s", boolText(s.classicSearchEnabled())),
				fmt.Sprintf("Trusted proxies: %s", s.textFromConfig("runtime.login.trusted_proxies", "WOLFBBS_TRUSTED_PROXIES", "(none)")),
				fmt.Sprintf("Telnet login: %s (%s)", boolText(s.flagFromConfig("runtime.login.telnet.enabled", "WOLFBBS_TELNET_ENABLE", false)), s.textFromConfig("runtime.login.telnet.listen", "WOLFBBS_TELNET_LISTEN", ":2323")),
				fmt.Sprintf("WebSocket login: %s (%s%s)", boolText(s.flagFromConfig("runtime.login.ws.enabled", "WOLFBBS_WS_ENABLE", false)), s.textFromConfig("runtime.login.ws.listen", "WOLFBBS_WS_LISTEN", ":6080"), s.textFromConfig("runtime.login.ws.path", "WOLFBBS_WS_PATH", "/ws-login")),
				fmt.Sprintf("WebSocket TLS: %s (%s%s)", boolText(s.flagFromConfig("runtime.login.wss.enabled", "WOLFBBS_WSS_ENABLE", false)), s.textFromConfig("runtime.login.wss.listen", "WOLFBBS_WSS_LISTEN", ":6443"), s.textFromConfig("runtime.login.wss.path", "WOLFBBS_WSS_PATH", "/ws-login")),
				"",
				"User preferences: press S at main menu for Settings.",
				"Sysop runtime flags: /admin/config on web panel.",
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderConfigCenter(renderWidth, cfgLines), sessionANSI, sessionEncoding)
			_, _ = readKey(reader)
			touch()
			state = stateMainMenu
		case stateStatusCenter:
			setArea("Status Center")
			touch()
			writeClear(sess, sessionANSI)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Status Center", currentUser, time.Now(), nodeLabel, th, sessionTime24h)+"\r\n", sessionANSI, sessionEncoding)
			onlineCount := 0
			if s.nodes != nil {
				onlineCount = len(s.nodes.Online())
			}
			chatChannels := 0
			if s.chatSvc != nil {
				chatChannels = len(s.chatSvc.ListChannels())
			}
			statusLines := []string{
				fmt.Sprintf("Node: %s", nodeLabel),
				fmt.Sprintf("Session: %s", clampForTTY(sessionID, 18)),
				fmt.Sprintf("Remote origin: %s", strings.ToUpper(remoteOrigin)),
				fmt.Sprintf("Remote host: %s", remoteHost),
				fmt.Sprintf("Role: %s", currentAccount.Role),
				fmt.Sprintf("Current area: %s", currentArea),
				fmt.Sprintf("Boards backend ready: %s", boolText(s.boards != nil && s.msgs != nil)),
				fmt.Sprintf("Mail backend ready: %s", boolText(s.mail != nil)),
				fmt.Sprintf("Chat backend ready: %s", boolText(s.chatSvc != nil)),
				fmt.Sprintf("Doors registry ready: %s", boolText(s.doors != nil)),
				fmt.Sprintf("Chat channels: %d", chatChannels),
				fmt.Sprintf("Online callers: %d", onlineCount),
				fmt.Sprintf("Guest tour state: %s", boolText(s.guestTourEnabled())),
				fmt.Sprintf("Smart newscan state: %s", boolText(s.smartNewscanEnabled())),
				fmt.Sprintf("Discover feed state: %s", boolText(s.discoverEnabled())),
				fmt.Sprintf("Quick jump state: %s", boolText(s.quickJumpEnabled())),
				fmt.Sprintf("Classic search state: %s", boolText(s.classicSearchEnabled())),
				fmt.Sprintf("Telnet login enabled: %s", boolText(s.flagFromConfig("runtime.login.telnet.enabled", "WOLFBBS_TELNET_ENABLE", false))),
				fmt.Sprintf("WebSocket login enabled: %s", boolText(s.flagFromConfig("runtime.login.ws.enabled", "WOLFBBS_WS_ENABLE", false))),
				fmt.Sprintf("WebSocket TLS enabled: %s", boolText(s.flagFromConfig("runtime.login.wss.enabled", "WOLFBBS_WSS_ENABLE", false))),
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderStatusCenter(renderWidth, statusLines), sessionANSI, sessionEncoding)
			_, _ = readKey(reader)
			touch()
			state = stateMainMenu
		case stateExit:
			writeClear(sess, sessionANSI)
			io.WriteString(sess, "Signing off "+s.siteName()+"...\r\n")
			return
		}
	}
}

func renderFrame(out io.Writer, termWidth, contentWidth int, frame string, ansiEnabled bool, encoding string) {
	frame = ui.ApplyOutputProfile(frame, ansiEnabled, encoding)
	frame = strings.ReplaceAll(frame, "\r\n", "\n")
	frame = strings.TrimSuffix(frame, "\n")
	if termWidth <= contentWidth {
		io.WriteString(out, frame)
		return
	}
	padding := (termWidth - contentWidth) / 2
	if padding <= 0 {
		io.WriteString(out, frame)
		return
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", padding) + line
	}
	_, _ = io.WriteString(out, strings.Join(lines, "\r\n"))
}

func showHelpPanel(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, area, user, nodeLabel string, th ui.Theme, time24h bool, ansiEnabled bool, encoding string, panel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	writeClear(sess, ansiEnabled)
	renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, area, user, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
	renderFrame(sess, termWidth, renderWidth, panel, ansiEnabled, encoding)
	_, _ = readKey(reader)
	touch()
}

func writeClear(out io.Writer, ansiEnabled bool) {
	if ansiEnabled {
		_, _ = io.WriteString(out, ui.ClearScreen())
		return
	}
	_, _ = io.WriteString(out, "\r\n")
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func truncateText(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func doorSpotlightLabel(items []ui.DoorMenuItem) string {
	if len(items) == 0 {
		return ""
	}
	best := items[0]
	bestScore := -1
	for _, row := range items {
		score := 0
		if row.Favorite {
			score += 100
		}
		if row.TurnsRemaining > 0 {
			score += row.TurnsRemaining * 3
		}
		if score > bestScore {
			best = row
			bestScore = score
		}
	}
	label := best.Name + " [" + strings.ToUpper(best.Hotkey) + "]"
	if best.TurnsRemaining > 0 {
		label += fmt.Sprintf(" • %d turns", best.TurnsRemaining)
	}
	return label
}

func pagerWrite(out io.Writer, reader *bufio.Reader, text string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	const pageSize = 16
	for i := 0; i < len(lines); i += pageSize {
		chunkEnd := i + pageSize
		if chunkEnd > len(lines) {
			chunkEnd = len(lines)
		}
		io.WriteString(out, strings.Join(lines[i:chunkEnd], "\r\n"))
		if chunkEnd >= len(lines) {
			break
		}
		io.WriteString(out, "\r\n"+ui.FooterPrompt(80, "More")+"\r\n")
		key, err := readKey(reader)
		if err != nil || key == "Q" || key == "ESC" {
			return
		}
	}
}

func readLine(reader *bufio.Reader, max int) (string, error) {
	if max <= 0 {
		max = 80
	}
	var b strings.Builder
	for {
		ch, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		if ch == '\r' || ch == '\n' {
			return b.String(), nil
		}
		if ch == 0x7f || ch == 0x08 {
			if b.Len() > 0 {
				cur := b.String()
				if len(cur) > 0 {
					cur = cur[:len(cur)-1]
					b.Reset()
					_, _ = b.WriteString(cur)
				}
			}
			continue
		}
		if b.Len() >= max {
			continue
		}
		if ch >= 32 && ch <= 126 {
			b.WriteByte(ch)
		}
	}
}

func readKey(reader *bufio.Reader) (string, error) {
	ch, err := reader.ReadByte()
	if err != nil {
		return "", err
	}
	readBufferedByte := func() (byte, bool) {
		if reader.Buffered() == 0 {
			return 0, false
		}
		next, err := reader.ReadByte()
		if err != nil {
			return 0, false
		}
		return next, true
	}
	switch ch {
	case 0x1b:
		next, ok := readBufferedByte()
		if !ok {
			return "ESC", nil
		}
		if next != '[' {
			return "ESC", nil
		}
		next2, ok := readBufferedByte()
		if !ok {
			return "ESC", nil
		}
		switch next2 {
		case 'A':
			return "UP", nil
		case 'B':
			return "DOWN", nil
		case 'C':
			return "RIGHT", nil
		case 'D':
			return "LEFT", nil
		case 'H':
			return "HOME", nil
		case 'F':
			return "END", nil
		case '5':
			p, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if p == '~' {
				return "PGUP", nil
			}
		case '6':
			p, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if p == '~' {
				return "PGDN", nil
			}
		case '1':
			mid, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if mid != ';' {
				return "UNKNOWN", nil
			}
			tail, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			next, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if tail == '5' && next == '~' {
				return "HOME", nil
			}
			if tail == '6' && next == '~' {
				return "END", nil
			}
		case '3':
			t, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if t == '~' {
				return "DEL", nil
			}
		case '4':
			t, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if t == '~' {
				return "END", nil
			}
		case '2':
			t, ok := readBufferedByte()
			if !ok {
				return "ESC", nil
			}
			if t == '~' {
				return "INSERT", nil
			}
		}
		return "ESC", nil
	case '\n', '\r':
		return "ENTER", nil
	case '\x03':
		return "CTRL-C", nil
	case '?':
		return "?", nil
	default:
		if ch >= 32 && ch <= 126 {
			return strings.ToUpper(string(ch)), nil
		}
		return strconv.Itoa(int(ch)), nil
	}
}

func legacyHotkeyToAction(key string) string {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "Q", "ESC":
		return "session.quit"
	case "N":
		return "system.newscan"
	case "M":
		return "boards.open"
	case "P":
		return "mail.open"
	case "C":
		return "chat.open"
	case "G":
		return "gateway.open"
	case "D":
		return "doors.open"
	case "L":
		return "system.last_callers"
	case "W":
		return "system.who_online"
	case "X":
		return "system.config_center"
	case "Y":
		return "system.status_center"
	case "/":
		return "system.quick_jump"
	case "F":
		return "files.open"
	case "S":
		return "settings.open"
	case "A":
		return "admin.open"
	default:
		return ""
	}
}

func quickJumpToAction(raw string) string {
	target := strings.ToLower(strings.TrimSpace(raw))
	switch target {
	case "n", "new", "newscan", "bulletins":
		return "system.newscan"
	case "m", "msg", "msgs", "boards", "messages":
		return "boards.open"
	case "p", "pm", "mail", "private":
		return "mail.open"
	case "c", "chat":
		return "chat.open"
	case "g", "gateway", "web":
		return "gateway.open"
	case "d", "door", "doors":
		return "doors.open"
	case "s", "settings", "prefs":
		return "settings.open"
	case "l", "last", "callers":
		return "system.last_callers"
	case "w", "who", "online":
		return "system.who_online"
	case "x", "config", "config-center":
		return "system.config_center"
	case "y", "status", "status-center":
		return "system.status_center"
	case "a", "admin", "sysop":
		return "admin.open"
	case "f", "files":
		return "files.open"
	case "u", "upgrade", "update", "app", "app-upgrade", "app upgrade", "/app", "/app upgrade":
		return "system.app_upgrade"
	case "q", "quit", "exit":
		return "session.quit"
	default:
		return ""
	}
}

func appUpgradeTimeout() time.Duration {
	timeout := 15 * time.Minute
	raw := strings.TrimSpace(os.Getenv(appUpgradeTimeoutEnv))
	if raw == "" {
		return timeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 15 || seconds > 3600 {
		return timeout
	}
	return time.Duration(seconds) * time.Second
}

func executeAppUpgradeCommand(handle string) (string, error) {
	command := strings.TrimSpace(os.Getenv(appUpgradeCommandEnv))
	if command == "" {
		return "", errors.New(appUpgradeCommandEnv + " is not configured")
	}
	timeout := appUpgradeTimeout()
	workDir := strings.TrimSpace(os.Getenv(appUpgradeWorkDirEnv))
	env := append(os.Environ(),
		"WOLFBBS_UPGRADE_TRIGGER=ssh",
		"WOLFBBS_UPGRADE_USER="+strings.TrimSpace(handle),
	)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := appUpgradeExec(ctx, command, workDir, env)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return output, fmt.Errorf("upgrade timed out after %s", timeout)
	}
	if err != nil {
		return output, fmt.Errorf("upgrade command failed: %w", err)
	}
	return output, nil
}

func limitCommandOutput(raw string, maxRunes int) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if maxRunes <= 0 {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return string(runes[:maxRunes]) + "\n...[truncated]"
}

func (s *Server) runAppUpgrade(sess gssh.Session, reader *bufio.Reader, handle string, account *domain.User, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	if !evaluateAccess("role=sysop", account, handle, map[string]string{"area": "system", "mode": "app_upgrade"}, true, s.logger) {
		io.WriteString(sess, "\r\n/app upgrade is sysop-only. Press any key.")
		_, _ = readKey(reader)
		touch()
		return
	}
	if strings.TrimSpace(os.Getenv(appUpgradeCommandEnv)) == "" {
		io.WriteString(sess, "\r\nApp upgrade is not configured.")
		io.WriteString(sess, "\r\nSet "+appUpgradeCommandEnv+" (example: bash install.sh --rapid-upgrade --yes).")
		io.WriteString(sess, "\r\nPress any key.")
		_, _ = readKey(reader)
		touch()
		return
	}

	io.WriteString(sess, "\r\nRun /app upgrade now? [y/N]: ")
	answer, err := readLine(reader, 8)
	if err != nil {
		return
	}
	touch()
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		io.WriteString(sess, "\r\nUpgrade canceled. Press any key.")
		_, _ = readKey(reader)
		touch()
		return
	}

	io.WriteString(sess, "\r\nRunning upgrade command. Session may disconnect if services restart.\r\n")
	output, runErr := executeAppUpgradeCommand(handle)
	if output != "" {
		io.WriteString(sess, "\r\nCommand output:\r\n")
		io.WriteString(sess, strings.ReplaceAll(limitCommandOutput(output, 1600), "\n", "\r\n"))
		io.WriteString(sess, "\r\n")
	}
	if runErr != nil {
		io.WriteString(sess, "\r\nUpgrade failed: "+runErr.Error()+"\r\nPress any key.")
		_, _ = readKey(reader)
		touch()
		return
	}
	io.WriteString(sess, "\r\nUpgrade command completed. Press any key.")
	_, _ = readKey(reader)
	touch()
}

func (s *Server) guestTourLines(time24h bool) []string {
	lines := []string{
		"Welcome to " + s.siteName() + ". This mode is read-only.",
		"Create an account from Login to post and chat.",
		"",
		"Last callers:",
	}
	if s.nodes != nil {
		last := s.nodes.LastCallers(3)
		if len(last) == 0 {
			lines = append(lines, "- No caller history yet.")
		}
		for _, row := range last {
			lines = append(lines, fmt.Sprintf("- Node %02d %-12s %s",
				row.NodeID,
				clampForTTY(row.Username, 12),
				formatClock(row.LoginAt.Local(), time24h)))
		}
	} else {
		lines = append(lines, "- Node tracking unavailable.")
	}

	lines = append(lines, "", "One-liners (#lobby):")
	if s.chatSvc != nil {
		chatRows := s.chatSvc.History("#lobby", 3)
		if len(chatRows) == 0 {
			lines = append(lines, "- No chat lines yet.")
		}
		for _, row := range chatRows {
			stamp := row.CreatedAt.Local().Format("15:04")
			if !time24h {
				stamp = row.CreatedAt.Local().Format("03:04PM")
			}
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", stamp, clampForTTY(row.From, 12), clampForTTY(row.Body, 40)))
		}
	} else {
		lines = append(lines, "- Chat service unavailable.")
	}

	lines = append(lines, "", "Featured thread:")
	if thread := s.latestThreadHeadline(); thread != "" {
		lines = append(lines, "- "+thread)
	} else {
		lines = append(lines, "- No messages yet. Be the first caller to post.")
	}
	lines = append(lines, "", "Today's download pick:")
	lines = append(lines, "- FileBase Pro door (X): check recent uploads and tags.")
	return lines
}

func (s *Server) latestThreadHeadline() string {
	if s.boards == nil || s.msgs == nil {
		return ""
	}
	boards, err := s.boards.List()
	if err != nil {
		return ""
	}
	var selected *domain.Message
	boardName := ""
	for _, board := range boards {
		msgs, err := s.msgs.ListByBoard(board.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		last := msgs[len(msgs)-1]
		if selected == nil || last.CreatedAt.After(selected.CreatedAt) {
			copy := last
			selected = &copy
			boardName = board.Name
		}
	}
	if selected == nil {
		return ""
	}
	return fmt.Sprintf("%s / %s", clampForTTY(boardName, 18), clampForTTY(selected.Subject, 42))
}

func isGuestTourHandle(handle string) bool {
	handle = strings.ToUpper(strings.TrimSpace(handle))
	return handle == "GUEST" || handle == "TOUR" || handle == "G"
}

func (s *Server) flagFromConfig(settingKey, envKey string, fallback bool) bool {
	if s != nil && s.admin != nil {
		if raw, err := s.admin.GetSystemSetting(settingKey); err == nil && strings.TrimSpace(raw) != "" {
			return envBool(raw)
		}
	}
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return fallback
	}
	return envBool(raw)
}

func (s *Server) textFromConfig(settingKey, envKey, fallback string) string {
	if s != nil && s.admin != nil {
		if raw, err := s.admin.GetSystemSetting(settingKey); err == nil {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				return raw
			}
		}
	}
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return fallback
	}
	return raw
}

func (s *Server) siteName() string {
	return s.textFromConfig("site.name", "WOLFBBS_BBS_NAME", "WolfBBS")
}

func (s *Server) requireVerifiedEmail() bool {
	return s.flagFromConfig("mail.require_verified", "WOLFBBS_REQUIRE_VERIFIED_EMAIL", true)
}

func (s *Server) guestTourEnabled() bool {
	return s.flagFromConfig("site.guest_tour_enable", "WOLFBBS_GUEST_TOUR_ENABLE", false)
}

func (s *Server) smartNewscanEnabled() bool {
	return s.flagFromConfig("site.discover_enable", "WOLFBBS_SMART_NEWSCAN_ENABLE", true)
}

func (s *Server) discoverEnabled() bool {
	return s.flagFromConfig("site.discover_enable", "WOLFBBS_DISCOVER_ENABLE", true)
}

func (s *Server) quickJumpEnabled() bool {
	return s.flagFromConfig("site.quick_jump_enable", "WOLFBBS_QUICK_JUMP_ENABLE", false)
}

func (s *Server) classicSearchEnabled() bool {
	return s.flagFromConfig("site.classic_search_enable", "WOLFBBS_CLASSIC_SEARCH_ENABLE", false)
}

func smartNewscanMaxItems() int {
	raw := strings.TrimSpace(os.Getenv("WOLFBBS_SMART_NEWSCAN_MAX"))
	if raw == "" {
		return 12
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 12
	}
	if v > 30 {
		return 30
	}
	return v
}

func (s *Server) aiAssistEnabled() bool {
	return s.flagFromConfig("site.ai_assist_enable", "WOLFBBS_AI_ASSIST_ENABLE", false)
}

func envBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Server) publishEvent(name string, fields map[string]string) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(name, fields)
}

func evaluateAccess(expr string, user *domain.User, fallbackHandle string, attrs map[string]string, strict bool, logger *slog.Logger) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	role := "user"
	verified := false
	handle := strings.TrimSpace(fallbackHandle)
	if user != nil {
		if strings.TrimSpace(user.Role) != "" {
			role = strings.ToLower(strings.TrimSpace(user.Role))
		}
		verified = user.Verified
		if strings.TrimSpace(user.Handle) != "" {
			handle = user.Handle
		}
	}
	attrs["role"] = role
	attrs["verified"] = boolText(verified)
	attrs["handle"] = handle
	allowed, err := acs.Evaluate(expr, acs.Context{
		Role:     role,
		Verified: verified,
		Attrs:    attrs,
	})
	if err != nil {
		if logger != nil {
			logger.Warn("acs evaluation failed", "expr", expr, "error", err)
		}
		return !strict
	}
	return allowed
}

func (s *Server) runChat(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.chatSvc == nil {
		io.WriteString(sess, "\r\nChat service is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	if touch == nil {
		touch = func() {}
	}
	channel := "#lobby"
	s.chatSvc.JoinChannel(handle, channel)
	defer s.chatSvc.LeaveChannel(handle, channel)

	for {
		touch()
		history := s.chatSvc.History(channel, 12)
		online := s.chatSvc.OnlineInChannel(channel)
		lines := []string{
			fmt.Sprintf("Channel: %s   Online: %d", channel, len(online)),
			strings.Repeat("-", 62),
		}
		if len(history) == 0 {
			lines = append(lines, "No messages yet.")
		} else {
			for _, msg := range history {
				stamp := msg.CreatedAt.Local().Format("15:04")
				if !time24h {
					stamp = msg.CreatedAt.Local().Format("03:04PM")
				}
				lines = append(lines, fmt.Sprintf("%-7s %-12s %s", stamp, clampForTTY(msg.From, 12), clampForTTY(msg.Body, 44)))
			}
		}
		lines = append(lines, "")
		lines = append(lines, "(S)end  (J)oin  (O)nline  (R)efresh  (?)Help  (Q)uit")

		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Chat", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Live Chat", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Selection: ")
		key, err := readKey(reader)
		if err != nil {
			return
		}
		touch()
		switch key {
		case "Q", "ESC":
			return
		case "R", "ENTER":
			continue
		case "?":
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderChatHelp(renderWidth), touch)
		case "O":
			io.WriteString(sess, "\r\nOnline users:\r\n")
			if len(online) == 0 {
				io.WriteString(sess, "  none\r\n")
			}
			for _, p := range online {
				io.WriteString(sess, fmt.Sprintf("  %-14s idle:%4ds  area:%s\r\n", clampForTTY(p.Nick, 14), p.IdleSec, p.Area))
			}
			io.WriteString(sess, "Press any key.")
			_, _ = readKey(reader)
			touch()
		case "J":
			io.WriteString(sess, "\r\nJoin channel (#lobby): ")
			raw, err := readLine(reader, 32)
			if err != nil {
				return
			}
			touch()
			next := chat.NormalizeChannel(strings.TrimSpace(raw))
			if next == "" {
				next = "#lobby"
			}
			s.chatSvc.LeaveChannel(handle, channel)
			channel = next
			s.chatSvc.JoinChannel(handle, channel)
		case "S":
			io.WriteString(sess, "\r\nMessage: ")
			body, err := readLine(reader, 512)
			if err != nil {
				return
			}
			touch()
			body = strings.TrimSpace(body)
			if body == "" {
				io.WriteString(sess, "\r\nEmpty message. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if _, err := s.chatSvc.Post(handle, channel, body); err != nil {
				io.WriteString(sess, "\r\nSend blocked: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
			}
		default:
			io.WriteString(sess, "\r\nUnknown key. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func (s *Server) runBoards(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.boards == nil || s.msgs == nil {
		io.WriteString(sess, "\r\nMessage board storage is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	if touch == nil {
		touch = func() {}
	}
	currentUser, err := s.auth.GetUser(handle)
	if err != nil || currentUser == nil {
		io.WriteString(sess, "\r\nCould not load account for posting. Press any key.")
		_, _ = readKey(reader)
		return
	}
	acsStrict := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_STRICT")))
	if !evaluateAccess(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_READ")), currentUser, handle, map[string]string{"area": "boards", "mode": "read"}, acsStrict, s.logger) {
		io.WriteString(sess, "\r\nAccess denied by board read ACS rule. Press any key.")
		_, _ = readKey(reader)
		touch()
		return
	}
	conferenceFilter := ""
	for {
		boards, err := s.boards.List()
		if err != nil {
			io.WriteString(sess, "\r\nCould not load boards. Press any key.")
			_, _ = readKey(reader)
			return
		}
		if len(boards) == 0 {
			_ = s.boards.Create(&domain.Board{Name: "General", Description: "Default lobby board", CreatedBy: currentUser.ID})
			boards, _ = s.boards.List()
		}
		visibleBoards := make([]domain.Board, 0, len(boards))
		conferences := map[string]struct{}{}
		for _, board := range boards {
			if evaluateAccess(boardReadRule(board), currentUser, handle, map[string]string{
				"area":       "boards",
				"mode":       "read",
				"board_id":   strconv.FormatInt(board.ID, 10),
				"board":      board.Name,
				"conference": board.Conference,
			}, acsStrict, s.logger) {
				conferences[boardConference(board)] = struct{}{}
				visibleBoards = append(visibleBoards, board)
			}
		}
		boards = make([]domain.Board, 0, len(visibleBoards))
		for _, board := range visibleBoards {
			if conferenceFilter != "" && !strings.EqualFold(boardConference(board), conferenceFilter) {
				continue
			}
			boards = append(boards, board)
		}

		names := make([]string, 0, len(boards))
		for _, board := range boards {
			names = append(names, formatBoardListRow(renderWidth, board.ID, board.Name, boardConference(board)))
		}

		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Boards", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.RenderMessageBoardList(renderWidth, names)+"\r\n", ansiEnabled, encoding)
		confLabel := conferenceFilter
		if confLabel == "" {
			confLabel = "All"
		}
		io.WriteString(sess, "\r\nConference: "+confLabel+"  (C=change filter)")
		if len(boards) == 0 {
			io.WriteString(sess, "\r\nNo boards are currently readable for your account. Press any key.")
			_, _ = readKey(reader)
			touch()
			conferenceFilter = ""
			continue
		}
		io.WriteString(sess, "Select board ID (or ? help, Q return): ")
		raw, err := readLine(reader, 16)
		if err != nil {
			return
		}
		touch()
		raw = strings.TrimSpace(raw)
		if strings.EqualFold(raw, "q") {
			return
		}
		if strings.EqualFold(raw, "c") {
			confList := make([]string, 0, len(conferences))
			for row := range conferences {
				confList = append(confList, row)
			}
			sort.Slice(confList, func(i, j int) bool { return strings.ToLower(confList[i]) < strings.ToLower(confList[j]) })
			io.WriteString(sess, "\r\nConferences:\r\n  * All\r\n")
			for _, row := range confList {
				io.WriteString(sess, "  - "+row+"\r\n")
			}
			io.WriteString(sess, "Filter conference (blank = all): ")
			next, err := readLine(reader, 48)
			if err != nil {
				return
			}
			touch()
			conferenceFilter = strings.TrimSpace(next)
			continue
		}
		if raw == "?" {
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderBoardsHelp(renderWidth), touch)
			continue
		}
		boardID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			io.WriteString(sess, "\r\nInvalid board ID. Press any key.")
			_, _ = readKey(reader)
			continue
		}
		selected, err := s.boards.Get(boardID)
		if err != nil {
			io.WriteString(sess, "\r\nUnknown board. Press any key.")
			_, _ = readKey(reader)
			continue
		}
		if !evaluateAccess(boardReadRule(*selected), currentUser, handle, map[string]string{
			"area":       "boards",
			"mode":       "read",
			"board_id":   strconv.FormatInt(selected.ID, 10),
			"board":      selected.Name,
			"conference": selected.Conference,
		}, acsStrict, s.logger) {
			io.WriteString(sess, "\r\nAccess denied by board ACS rule. Press any key.")
			_, _ = readKey(reader)
			touch()
			continue
		}

		for {
			msgs, err := s.msgs.ListByBoard(boardID)
			if err != nil {
				io.WriteString(sess, "\r\nCould not load messages. Press any key.")
				_, _ = readKey(reader)
				break
			}
			pointerID := int64(0)
			if ptr, ptrErr := s.msgs.GetPointer(currentUser.ID, boardID); ptrErr == nil && ptr != nil {
				pointerID = ptr.LastReadID
			}
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, selected.Name, handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			rows := make([]string, 0, len(msgs))
			for _, msg := range msgs {
				threadMarker := " "
				if msg.ParentID > 0 {
					threadMarker = ">"
				}
				unread := " "
				if msg.ID > pointerID {
					unread = "N"
				}
				rows = append(rows, formatMessageIndexRow(renderWidth, msg.ID, unread, threadMarker, msg.Subject, msg.CreatedAt.Format("01-02 15:04")))
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderBoardMessageIndex(renderWidth, selected.Name, rows)+"\r\n", ansiEnabled, encoding)
			io.WriteString(sess, "> ")
			choice, err := readLine(reader, 24)
			if err != nil {
				return
			}
			touch()
			choice = strings.TrimSpace(choice)
			switch strings.ToUpper(choice) {
			case "Q":
				goto nextBoard
			case "S":
				if !s.classicSearchEnabled() {
					io.WriteString(sess, "\r\nClassic search is disabled by sysop. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				io.WriteString(sess, "Search query: ")
				query, err := readLine(reader, 72)
				if err != nil {
					return
				}
				touch()
				query = strings.ToLower(strings.TrimSpace(query))
				if query == "" {
					io.WriteString(sess, "\r\nSearch cancelled. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				results := make([]string, 0, 20)
				for _, msg := range msgs {
					haystack := strings.ToLower(msg.Subject + "\n" + msg.Body)
					if !strings.Contains(haystack, query) {
						continue
					}
					results = append(results, formatSearchResultRow(renderWidth, msg.ID, msg.Subject, msg.CreatedAt.Format("01-02 15:04")))
					if len(results) >= 20 {
						break
					}
				}
				writeClear(sess, ansiEnabled)
				renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, selected.Name+" / Search", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
				renderFrame(sess, termWidth, renderWidth, ui.RenderSearchResults(renderWidth, "Classic Search Results", results)+"\r\n", ansiEnabled, encoding)
				_, _ = readKey(reader)
				touch()
			case "N":
				if !evaluateAccess(boardWriteRule(*selected), currentUser, handle, map[string]string{
					"area":       "boards",
					"mode":       "post",
					"board_id":   strconv.FormatInt(selected.ID, 10),
					"board":      selected.Name,
					"conference": selected.Conference,
				}, acsStrict, s.logger) {
					io.WriteString(sess, "\r\nPosting denied by ACS rule. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				writeClear(sess, ansiEnabled)
				renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, selected.Name+" / New Post", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
				renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, "")+"\r\n", ansiEnabled, encoding)
				io.WriteString(sess, "Subject: ")
				subject, err := readLine(reader, 120)
				if err != nil {
					return
				}
				touch()
				body, err := readMessageBody(reader, 80, 2048)
				if err != nil {
					return
				}
				touch()
				subject = strings.TrimSpace(subject)
				body = strings.TrimSpace(body)
				if subject == "" || body == "" {
					io.WriteString(sess, "\r\nSubject/body required. Press any key.")
					_, _ = readKey(reader)
					continue
				}
				if err := s.msgs.CreateMessage(&domain.Message{
					BoardID:  boardID,
					AuthorID: currentUser.ID,
					Subject:  subject,
					Body:     body,
				}); err != nil {
					io.WriteString(sess, "\r\nPost failed: "+err.Error()+"\r\nPress any key.")
					_, _ = readKey(reader)
					touch()
				} else {
					s.publishEvent("message.posted", map[string]string{
						"user":    handle,
						"board":   strconv.FormatInt(boardID, 10),
						"subject": subject,
					})
				}
			case "R":
				if len(msgs) == 0 {
					io.WriteString(sess, "\r\nNo messages to read. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				io.WriteString(sess, "Message ID (blank = first): ")
				rawID, err := readLine(reader, 20)
				if err != nil {
					return
				}
				touch()
				startIndex := 0
				if trimmed := strings.TrimSpace(rawID); trimmed != "" {
					msgID, parseErr := strconv.ParseInt(trimmed, 10, 64)
					if parseErr != nil {
						io.WriteString(sess, "\r\nInvalid message ID. Press any key.")
						_, _ = readKey(reader)
						touch()
						continue
					}
					found := -1
					for i, msg := range msgs {
						if msg.ID == msgID {
							found = i
							break
						}
					}
					if found < 0 {
						io.WriteString(sess, "\r\nMessage not found. Press any key.")
						_, _ = readKey(reader)
						touch()
						continue
					}
					startIndex = found
				}
				posted, err := s.runBoardReader(sess, reader, termWidth, renderWidth, selected.Name, handle, currentUser, boardID, msgs, startIndex, boardWriteRule(*selected), acsStrict, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
				if err != nil {
					return
				}
				if posted {
					continue
				}
			case "?":
				showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderBoardsHelp(renderWidth), touch)
			default:
				io.WriteString(sess, "\r\nUnknown command. Press any key.")
				_, _ = readKey(reader)
				touch()
			}
		}
	nextBoard:
	}
}

func (s *Server) runBoardReader(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, boardName, handle string, currentUser *domain.User, boardID int64, msgs []domain.Message, startIndex int, writeRule string, acsStrict bool, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) (bool, error) {
	if len(msgs) == 0 {
		return false, nil
	}
	if touch == nil {
		touch = func() {}
	}
	if startIndex < 0 || startIndex >= len(msgs) {
		startIndex = 0
	}
	index := startIndex
	posted := false

	for {
		if index < 0 {
			index = 0
		}
		if index >= len(msgs) {
			index = len(msgs) - 1
		}
		msg := msgs[index]
		if currentUser != nil && currentUser.ID > 0 {
			_ = s.msgs.SetPointer(currentUser.ID, boardID, msg.ID, time.Now().UTC())
		}
		lines := strings.Split(strings.ReplaceAll(msg.Body, "\r\n", "\n"), "\n")
		title := fmt.Sprintf("#%d %s", msg.ID, msg.Subject)
		shouldPage := currentUser != nil && currentUser.PagingEnabled && len(lines) > 20
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, boardName, handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		if shouldPage {
			summary := []string{
				title,
				strings.Repeat("-", 30),
				"Long body detected; pager enabled.",
				"Space/Enter for more, Q/Esc to stop paging.",
			}
			renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(summary)+2, "Message Reader", summary, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
			io.WriteString(sess, "\r\n")
			pagerWrite(sess, reader, strings.Join(lines, "\n"))
			io.WriteString(sess, "\r\n")
			io.WriteString(sess, ui.FooterPrompt(renderWidth, fmt.Sprintf("(R)eply (N)ext (P)rev (Q)uit (?)Help   Msg %d/%d", index+1, len(msgs)))+"\r\n")
		} else {
			renderFrame(sess, termWidth, renderWidth, ui.RenderMessageReader(renderWidth, title, lines, index+1, len(msgs))+"\r\n", ansiEnabled, encoding)
		}
		io.WriteString(sess, "\r\nCommand (R/N/P/Q/?): ")
		key, err := readKey(reader)
		if err != nil {
			return posted, err
		}
		touch()
		switch key {
		case "N", "RIGHT", "PGDN", "ENTER", " ":
			if index+1 < len(msgs) {
				index++
			}
		case "P", "LEFT", "PGUP":
			if index > 0 {
				index--
			}
		case "R":
			if !evaluateAccess(writeRule, currentUser, handle, map[string]string{
				"area":     "boards",
				"mode":     "reply",
				"board_id": strconv.FormatInt(boardID, 10),
				"board":    boardName,
			}, acsStrict, s.logger) {
				io.WriteString(sess, "\r\nReply denied by ACS rule. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			defaultSubject := "Re: " + msg.Subject
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, boardName+" / Reply", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, defaultSubject)+"\r\n", ansiEnabled, encoding)
			io.WriteString(sess, "\r\nReply subject ["+defaultSubject+"]: ")
			replySubject, err := readLine(reader, 120)
			if err != nil {
				return posted, err
			}
			touch()
			replySubject = strings.TrimSpace(replySubject)
			if replySubject == "" {
				replySubject = defaultSubject
			}
			io.WriteString(sess, "\r\nEnter reply body, end with blank line then '.'\r\n")
			replyBody, err := readMessageBody(reader, 80, 4096)
			if err != nil {
				return posted, err
			}
			touch()
			replyBody = strings.TrimSpace(replyBody)
			if replyBody == "" {
				io.WriteString(sess, "\r\nReply cancelled (empty body). Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			replyBody = strings.TrimSpace(replyBody + "\n\n" + quoteMessage(msg.Body))
			if err := s.msgs.CreateMessage(&domain.Message{
				BoardID:  boardID,
				AuthorID: currentUser.ID,
				ParentID: msg.ID,
				Subject:  replySubject,
				Body:     replyBody,
			}); err != nil {
				io.WriteString(sess, "\r\nReply failed: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			s.publishEvent("message.reply_posted", map[string]string{
				"user":      handle,
				"board":     strconv.FormatInt(boardID, 10),
				"parent_id": strconv.FormatInt(msg.ID, 10),
			})
			posted = true
			return posted, nil
		case "?", "H":
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderBoardsHelp(renderWidth), touch)
		case "Q", "ESC":
			return posted, nil
		default:
			io.WriteString(sess, "\r\nUnknown command. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func (s *Server) runMail(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.mail == nil {
		io.WriteString(sess, "\r\nMail storage is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	if touch == nil {
		touch = func() {}
	}
	currentUser, err := s.auth.GetUser(handle)
	if err != nil || currentUser == nil {
		io.WriteString(sess, "\r\nCould not load account. Press any key.")
		_, _ = readKey(reader)
		return
	}
	acsStrict := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_STRICT")))
	if !evaluateAccess(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_MAIL_READ")), currentUser, handle, map[string]string{"area": "mail", "mode": "read"}, acsStrict, s.logger) {
		io.WriteString(sess, "\r\nAccess denied by mail read ACS rule. Press any key.")
		_, _ = readKey(reader)
		touch()
		return
	}
	for {
		inbox, _ := s.mail.ListInbox(currentUser.ID, 50)
		outbox, _ := s.mail.ListOutbox(currentUser.ID, 50)

		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Mail", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		inboxRows := make([]string, 0, len(inbox))
		for _, row := range inbox {
			status := "new"
			if row.ReadAt != nil {
				status = "read"
			}
			inboxRows = append(inboxRows, formatMailInboxRow(renderWidth, row.ID, row.Subject, row.CreatedAt.Format("01-02 15:04"), status))
		}
		outboxRows := make([]string, 0, len(outbox))
		for _, row := range outbox {
			target := fmt.Sprintf("uid:%d", row.ToUserID)
			if row.ExternalTo != nil {
				target = *row.ExternalTo
			}
			outboxRows = append(outboxRows, formatMailOutboxRow(renderWidth, row.ID, row.Subject, target))
		}
		renderFrame(sess, termWidth, renderWidth, ui.RenderMailOverview(renderWidth, inboxRows, outboxRows)+"\r\n", ansiEnabled, encoding)
		io.WriteString(sess, "> ")
		choice, err := readLine(reader, 24)
		if err != nil {
			return
		}
		touch()
		switch strings.ToUpper(strings.TrimSpace(choice)) {
		case "Q":
			return
		case "?":
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderMailHelp(renderWidth), touch)
		case "C":
			if !evaluateAccess(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_MAIL_SEND")), currentUser, handle, map[string]string{"area": "mail", "mode": "compose"}, acsStrict, s.logger) {
				io.WriteString(sess, "\r\nSending mail denied by ACS rule. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Mail Compose", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, "")+"\r\n", ansiEnabled, encoding)
			io.WriteString(sess, "To handle or external email: ")
			to, err := readLine(reader, 128)
			if err != nil {
				return
			}
			touch()
			io.WriteString(sess, "Subject: ")
			subject, err := readLine(reader, 120)
			if err != nil {
				return
			}
			touch()
			body, err := readMessageBody(reader, 80, 4096)
			if err != nil {
				return
			}
			touch()
			to = strings.TrimSpace(to)
			subject = strings.TrimSpace(subject)
			body = strings.TrimSpace(body)
			if to == "" || subject == "" || body == "" {
				io.WriteString(sess, "\r\nTo/subject/body are required. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			var msg domain.PrivateMail
			msg.FromUserID = currentUser.ID
			msg.Subject = subject
			msg.Body = body
			if strings.Contains(to, "@") {
				msg.ExternalTo = &to
			} else {
				targetUser, err := s.auth.GetUser(to)
				if err != nil || targetUser == nil {
					io.WriteString(sess, "\r\nUnknown recipient handle. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				msg.ToUserID = targetUser.ID
			}
			if err := s.mail.CreateMail(&msg); err != nil {
				io.WriteString(sess, "\r\nCould not send mail: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			s.publishEvent("mail.sent", map[string]string{
				"user": handle,
				"to":   to,
			})
		case "P":
			io.WriteString(sess, "Reply to Mail ID: ")
			rawID, err := readLine(reader, 16)
			if err != nil {
				return
			}
			mailID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
			if err != nil {
				io.WriteString(sess, "\r\nInvalid mail ID. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			row, err := s.mail.GetMail(mailID)
			if err != nil {
				io.WriteString(sess, "\r\nMail not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if row.ToUserID != currentUser.ID && row.FromUserID != currentUser.ID {
				io.WriteString(sess, "\r\nNot authorized to reply to that mail. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			var toUserID int64
			var externalTo *string
			if row.ToUserID == currentUser.ID {
				toUserID = row.FromUserID
			} else {
				toUserID = row.ToUserID
				if row.ExternalTo != nil {
					trimmed := strings.TrimSpace(*row.ExternalTo)
					if trimmed != "" {
						externalTo = &trimmed
					}
				}
			}
			if toUserID <= 0 && externalTo == nil {
				io.WriteString(sess, "\r\nCould not determine reply target. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			subject := strings.TrimSpace(row.Subject)
			if subject == "" {
				subject = "Re: (no subject)"
			} else if !strings.HasPrefix(strings.ToLower(subject), "re:") {
				subject = "Re: " + subject
			}
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Mail Reply", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, subject)+"\r\n", ansiEnabled, encoding)
			io.WriteString(sess, "Subject ["+subject+"]: ")
			inputSubject, err := readLine(reader, 120)
			if err != nil {
				return
			}
			touch()
			inputSubject = strings.TrimSpace(inputSubject)
			if inputSubject != "" {
				subject = inputSubject
			}
			io.WriteString(sess, "\r\nEnter reply body, end with '.' on a line by itself.\r\n")
			replyBody, err := readMessageBody(reader, 80, 4096)
			if err != nil {
				return
			}
			touch()
			replyBody = strings.TrimSpace(replyBody)
			if replyBody == "" {
				io.WriteString(sess, "\r\nReply body is required. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			replyBody = strings.TrimSpace(replyBody + "\n\n" + quoteMessage(row.Body))
			reply := &domain.PrivateMail{
				FromUserID: currentUser.ID,
				ToUserID:   toUserID,
				ExternalTo: externalTo,
				Subject:    subject,
				Body:       replyBody,
			}
			if err := s.mail.CreateMail(reply); err != nil {
				io.WriteString(sess, "\r\nCould not send reply: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			s.publishEvent("mail.reply", map[string]string{
				"user": handle,
				"id":   strconv.FormatInt(reply.ID, 10),
			})
			io.WriteString(sess, "\r\nReply sent. Press any key.")
			_, _ = readKey(reader)
			touch()
		case "R":
			io.WriteString(sess, "Mail ID: ")
			rawID, err := readLine(reader, 16)
			if err != nil {
				return
			}
			mailID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
			if err != nil {
				io.WriteString(sess, "\r\nInvalid mail ID. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			row, err := s.mail.GetMail(mailID)
			if err != nil {
				io.WriteString(sess, "\r\nMail not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if row.ToUserID != currentUser.ID && row.FromUserID != currentUser.ID {
				io.WriteString(sess, "\r\nNot authorized to read that mail. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if row.ToUserID == currentUser.ID {
				_ = s.mail.MarkRead(row.ID, time.Now().UTC())
			}
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Private Mail", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			io.WriteString(sess, "Subject: "+row.Subject+"\r\n")
			io.WriteString(sess, "Sent: "+row.CreatedAt.Format("2006-01-02 15:04:05")+"\r\n")
			if row.ExternalTo != nil {
				io.WriteString(sess, "External To: "+*row.ExternalTo+"\r\n")
			}
			io.WriteString(sess, "\r\n"+row.Body+"\r\n")
			io.WriteString(sess, "\r\nReader commands: (P) reply  (D) delete  (Q) back")
			key, keyErr := readKey(reader)
			if keyErr != nil {
				return
			}
			touch()
			switch strings.ToUpper(strings.TrimSpace(key)) {
			case "P":
				replySubject := strings.TrimSpace(row.Subject)
				if replySubject == "" {
					replySubject = "Re: (no subject)"
				} else if !strings.HasPrefix(strings.ToLower(replySubject), "re:") {
					replySubject = "Re: " + replySubject
				}
				var toUserID int64
				var externalTo *string
				if row.ToUserID == currentUser.ID {
					toUserID = row.FromUserID
				} else {
					toUserID = row.ToUserID
					if row.ExternalTo != nil {
						trimmed := strings.TrimSpace(*row.ExternalTo)
						if trimmed != "" {
							externalTo = &trimmed
						}
					}
				}
				if toUserID <= 0 && externalTo == nil {
					io.WriteString(sess, "\r\nCould not determine reply target. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				io.WriteString(sess, "\r\nSubject ["+replySubject+"]: ")
				override, err := readLine(reader, 120)
				if err != nil {
					return
				}
				touch()
				override = strings.TrimSpace(override)
				if override != "" {
					replySubject = override
				}
				io.WriteString(sess, "\r\nEnter reply body, end with '.' on a line by itself.\r\n")
				replyBody, err := readMessageBody(reader, 80, 4096)
				if err != nil {
					return
				}
				touch()
				replyBody = strings.TrimSpace(replyBody)
				if replyBody == "" {
					io.WriteString(sess, "\r\nReply body is required. Press any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				replyBody = strings.TrimSpace(replyBody + "\n\n" + quoteMessage(row.Body))
				reply := &domain.PrivateMail{
					FromUserID: currentUser.ID,
					ToUserID:   toUserID,
					ExternalTo: externalTo,
					Subject:    replySubject,
					Body:       replyBody,
				}
				if err := s.mail.CreateMail(reply); err != nil {
					io.WriteString(sess, "\r\nCould not send reply: "+err.Error()+"\r\nPress any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				s.publishEvent("mail.reply", map[string]string{
					"user": handle,
					"id":   strconv.FormatInt(reply.ID, 10),
				})
				io.WriteString(sess, "\r\nReply sent. Press any key.")
				_, _ = readKey(reader)
				touch()
			case "D":
				if err := s.mail.DeleteMail(row.ID); err != nil {
					io.WriteString(sess, "\r\nCould not delete mail: "+err.Error()+"\r\nPress any key.")
					_, _ = readKey(reader)
					touch()
					continue
				}
				s.publishEvent("mail.deleted", map[string]string{
					"user": handle,
					"id":   strconv.FormatInt(row.ID, 10),
				})
				io.WriteString(sess, "\r\nMail deleted. Press any key.")
				_, _ = readKey(reader)
				touch()
			default:
				// Return to mail menu.
			}
		case "D":
			io.WriteString(sess, "Delete Mail ID: ")
			rawID, err := readLine(reader, 16)
			if err != nil {
				return
			}
			mailID, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
			if err != nil {
				io.WriteString(sess, "\r\nInvalid mail ID. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			row, err := s.mail.GetMail(mailID)
			if err != nil {
				io.WriteString(sess, "\r\nMail not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if row.ToUserID != currentUser.ID && row.FromUserID != currentUser.ID {
				io.WriteString(sess, "\r\nNot authorized to delete that mail. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if err := s.mail.DeleteMail(mailID); err != nil {
				io.WriteString(sess, "\r\nCould not delete mail: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			s.publishEvent("mail.deleted", map[string]string{
				"user": handle,
				"id":   strconv.FormatInt(mailID, 10),
			})
			io.WriteString(sess, "\r\nMail deleted. Press any key.")
			_, _ = readKey(reader)
			touch()
		default:
			io.WriteString(sess, "\r\nUnknown command. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

type fileListing struct {
	Area    string
	Name    string
	Size    int64
	ModTime time.Time
}

func (s *Server) runFiles(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.admin == nil {
		io.WriteString(sess, "\r\nFile areas are unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	if touch == nil {
		touch = func() {}
	}
	if account == nil && s.auth != nil {
		if loaded, err := s.auth.GetUser(handle); err == nil && loaded != nil {
			account = loaded
		}
	}
	for {
		areas, err := s.ensureFileAreasSeeded()
		if err != nil {
			io.WriteString(sess, "\r\nCould not load file areas. Press any key.")
			_, _ = readKey(reader)
			return
		}
		areaRows := make([]string, 0, len(areas))
		for _, area := range areas {
			areaRows = append(areaRows, formatFileAreaRow(renderWidth, area.ID, area.Name, filepath.Clean(area.Path), area.Description))
		}

		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Files", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.RenderFilesMenu(renderWidth, areaRows), ansiEnabled, encoding)
		io.WriteString(sess, "Selection: ")
		choice, err := readLine(reader, 64)
		if err != nil {
			return
		}
		touch()
		switch strings.ToUpper(strings.TrimSpace(choice)) {
		case "Q":
			return
		case "?":
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderFilesHelp(renderWidth), touch)
		case "R":
			rows := formatFileRows(renderWidth, s.collectFilesAcrossAreas(areas, "", nil, 40), true, time24h)
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Recent Files", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderSearchResults(renderWidth, "Recent Files", rows), ansiEnabled, encoding)
			_, _ = readKey(reader)
			touch()
		case "N":
			since := time.Now().UTC().Add(-24 * time.Hour)
			if account != nil && account.LastLoginAt != nil && !account.LastLoginAt.IsZero() {
				since = account.LastLoginAt.UTC()
			}
			rows := formatFileRows(renderWidth, s.collectFilesAcrossAreas(areas, "", &since, 40), true, time24h)
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "New Files", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderSearchResults(renderWidth, "New Files Since Last Call", rows), ansiEnabled, encoding)
			_, _ = readKey(reader)
			touch()
		case "S":
			io.WriteString(sess, "\r\nSearch query: ")
			query, err := readLine(reader, 64)
			if err != nil {
				return
			}
			touch()
			query = strings.TrimSpace(query)
			rows := formatFileRows(renderWidth, s.collectFilesAcrossAreas(areas, query, nil, 60), true, time24h)
			writeClear(sess, ansiEnabled)
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "File Search", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
			renderFrame(sess, termWidth, renderWidth, ui.RenderSearchResults(renderWidth, "File Search: "+query, rows), ansiEnabled, encoding)
			_, _ = readKey(reader)
			touch()
		case "I":
			s.runIndexedFiles(sess, reader, termWidth, renderWidth, handle, account, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		case "D":
			s.runDownloadQueue(sess, reader, termWidth, renderWidth, handle, account, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		default:
			areaID, err := strconv.ParseInt(strings.TrimSpace(choice), 10, 64)
			if err != nil {
				io.WriteString(sess, "\r\nUse area ID or command key. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			var selected *domain.FileArea
			for _, area := range areas {
				if area.ID == areaID {
					copy := area
					selected = &copy
					break
				}
			}
			if selected == nil {
				io.WriteString(sess, "\r\nFile area not found. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			s.runFileArea(sess, reader, termWidth, renderWidth, handle, *selected, th, ansiEnabled, encoding, time24h, nodeLabel, touch)
		}
	}
}

func (s *Server) runFileArea(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, area domain.FileArea, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if touch == nil {
		touch = func() {}
	}
	query := ""
	for {
		rows, err := s.readAreaFiles(area, query, nil, 200)
		lines := []string{
			"Path: " + clampForTTY(filepath.Clean(area.Path), max(20, renderWidth-12)),
			"Query: " + clampForTTY(query, max(16, renderWidth-20)),
			"",
		}
		if err != nil {
			lines = append(lines, "Area read failed: "+clampForTTY(err.Error(), 48))
		} else {
			lines = append(lines, areaFileHeader(renderWidth))
			lines = append(lines, areaFileDivider(renderWidth))
			lines = append(lines, formatFileRows(renderWidth, rows, false, time24h)...)
		}
		lines = append(lines, "")
		lines = append(lines, "(S)earch  (R)efresh  (Q)uit area  (?)Help")

		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Files: "+area.Name, handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, area.Name, lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Selection: ")
		choice, err := readLine(reader, 48)
		if err != nil {
			return
		}
		touch()
		switch strings.ToUpper(strings.TrimSpace(choice)) {
		case "Q":
			return
		case "R":
		case "?":
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", handle, nodeLabel, th, time24h, ansiEnabled, encoding, ui.RenderFilesHelp(renderWidth), touch)
		case "S":
			io.WriteString(sess, "\r\nSearch query (blank clears): ")
			next, err := readLine(reader, 64)
			if err != nil {
				return
			}
			touch()
			query = strings.TrimSpace(next)
		default:
			io.WriteString(sess, "\r\nUnknown key. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func (s *Server) ensureFileAreasSeeded() ([]domain.FileArea, error) {
	areas, err := s.admin.ListFileAreas()
	if err != nil {
		return nil, err
	}
	if len(areas) > 0 {
		return areas, nil
	}
	defaultPath := strings.TrimSpace(os.Getenv("WOLFBBS_FILES_ROOT"))
	if defaultPath == "" {
		defaultPath = filepath.Join(".wolfbbs", "files")
	}
	defaultPath = filepath.Clean(defaultPath)
	_ = os.MkdirAll(defaultPath, 0o755)
	_ = s.admin.CreateFileArea(&domain.FileArea{
		Name:        "Uploads",
		Path:        defaultPath,
		Description: "Default local file area",
	})
	return s.admin.ListFileAreas()
}

func (s *Server) collectFilesAcrossAreas(areas []domain.FileArea, query string, since *time.Time, limit int) []fileListing {
	all := make([]fileListing, 0, 64)
	for _, area := range areas {
		rows, err := s.readAreaFiles(area, query, since, 0)
		if err != nil {
			continue
		}
		all = append(all, rows...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].ModTime.Equal(all[j].ModTime) {
			if all[i].Area == all[j].Area {
				return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name)
			}
			return strings.ToLower(all[i].Area) < strings.ToLower(all[j].Area)
		}
		return all[i].ModTime.After(all[j].ModTime)
	})
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

func (s *Server) readAreaFiles(area domain.FileArea, query string, since *time.Time, limit int) ([]fileListing, error) {
	root := filepath.Clean(strings.TrimSpace(area.Path))
	if root == "" || root == "." {
		return nil, errors.New("invalid area path")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	out := make([]fileListing, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		mod := info.ModTime().UTC()
		if since != nil && !mod.After(since.UTC()) {
			continue
		}
		out = append(out, fileListing{
			Area:    area.Name,
			Name:    name,
			Size:    info.Size(),
			ModTime: mod,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ModTime.Equal(out[j].ModTime) {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].ModTime.After(out[j].ModTime)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Server) runIndexedFiles(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.admin == nil {
		io.WriteString(sess, "\r\nIndexed filebase is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	if account == nil || account.ID <= 0 {
		io.WriteString(sess, "\r\nSign in required for indexed filebase access. Press any key.")
		_, _ = readKey(reader)
		return
	}
	acsStrict := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_STRICT")))
	if !evaluateAccess(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_FILES_READ")), account, handle, map[string]string{"area": "files", "mode": "indexed"}, acsStrict, s.logger) {
		io.WriteString(sess, "\r\nIndexed filebase denied by ACS rule. Press any key.")
		_, _ = readKey(reader)
		if touch != nil {
			touch()
		}
		return
	}
	if touch == nil {
		touch = func() {}
	}
	query := ""
	tagsRaw := ""
	for {
		tags := splitCSV(tagsRaw)
		rows, err := s.admin.ListFileEntries(0, query, tags, 120)
		areaNames := map[int64]string{}
		areas, _ := s.admin.ListFileAreas()
		for _, area := range areas {
			areaNames[area.ID] = area.Name
		}
		lines := []string{
			"Query: " + clampForTTY(query, max(16, renderWidth/2-8)) + "  Tags: " + clampForTTY(tagsRaw, max(10, renderWidth/3-4)),
			"",
			indexedFilesHeader(renderWidth),
			indexedFilesDivider(renderWidth),
		}
		if err != nil {
			lines = append(lines, "Search failed: "+clampForTTY(err.Error(), 54))
		} else if len(rows) == 0 {
			lines = append(lines, "No indexed files matched.")
		} else {
			for _, row := range rows {
				lines = append(lines, formatIndexedFileRow(renderWidth, row.ID, areaNames[row.AreaID], row.Name, row.RatingAvg, strings.Join(row.Tags, ",")))
			}
		}
		lines = append(lines, "", "Commands: (S)earch  [ID] queue add  (Q)uit")
		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Indexed FileBase", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "FileBase", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Selection: ")
		choice, err := readLine(reader, 48)
		if err != nil {
			return
		}
		touch()
		choice = strings.TrimSpace(choice)
		switch strings.ToUpper(choice) {
		case "Q":
			return
		case "S":
			io.WriteString(sess, "\r\nQuery (blank=all): ")
			nextQuery, err := readLine(reader, 72)
			if err != nil {
				return
			}
			touch()
			io.WriteString(sess, "Tags csv (blank=none): ")
			nextTags, err := readLine(reader, 72)
			if err != nil {
				return
			}
			touch()
			query = strings.TrimSpace(nextQuery)
			tagsRaw = strings.TrimSpace(nextTags)
		default:
			fileID, parseErr := strconv.ParseInt(choice, 10, 64)
			if parseErr != nil || fileID <= 0 {
				io.WriteString(sess, "\r\nUse file ID, S, or Q. Press any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			if err := s.admin.EnqueueDownload(account.ID, fileID); err != nil {
				io.WriteString(sess, "\r\nQueue add failed: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			io.WriteString(sess, "\r\nQueued file ID "+strconv.FormatInt(fileID, 10)+". Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func (s *Server) runDownloadQueue(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, th ui.Theme, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) {
	if s.admin == nil {
		io.WriteString(sess, "\r\nDownload queue is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	if account == nil || account.ID <= 0 {
		io.WriteString(sess, "\r\nSign in required for download queue. Press any key.")
		_, _ = readKey(reader)
		return
	}
	acsStrict := envBool(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_STRICT")))
	if !evaluateAccess(strings.TrimSpace(os.Getenv("WOLFBBS_ACS_FILES_READ")), account, handle, map[string]string{"area": "files", "mode": "queue"}, acsStrict, s.logger) {
		io.WriteString(sess, "\r\nDownload queue denied by ACS rule. Press any key.")
		_, _ = readKey(reader)
		if touch != nil {
			touch()
		}
		return
	}
	if touch == nil {
		touch = func() {}
	}
	for {
		queue, err := s.admin.ListDownloadQueue(account.ID, 200)
		lines := []string{
			"Queue items are shared with web FileBase queue.",
			"",
			queueHeader(renderWidth),
			queueDivider(renderWidth),
		}
		if err != nil {
			lines = append(lines, "Queue read failed: "+clampForTTY(err.Error(), 48))
		} else if len(queue) == 0 {
			lines = append(lines, "Queue is empty.")
		} else {
			for _, row := range queue {
				name := "file #" + strconv.FormatInt(row.FileID, 10)
				if entry, eErr := s.admin.GetFileEntry(row.FileID); eErr == nil && entry != nil {
					name = entry.Name
				}
				lines = append(lines, formatQueueRow(renderWidth, row.FileID, name, formatClock(row.CreatedAt.Local(), time24h)))
			}
		}
		lines = append(lines, "", "Commands: R<ID> remove  T<ID> ticket  B batch tip  Q quit")

		writeClear(sess, ansiEnabled)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, "Download Queue", handle, time.Now(), nodeLabel, th, time24h)+"\r\n", ansiEnabled, encoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "Queue", lines, ui.CP437Box, ui.FgCyan, ui.BgBlack), ansiEnabled, encoding)
		io.WriteString(sess, "Selection: ")
		choice, err := readLine(reader, 40)
		if err != nil {
			return
		}
		touch()
		choice = strings.TrimSpace(choice)
		if strings.EqualFold(choice, "q") {
			return
		}
		if strings.EqualFold(choice, "b") {
			io.WriteString(sess, "\r\nBatch ZIP: sign in to web and open /gateway?view=files&batch=1\r\nPress any key.")
			_, _ = readKey(reader)
			touch()
			continue
		}
		if len(choice) < 2 {
			io.WriteString(sess, "\r\nUse R<ID>, T<ID>, B, or Q. Press any key.")
			_, _ = readKey(reader)
			touch()
			continue
		}
		mode := strings.ToUpper(choice[:1])
		fileID, parseErr := strconv.ParseInt(strings.TrimSpace(choice[1:]), 10, 64)
		if parseErr != nil || fileID <= 0 {
			io.WriteString(sess, "\r\nInvalid file ID. Press any key.")
			_, _ = readKey(reader)
			touch()
			continue
		}
		switch mode {
		case "R":
			if err := s.admin.DequeueDownload(account.ID, fileID); err != nil {
				io.WriteString(sess, "\r\nQueue remove failed: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			io.WriteString(sess, "\r\nRemoved file ID "+strconv.FormatInt(fileID, 10)+" from queue. Press any key.")
			_, _ = readKey(reader)
			touch()
		case "T":
			ticket, err := s.issueDownloadTicket(account.ID, fileID, 15*time.Minute)
			if err != nil {
				io.WriteString(sess, "\r\nTicket creation failed: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				touch()
				continue
			}
			io.WriteString(sess, "\r\nTicket: "+ticket.Token+"\r\nURL: /gateway?download="+ticket.Token+"\r\nPress any key.")
			_, _ = readKey(reader)
			touch()
		default:
			io.WriteString(sess, "\r\nUse R<ID>, T<ID>, B, or Q. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func (s *Server) issueDownloadTicket(userID, fileID int64, ttl time.Duration) (*domain.DownloadTicket, error) {
	if s.admin == nil {
		return nil, errors.New("admin repository unavailable")
	}
	if userID <= 0 || fileID <= 0 {
		return nil, errors.New("user id and file id are required")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	token, err := randomTokenHex(24)
	if err != nil {
		return nil, err
	}
	ticket := &domain.DownloadTicket{
		Token:     "dl_" + token,
		UserID:    userID,
		FileID:    fileID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}
	if err := s.admin.CreateDownloadTicket(ticket); err != nil {
		return nil, err
	}
	return ticket, nil
}

func randomTokenHex(bytesLen int) (string, error) {
	if bytesLen <= 0 {
		bytesLen = 16
	}
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func formatCallerTTYRow(width int, nodeID int, username, loginAt, origin, fromHost, area, tail string) string {
	switch {
	case width >= 74:
		return fmt.Sprintf("%02d  %-12s %-16s %-5s %-15s %-12s %s",
			nodeID,
			clampForTTY(username, 12),
			clampForTTY(loginAt, 16),
			clampForTTY(origin, 5),
			clampForTTY(fromHost, 15),
			clampForTTY(area, 12),
			tail,
		)
	case width >= 58:
		return fmt.Sprintf("%02d  %-12s %-5s %-15s %-12s %s",
			nodeID,
			clampForTTY(username, 12),
			clampForTTY(origin, 5),
			clampForTTY(fromHost, 15),
			clampForTTY(area, 12),
			tail,
		)
	default:
		return fmt.Sprintf("%02d  %-12s %-5s %-12s %s",
			nodeID,
			clampForTTY(username, 12),
			clampForTTY(origin, 5),
			clampForTTY(area, 12),
			tail,
		)
	}
}

func formatBoardListRow(width int, boardID int64, boardName, conference string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("[%d] %-24s (%s)", boardID, clampForTTY(boardName, 24), clampForTTY(conference, 16))
	case width >= 54:
		return fmt.Sprintf("[%d] %-22s %s", boardID, clampForTTY(boardName, 22), clampForTTY(conference, 12))
	default:
		return fmt.Sprintf("[%d] %s", boardID, clampForTTY(boardName, max(18, width-10)))
	}
}

func formatMessageIndexRow(width int, msgID int64, unread, threadMarker, subject, stamp string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("%4d  %s%s %-29s %s", msgID, unread, threadMarker, clampForTTY(subject, 29), stamp)
	case width >= 56:
		return fmt.Sprintf("%4d  %s%s %-22s %s", msgID, unread, threadMarker, clampForTTY(subject, 22), stamp)
	default:
		return fmt.Sprintf("%4d  %s%s %s", msgID, unread, threadMarker, clampForTTY(subject, max(18, width-11)))
	}
}

func formatSearchResultRow(width int, msgID int64, subject, stamp string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("%4d  %-32s  %s", msgID, clampForTTY(subject, 32), stamp)
	case width >= 56:
		return fmt.Sprintf("%4d  %-24s  %s", msgID, clampForTTY(subject, 24), stamp)
	default:
		return fmt.Sprintf("%4d  %s", msgID, clampForTTY(subject, max(18, width-8)))
	}
}

func formatMailInboxRow(width int, messageID int64, subject, stamp, status string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("  %4d  %-26s  %s  %s", messageID, clampForTTY(subject, 26), stamp, status)
	case width >= 56:
		return fmt.Sprintf("  %4d  %-20s  %s  %s", messageID, clampForTTY(subject, 20), stamp, status)
	default:
		return fmt.Sprintf("  %4d  %-18s  %s", messageID, clampForTTY(subject, 18), status)
	}
}

func formatMailOutboxRow(width int, messageID int64, subject, target string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("  %4d  %-22s  %-18s", messageID, clampForTTY(subject, 22), clampForTTY(target, 18))
	case width >= 56:
		return fmt.Sprintf("  %4d  %-18s  %-14s", messageID, clampForTTY(subject, 18), clampForTTY(target, 14))
	default:
		return fmt.Sprintf("  %4d  %-16s", messageID, clampForTTY(subject, 16))
	}
}

func formatFileAreaRow(width int, areaID int64, name, path, description string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("%3d %-18s %-28s %s",
			areaID,
			clampForTTY(name, 18),
			clampForTTY(path, 28),
			clampForTTY(description, 18),
		)
	case width >= 54:
		return fmt.Sprintf("%3d %-18s %-20s %s",
			areaID,
			clampForTTY(name, 18),
			clampForTTY(path, 20),
			clampForTTY(description, 12),
		)
	default:
		return fmt.Sprintf("%3d %-18s %s",
			areaID,
			clampForTTY(name, 18),
			clampForTTY(path, max(14, width-24)),
		)
	}
}

func fileRowsHeader(width int, includeArea bool) string {
	if includeArea {
		if width >= 72 {
			return "Area       Name                               Bytes      Updated"
		}
		if width >= 56 {
			return "Area       Name                       Bytes      Updated"
		}
		return "Area       Name                 Updated"
	}
	if width >= 56 {
		return "Name                               Bytes      Updated"
	}
	return "Name                         Updated"
}

func fileRowsDivider(width int, includeArea bool) string {
	switch {
	case includeArea && width >= 72:
		return strings.Repeat("-", 70)
	case includeArea && width >= 56:
		return strings.Repeat("-", 58)
	case includeArea:
		return strings.Repeat("-", 40)
	case width >= 56:
		return strings.Repeat("-", 58)
	default:
		return strings.Repeat("-", 36)
	}
}

func formatFileRow(width int, includeArea bool, area, name string, size int64, stamp string) string {
	if includeArea {
		switch {
		case width >= 72:
			return fmt.Sprintf("%-10s %-34s %9d %s",
				clampForTTY(strings.ToUpper(area), 10),
				clampForTTY(name, 34),
				size,
				stamp,
			)
		case width >= 56:
			return fmt.Sprintf("%-10s %-24s %9d %s",
				clampForTTY(strings.ToUpper(area), 10),
				clampForTTY(name, 24),
				size,
				stamp,
			)
		default:
			return fmt.Sprintf("%-10s %-18s %s",
				clampForTTY(strings.ToUpper(area), 10),
				clampForTTY(name, 18),
				stamp,
			)
		}
	}
	if width >= 56 {
		return fmt.Sprintf("%-34s %9d %s",
			clampForTTY(name, 34),
			size,
			stamp,
		)
	}
	return fmt.Sprintf("%-22s %s",
		clampForTTY(name, 22),
		stamp,
	)
}

func areaFileHeader(width int) string {
	if width >= 56 {
		return "Name                               Bytes      Updated"
	}
	return "Name                         Updated"
}

func areaFileDivider(width int) string {
	if width >= 56 {
		return strings.Repeat("-", 58)
	}
	return strings.Repeat("-", 36)
}

func indexedFilesHeader(width int) string {
	if width >= 72 {
		return " ID   Area       Name                          Rating   Tags"
	}
	if width >= 58 {
		return " ID   Area       Name                   Rating   Tags"
	}
	return " ID   Name                   Rating"
}

func indexedFilesDivider(width int) string {
	switch {
	case width >= 72:
		return strings.Repeat("-", 70)
	case width >= 58:
		return strings.Repeat("-", 58)
	default:
		return strings.Repeat("-", 36)
	}
}

func formatIndexedFileRow(width int, fileID int64, area, name string, rating float64, tags string) string {
	switch {
	case width >= 72:
		return fmt.Sprintf("%4d %-10s %-28s %6.2f  %s",
			fileID,
			clampForTTY(strings.ToUpper(area), 10),
			clampForTTY(name, 28),
			rating,
			clampForTTY(tags, 18),
		)
	case width >= 58:
		return fmt.Sprintf("%4d %-10s %-20s %6.2f  %s",
			fileID,
			clampForTTY(strings.ToUpper(area), 10),
			clampForTTY(name, 20),
			rating,
			clampForTTY(tags, 10),
		)
	default:
		return fmt.Sprintf("%4d %-20s %6.2f",
			fileID,
			clampForTTY(name, 20),
			rating,
		)
	}
}

func queueHeader(width int) string {
	if width >= 58 {
		return " ID   File Name                        Queued"
	}
	return " ID   File Name              Queued"
}

func queueDivider(width int) string {
	if width >= 58 {
		return strings.Repeat("-", 68)
	}
	return strings.Repeat("-", 40)
}

func formatQueueRow(width int, fileID int64, name, stamp string) string {
	if width >= 58 {
		return fmt.Sprintf("%4d %-32s %s",
			fileID,
			clampForTTY(name, 32),
			stamp,
		)
	}
	return fmt.Sprintf("%4d %-20s %s",
		fileID,
		clampForTTY(name, 20),
		stamp,
	)
}

func formatFileRows(width int, rows []fileListing, includeArea bool, time24h bool) []string {
	if len(rows) == 0 {
		return []string{"No files matched."}
	}
	out := make([]string, 0, len(rows)+2)
	if includeArea {
		out = append(out, fileRowsHeader(width, true))
		out = append(out, fileRowsDivider(width, true))
		for _, row := range rows {
			out = append(out, formatFileRow(width, true, row.Area, row.Name, row.Size, formatClock(row.ModTime.Local(), time24h)))
		}
		return out
	}
	for _, row := range rows {
		out = append(out, formatFileRow(width, false, "", row.Name, row.Size, formatClock(row.ModTime.Local(), time24h)))
	}
	return out
}

func (s *Server) runSettingsMCI(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, account *domain.User, ansiEnabled bool, encoding string, time24h bool, nodeLabel string, touch func()) (*domain.User, error) {
	if touch == nil {
		touch = func() {}
	}
	if s.auth == nil {
		return account, errors.New("auth service unavailable")
	}
	user := account
	if latest, err := s.auth.GetUser(handle); err == nil && latest != nil {
		user = latest
	}
	if user == nil {
		user = &domain.User{
			Handle:        handle,
			Theme:         ui.ThemeNames()[0],
			ANSIEnabled:   ansiEnabled,
			PagingEnabled: true,
			TimeFormat24h: time24h,
		}
	}
	if strings.TrimSpace(user.Handle) == "" {
		user.Handle = handle
	}

	themes := ui.ThemeNames()
	selectedTheme := themeIndex(themes, user.Theme)

	for {
		if selectedTheme < 0 || selectedTheme >= len(themes) {
			selectedTheme = 0
		}
		user.Theme = themes[selectedTheme]
		view, err := mci.Normalize(s.buildSettingsMCIView(user, themes, selectedTheme))
		if err != nil {
			return user, err
		}
		lines := mci.RenderLines(view)
		lines = append(lines, "")
		lines = append(lines, "Selection:")
		currentANSI := user.ANSIEnabled
		currentEncoding := encoding
		if !currentANSI {
			currentEncoding = string(term.EncodingASCII)
		}
		previewTheme := ui.ThemeByName(user.Theme)
		writeClear(sess, currentANSI)
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBarWithClock(renderWidth, s.siteName()+" Settings", user.Handle, time.Now(), nodeLabel, previewTheme, user.TimeFormat24h)+"\r\n", currentANSI, currentEncoding)
		renderFrame(sess, termWidth, renderWidth, ui.DrawBox(renderWidth, len(lines)+2, "MCI Preferences", lines, ui.CP437Box, previewTheme.BodyFg, ui.BgBlack), currentANSI, currentEncoding)
		io.WriteString(sess, "Selection: ")
		key, err := readKey(reader)
		if err != nil {
			return user, err
		}
		touch()
		switch key {
		case "Q", "ESC":
			return user, nil
		case "T":
			selectedTheme = (selectedTheme + 1) % len(themes)
		case "A":
			user.ANSIEnabled = !user.ANSIEnabled
		case "P":
			user.PagingEnabled = !user.PagingEnabled
		case "C":
			user.TimeFormat24h = !user.TimeFormat24h
		case "S":
			if err := s.auth.SetPreferences(user.Handle, themes[selectedTheme], user.ANSIEnabled, user.PagingEnabled, user.TimeFormat24h); err != nil {
				return user, err
			}
			if latest, err := s.auth.GetUser(user.Handle); err == nil && latest != nil {
				user = latest
				selectedTheme = themeIndex(themes, user.Theme)
			}
			io.WriteString(sess, "\r\nPreferences saved. Press any key.")
			_, _ = readKey(reader)
			touch()
			return user, nil
		case "?":
			showHelpPanel(sess, reader, termWidth, renderWidth, s.siteName()+" Help", user.Handle, nodeLabel, previewTheme, user.TimeFormat24h, currentANSI, currentEncoding, ui.RenderSettingsHelp(renderWidth), touch)
		default:
			io.WriteString(sess, "\r\nUnknown key. Press any key.")
			_, _ = readKey(reader)
			touch()
		}
	}
}

func (s *Server) buildSettingsMCIView(user *domain.User, themes []string, selectedTheme int) mci.View {
	base := mci.View{
		ID:     "settings",
		Title:  "Settings",
		Footer: "T theme, A ansi, P paging, C clock, S save, Q quit",
		Controls: []mci.Control{
			{Type: mci.ControlLabel, ID: "header", Label: s.siteName() + " user preferences"},
			{Type: mci.ControlInput, ID: "theme", Label: "Theme", Value: user.Theme},
			{Type: mci.ControlToggle, ID: "ansi", Label: "ANSI enabled", Value: boolText(user.ANSIEnabled)},
			{Type: mci.ControlToggle, ID: "paging", Label: "Paging enabled", Value: boolText(user.PagingEnabled)},
			{Type: mci.ControlToggle, ID: "clock", Label: "24-hour clock", Value: boolText(user.TimeFormat24h)},
			{Type: mci.ControlLightbar, ID: "theme_list", Label: "Themes", Options: themes, Selected: selectedTheme},
			{Type: mci.ControlButton, ID: "save", Label: "Save Preferences"},
		},
	}
	templatePath := strings.TrimSpace(os.Getenv("WOLFBBS_MCI_SETTINGS_FILE"))
	if templatePath == "" {
		return base
	}
	view, err := mci.LoadViewFile(templatePath)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("mci settings template load failed", "path", templatePath, "error", err)
		}
		return base
	}
	if strings.TrimSpace(view.Footer) == "" {
		view.Footer = base.Footer
	}
	if strings.TrimSpace(view.Title) == "" {
		view.Title = base.Title
	}
	setControl := func(control mci.Control) {
		id := strings.ToLower(strings.TrimSpace(control.ID))
		for i := range view.Controls {
			if strings.ToLower(strings.TrimSpace(view.Controls[i].ID)) != id {
				continue
			}
			if strings.TrimSpace(view.Controls[i].Label) != "" {
				control.Label = view.Controls[i].Label
			}
			view.Controls[i] = control
			return
		}
		view.Controls = append(view.Controls, control)
	}
	setControl(mci.Control{Type: mci.ControlInput, ID: "theme", Label: "Theme", Value: user.Theme})
	setControl(mci.Control{Type: mci.ControlToggle, ID: "ansi", Label: "ANSI enabled", Value: boolText(user.ANSIEnabled)})
	setControl(mci.Control{Type: mci.ControlToggle, ID: "paging", Label: "Paging enabled", Value: boolText(user.PagingEnabled)})
	setControl(mci.Control{Type: mci.ControlToggle, ID: "clock", Label: "24-hour clock", Value: boolText(user.TimeFormat24h)})
	setControl(mci.Control{Type: mci.ControlLightbar, ID: "theme_list", Label: "Themes", Options: themes, Selected: selectedTheme})
	setControl(mci.Control{Type: mci.ControlButton, ID: "save", Label: "Save Preferences"})
	return view
}

func readMessageBody(reader *bufio.Reader, maxLines, maxChars int) (string, error) {
	if maxLines <= 0 {
		maxLines = 80
	}
	if maxChars <= 0 {
		maxChars = 4096
	}
	var lines []string
	total := 0
	for i := 0; i < maxLines; i++ {
		line, err := readLine(reader, 512)
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "." {
			break
		}
		if trimmed == "" && len(lines) > 0 {
			break
		}
		total += len(line)
		if total > maxChars {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func quoteMessage(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, "> "+line)
	}
	return strings.Join(out, "\n")
}

func clampForTTY(value string, max int) string {
	if max <= 0 {
		return ""
	}
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func themeIndex(themes []string, current string) int {
	current = strings.TrimSpace(strings.ToLower(current))
	if len(themes) == 0 {
		return 0
	}
	for idx, theme := range themes {
		if strings.EqualFold(strings.TrimSpace(theme), current) {
			return idx
		}
	}
	return 0
}

func formatClock(ts time.Time, time24h bool) string {
	if ts.IsZero() {
		return "-"
	}
	if time24h {
		return ts.Format("01-02 15:04")
	}
	return ts.Format("01-02 03:04PM")
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d.Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func normalizeRemoteHost(remoteAddr string) string {
	host := strings.TrimSpace(netutil.RemoteHost(remoteAddr))
	if host == "" {
		return "unknown"
	}
	return host
}

func formatOriginTag(remoteAddr string) string {
	switch netutil.RemoteOrigin(remoteAddr) {
	case "loopback":
		return "LOOP"
	case "lan":
		return "LAN"
	case "wan":
		return "WAN"
	case "host":
		return "HOST"
	default:
		return "UNK"
	}
}

func recordAudit(admin repository.AdminRepository, actor, target, action, details string) {
	if admin == nil {
		return
	}
	_ = admin.AddAudit(&domain.AdminAudit{
		Actor:   strings.TrimSpace(actor),
		Target:  strings.TrimSpace(target),
		Action:  strings.TrimSpace(action),
		Details: strings.TrimSpace(details),
	})
}

func translateRepoError(err error, fallback string) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, repository.ErrNotFound) {
		return "not found"
	}
	return fallback + ": " + err.Error()
}

func boardConference(board domain.Board) string {
	value := strings.TrimSpace(board.Conference)
	if value == "" {
		return "General"
	}
	return value
}

func boardReadRule(board domain.Board) string {
	if rule := strings.TrimSpace(board.ReadACS); rule != "" {
		return rule
	}
	return strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_READ"))
}

func boardWriteRule(board domain.Board) string {
	if rule := strings.TrimSpace(board.WriteACS); rule != "" {
		return rule
	}
	return strings.TrimSpace(os.Getenv("WOLFBBS_ACS_BOARDS_POST"))
}
