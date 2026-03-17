package loginserver

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/netutil"
	"wolfbbs/internal/session"
)

type textPeer interface {
	ReadLine() (string, error)
	WriteLine(string) error
	RemoteAddr() string
	Close() error
}

func runLoginSession(transport string, peer textPeer, authSvc *auth.Service, nodes *session.Manager, logger *slog.Logger, services sessionServices) {
	defer peer.Close()
	if authSvc == nil {
		_ = peer.WriteLine("Service unavailable.")
		return
	}
	if nodes == nil {
		nodes = session.NewManager(255, 256)
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	sessionID := fmt.Sprintf("%s-%d", strings.ToLower(strings.TrimSpace(transport)), time.Now().UnixNano())
	remoteAddr := peer.RemoteAddr()
	remoteHost := normalizeRemoteHost(remoteAddr)
	remoteOrigin := netutil.RemoteOrigin(remoteAddr)
	logger.Info("login transport session connected", "transport", transport, "session_id", sessionID, "remote_addr", remoteAddr, "remote_host", remoteHost, "remote_origin", remoteOrigin)
	node, err := nodes.Start(sessionID, "Guest", remoteAddr)
	if err != nil {
		_ = peer.WriteLine("No free nodes. Try again later.")
		return
	}
	defer func() {
		nodes.End(sessionID)
		logger.Info("login transport session disconnected", "transport", transport, "session_id", sessionID, "remote_host", remoteHost, "remote_origin", remoteOrigin)
	}()

	_ = peer.WriteLine("WolfBBS " + strings.ToUpper(transport) + " Access")
	_ = peer.WriteLine(fmt.Sprintf("Connected on Node #%d", node.NodeID))
	_ = peer.WriteLine("Command-mode transport with feature modules for boards/mail/chat/gateway/settings.")
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "telnet":
		_ = peer.WriteLine("Warning: Telnet is plaintext. Use only on trusted networks or inside a secure tunnel.")
	case "websocket":
		_ = peer.WriteLine("Warning: Prefer WSS for untrusted networks.")
	}
	_ = peer.WriteLine("Type HELP for commands.")

	user := loginPrompt(peer, authSvc)
	if user == nil {
		_ = peer.WriteLine("Disconnected.")
		return
	}

	nodes.SetUser(sessionID, user.Handle)
	nodes.SetArea(sessionID, "Main Menu")
	_ = peer.WriteLine("Login successful. Welcome, " + user.Handle + ".")
	_ = peer.WriteLine("Enter selection:")

	for {
		nodes.Touch(sessionID)
		cmd, err := peer.ReadLine()
		if err != nil {
			return
		}
		cmd = strings.ToUpper(strings.TrimSpace(cmd))
		if cmd == "" {
			continue
		}
		switch cmd {
		case "Q", "QUIT", "/QUIT", "EXIT":
			_ = peer.WriteLine("Goodbye.")
			return
		case "W", "WHO":
			nodes.SetArea(sessionID, "Who Online")
			writeWhoOnline(peer, nodes.Online())
			nodes.SetArea(sessionID, "Main Menu")
		case "L", "LAST":
			nodes.SetArea(sessionID, "Last Callers")
			writeLastCallers(peer, nodes.LastCallers(10))
			nodes.SetArea(sessionID, "Main Menu")
		case "B", "BOARDS":
			nodes.SetArea(sessionID, "Boards")
			runBoardsCommandMode(peer, authSvc, user, services)
			nodes.SetArea(sessionID, "Main Menu")
		case "M", "MAIL":
			nodes.SetArea(sessionID, "Mail")
			runMailCommandMode(peer, authSvc, user, services)
			nodes.SetArea(sessionID, "Main Menu")
		case "C", "CHAT":
			nodes.SetArea(sessionID, "Chat")
			runChatCommandMode(peer, user.Handle, services)
			nodes.SetArea(sessionID, "Main Menu")
		case "G", "GATEWAY":
			nodes.SetArea(sessionID, "Gateway")
			runGatewayCommandMode(peer, user.Handle, services)
			nodes.SetArea(sessionID, "Main Menu")
		case "S", "SETTINGS":
			nodes.SetArea(sessionID, "Settings")
			runSettingsCommandMode(peer, authSvc, user.Handle)
			nodes.SetArea(sessionID, "Main Menu")
		case "?", "H", "HELP":
			_ = peer.WriteLine("Commands: (W)ho (L)ast (B)oards (M)ail (C)hat (G)ateway (S)ettings (Q)uit")
			_ = peer.WriteLine("Boards: R <boardID> [msgID], P <boardID>")
			_ = peer.WriteLine("Mail: C compose, R/P/D <id>")
			_ = peer.WriteLine("Chat: S <msg>, J <#channel>, O online")
			_ = peer.WriteLine("Gateway: W <url>, O <url>, E <email>")
		default:
			_ = peer.WriteLine("Unknown selection. Enter (H)elp for commands.")
		}
		_ = peer.WriteLine("Enter selection:")
	}
}

func loginPrompt(peer textPeer, authSvc *auth.Service) *authUser {
	var selected *authUser
	for attempts := 0; attempts < 3; attempts++ {
		handle, err := prompt(peer, "Handle:")
		if err != nil {
			return nil
		}
		if isQuit(handle) {
			return nil
		}
		password, err := prompt(peer, "Password:")
		if err != nil {
			return nil
		}
		if isQuit(password) {
			return nil
		}

		code, err := prompt(peer, "2FA code (blank if not enabled):")
		if err != nil {
			return nil
		}
		user, authErr := authSvc.Authenticate(handle, password, code)
		if errors.Is(authErr, auth.ErrMissingSecondFactor) {
			code, err = prompt(peer, "2FA required. Code:")
			if err != nil {
				return nil
			}
			user, authErr = authSvc.Authenticate(handle, password, code)
		}
		if authErr != nil {
			_ = peer.WriteLine("Login failed.")
			continue
		}
		selected = &authUser{Handle: user.Handle, ID: user.ID}
		break
	}
	if selected == nil {
		_ = peer.WriteLine("Too many failed login attempts.")
		return nil
	}
	return selected
}

type authUser struct {
	Handle string
	ID     int64
}

func prompt(peer textPeer, label string) (string, error) {
	if err := peer.WriteLine(label); err != nil {
		return "", err
	}
	value, err := peer.ReadLine()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func writeWhoOnline(peer textPeer, rows []session.NodeState) {
	_ = peer.WriteLine("Who's Online")
	if len(rows) == 0 {
		_ = peer.WriteLine("No active nodes.")
		return
	}
	for _, row := range rows {
		host := normalizeRemoteHost(row.RemoteAddr)
		origin := strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr))
		line := fmt.Sprintf("Node %d | %s | Area: %s | Idle: %ds | From: %s (%s)", row.NodeID, fallback(row.Username, "Guest"), fallback(row.Area, "Main"), row.IdleSeconds, host, origin)
		_ = peer.WriteLine(line)
	}
}

func writeLastCallers(peer textPeer, rows []session.CallerState) {
	_ = peer.WriteLine("Last Callers")
	if len(rows) == 0 {
		_ = peer.WriteLine("No caller history.")
		return
	}
	for _, row := range rows {
		host := normalizeRemoteHost(row.RemoteAddr)
		origin := strings.ToUpper(netutil.RemoteOrigin(row.RemoteAddr))
		line := fmt.Sprintf("Node %d | %s | Area: %s | Duration: %s | From: %s (%s)", row.NodeID, fallback(row.Username, "Guest"), fallback(row.Area, "Main"), row.Duration.Round(time.Second), host, origin)
		_ = peer.WriteLine(line)
	}
}

func fallback(value, def string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return def
	}
	return value
}

func isQuit(value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	return value == "Q" || value == "QUIT" || value == "/QUIT" || value == "EXIT"
}

func normalizeRemoteHost(remoteAddr string) string {
	host := strings.TrimSpace(netutil.RemoteHost(remoteAddr))
	if host == "" {
		return "unknown"
	}
	return host
}
