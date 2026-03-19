package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/events"
	"wolfbbs/internal/logging"
	"wolfbbs/internal/rbac"
	"wolfbbs/internal/repository"
)

type ircState struct {
	nick        string
	user        string
	realName    string
	away        string
	pass        string
	saslReq     bool
	awaitSASL   bool
	authed      bool
	registered  bool
	canModerate bool
	channel     string
	hitTimes    []time.Time
	remoteHost  string
	connectedAt time.Time
	lastActive  time.Time
}

type ircClient struct {
	nick   string
	conn   net.Conn
	state  *ircState
	writer *bufio.Writer
	mu     sync.Mutex
}

const serverName = "wolfbbs"

var (
	// ircVersion is overridden in CI/release builds via -ldflags -X main.ircVersion=...
	ircVersion = "1.0.0"
	maxConn       = 512
	maxPerIP      = 16
	floodWindow   = 1500 * time.Millisecond
	floodBurst    = 8
	ipFloodWindow = 2 * time.Second
	ipFloodBurst  = 20
	requirePass   = true
	allowAlias    = false
	pollInterval  = 1200 * time.Millisecond
	bannedIPNets  = map[string]struct{}{}
	activeByIP    = map[string]int{}
	ipHits        = map[string][]time.Time{}
	activeTotal   = 0
	connectionMu  sync.Mutex
	ipFloodMu     sync.Mutex
	clientsMu     sync.Mutex
	clientsByNick = map[string]*ircClient{}
	channelPeers  = map[string]map[string]*ircClient{}
)

func main() {
	logging.ConfigureStdLogger("wolfbbs-irc")
	listen := flag.String("listen", ":6667", "IRC listen address")
	tlsListen := flag.String("tls-listen", strings.TrimSpace(os.Getenv("WOLFBBS_IRC_TLS_LISTEN")), "IRC TLS listen address")
	tlsCert := flag.String("tls-cert", strings.TrimSpace(os.Getenv("WOLFBBS_IRC_TLS_CERT")), "TLS cert file")
	tlsKey := flag.String("tls-key", strings.TrimSpace(os.Getenv("WOLFBBS_IRC_TLS_KEY")), "TLS key file")
	dbURL := flag.String("db", "", "PostgreSQL DSN (defaults to WOLFBBS_DATABASE_URL / DATABASE_URL / PG* env)")
	flag.Parse()
	if *dbURL == "" {
		*dbURL = repository.ResolveDatabaseURL()
	}
	loadIRCTuning()

	storage, err := repository.OpenStorageFromEnv(*dbURL)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}
	defer storage.Close()
	authSvc := auth.NewService(storage.Users)
	bus := events.NewBus()
	bus.Subscribe("*", func(ev events.Event) {
		log.Printf("event=%s fields=%v", ev.Name, ev.Fields)
	})
	authSvc.SetEventBus(bus)
	chatSvc := chat.NewServiceWithStorage(strings.TrimSpace(*dbURL))
	chatSvc.SetEventBus(bus)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("WolfBBS IRC on", *listen)
	go serveIRCListener(ln, chatSvc, authSvc)

	if *tlsListen != "" && *tlsCert != "" && *tlsKey != "" {
		certificate, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if err != nil {
			log.Fatalf("tls key pair: %v", err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS12,
		}
		tlsLn, err := tls.Listen("tcp", *tlsListen, tlsCfg)
		if err != nil {
			log.Fatalf("tls listen: %v", err)
		}
		fmt.Println("WolfBBS IRC TLS on", *tlsListen)
		go serveIRCListener(tlsLn, chatSvc, authSvc)
	}
	select {}
}

func serveIRCListener(ln net.Listener, chatSvc *chat.Service, authSvc *auth.Service) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		ip := normalizeHost(conn.RemoteAddr())
		if !registerConnection(ip) {
			_ = replyf(conn, ":%s 001 * :Too many connections from your address", serverName)
			_ = conn.Close()
			continue
		}
		go handleIRCConn(conn, chatSvc, authSvc, ip)
	}
}

func loadIRCTuning() {
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_MAX_CONN")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxConn = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_MAX_CONN_PER_IP")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxPerIP = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_MSG_WINDOW_MS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			floodWindow = time.Duration(parsed) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_MSG_BURST")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			floodBurst = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_IP_MSG_WINDOW_MS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			ipFloodWindow = time.Duration(parsed) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_IP_MSG_BURST")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			ipFloodBurst = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_POLL_MS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			pollInterval = time.Duration(parsed) * time.Millisecond
		}
	}
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("WOLFBBS_IRC_REQUIRE_PASS"))); v == "0" || v == "false" {
		requirePass = false
	}
	if strings.TrimSpace(strings.ToLower(os.Getenv("WOLFBBS_IRC_ALLOW_ALIAS"))) == "1" || strings.TrimSpace(strings.ToLower(os.Getenv("WOLFBBS_IRC_ALLOW_ALIAS"))) == "true" {
		allowAlias = true
	}
	rawBans := strings.TrimSpace(os.Getenv("WOLFBBS_IRC_BANNED_IPS"))
	if rawBans != "" {
		for _, ip := range strings.Split(rawBans, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			bannedIPNets[strings.ToLower(ip)] = struct{}{}
		}
	}
}

func registerConnection(remoteIP string) bool {
	connectionMu.Lock()
	defer connectionMu.Unlock()
	if _, ok := bannedIPNets[strings.ToLower(remoteIP)]; ok {
		return false
	}
	if maxConn > 0 && activeTotal >= maxConn {
		return false
	}
	if maxPerIP > 0 && activeByIP[remoteIP] >= maxPerIP {
		return false
	}
	activeTotal++
	activeByIP[remoteIP]++
	return true
}

func releaseConnection(remoteIP string) {
	connectionMu.Lock()
	defer connectionMu.Unlock()
	if activeTotal > 0 {
		activeTotal--
	}
	if activeByIP[remoteIP] > 1 {
		activeByIP[remoteIP]--
		return
	}
	delete(activeByIP, remoteIP)
}

func normalizeHost(addr net.Addr) string {
	host, _, err := net.SplitHostPort(addr.String())
	if err == nil {
		return host
	}
	return addr.String()
}

func handleIRCConn(conn net.Conn, svc *chat.Service, authSvc *auth.Service, ip string) {
	defer conn.Close()
	defer releaseConnection(ip)
	defer unregisterClientByConn(conn)

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	now := time.Now().UTC()
	state := &ircState{
		remoteHost:  ip,
		connectedAt: now,
		lastActive:  now,
	}
	client := &ircClient{
		conn:   conn,
		state:  state,
		writer: w,
	}

	_ = replyfConn(client, ":%s 001 * :Welcome to WolfBBS IRC", serverName)
	_ = replyfConn(client, ":%s 002 * :Your host is WolfBBS-IRCd", serverName)
	_ = replyfConn(client, ":%s 003 * :This server accepts BBS accounts", serverName)
	_ = replyfConn(client, ":%s 004 * WolfBBS "+ircVersion+" i", serverName)
	_ = replyfConn(client, ":%s 375 * :- WolfBBS Message of the day", serverName)
	_ = replyfConn(client, ":%s 372 * :- Authenticate with PASS, NICK, USER then join a channel", serverName)
	_ = replyfConn(client, ":%s 376 * :End of /MOTD command", serverName)

	linePoll := make(chan string, 32)
	done := make(chan struct{})

	go startChatPoller(client, svc, linePoll, done)
	defer close(done)

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		state.lastActive = time.Now().UTC()
		if !checkFloodIP(ip) {
			_ = replyfConn(client, ":%s 439 %s :Target change too fast", serverName, nickOrStar(state.nick))
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 0 {
			continue
		}
		cmd := strings.ToUpper(parts[0])
		raw := ""
		if len(parts) == 2 {
			raw = strings.TrimSpace(parts[1])
		}

		switch cmd {
		case "CAP":
			sub := strings.ToUpper(strings.TrimSpace(raw))
			switch {
			case strings.HasPrefix(sub, "LS"):
				_ = replyfConn(client, ":%s CAP %s LS :sasl", serverName, nickOrStar(state.nick))
			case strings.HasPrefix(sub, "REQ"):
				if strings.Contains(strings.ToLower(sub), "sasl") {
					state.saslReq = true
					_ = replyfConn(client, ":%s CAP %s ACK :sasl", serverName, nickOrStar(state.nick))
				} else {
					_ = replyfConn(client, ":%s CAP %s NAK :%s", serverName, nickOrStar(state.nick), strings.TrimSpace(raw))
				}
			case strings.HasPrefix(sub, "END"):
				// end capability negotiation.
			default:
				_ = replyfConn(client, ":%s CAP %s LS :sasl", serverName, nickOrStar(state.nick))
			}
			continue
		case "AUTHENTICATE":
			if !state.saslReq {
				_ = replyfConn(client, ":%s 904 %s :SASL authentication failed", serverName, nickOrStar(state.nick))
				continue
			}
			chunk := strings.TrimSpace(strings.TrimPrefix(raw, ":"))
			if strings.EqualFold(chunk, "PLAIN") {
				state.awaitSASL = true
				_ = replyfConn(client, "AUTHENTICATE +")
				continue
			}
			if chunk == "+" {
				continue
			}
			if !state.awaitSASL {
				_ = replyfConn(client, ":%s 905 %s :SASL message too long", serverName, nickOrStar(state.nick))
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(chunk)
			if err != nil {
				_ = replyfConn(client, ":%s 904 %s :SASL authentication failed", serverName, nickOrStar(state.nick))
				continue
			}
			parts := strings.Split(string(decoded), "\x00")
			if len(parts) < 3 {
				_ = replyfConn(client, ":%s 904 %s :SASL authentication failed", serverName, nickOrStar(state.nick))
				continue
			}
			authUser := strings.TrimSpace(parts[len(parts)-2])
			authPass := strings.TrimSpace(parts[len(parts)-1])
			if authUser == "" {
				authUser = state.nick
			}
			state.user = authUser
			state.pass = authPass
			state.awaitSASL = false
			tryAuthenticate(client, authSvc)
			if state.authed {
				_ = replyfConn(client, ":%s 903 %s :SASL authentication successful", serverName, nickOrStar(state.nick))
			} else {
				_ = replyfConn(client, ":%s 904 %s :SASL authentication failed", serverName, nickOrStar(state.nick))
			}
			continue
		case "PASS":
			if state.registered {
				_ = replyfConn(client, ":%s 462 %s :You may not reregister", serverName, nickOrStar(state.nick))
				continue
			}
			if raw == "" {
				_ = replyfConn(client, ":%s 461 %s :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			state.pass = strings.Trim(raw, ":")
			if state.user != "" {
				tryAuthenticate(client, authSvc)
			}
			continue
		case "NICK":
			nick := strings.TrimSpace(strings.TrimLeft(raw, ":"))
			if nick == "" {
				_ = replyfConn(client, ":%s 431 %s :No nickname given", serverName, nickOrStar(state.nick))
				continue
			}
			if len(nick) > 31 || strings.ContainsAny(nick, " \x00\r\n,:*?!@") {
				_ = replyfConn(client, ":%s 432 %s :Erroneous nickname", serverName, nickOrStar(nick))
				continue
			}
			clientsMu.Lock()
			if state.nick != "" {
				delete(clientsByNick, strings.ToLower(state.nick))
			}
			if existing := clientsByNick[strings.ToLower(nick)]; existing != nil && existing != client {
				clientsMu.Unlock()
				_ = replyfConn(client, ":%s 433 %s %s :Nickname is already in use", serverName, nickOrStar(state.nick), nick)
				continue
			}
			client.nick = nick
			state.nick = nick
			clientsByNick[strings.ToLower(nick)] = client
			clientsMu.Unlock()
			if state.user != "" {
				tryAuthenticate(client, authSvc)
			}
			continue
		case "USER":
			if state.registered {
				_ = replyfConn(client, ":%s 462 %s :You may not reregister", serverName, nickOrStar(state.nick))
				continue
			}
			if raw == "" {
				_ = replyfConn(client, ":%s 461 %s :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			userParts := strings.Fields(raw)
			if len(userParts) < 4 {
				_ = replyfConn(client, ":%s 461 %s :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			state.user = strings.TrimSpace(userParts[0])
			if state.user == "" {
				_ = replyfConn(client, ":%s 461 %s :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			if idx := strings.Index(raw, " :"); idx >= 0 {
				state.realName = strings.TrimSpace(raw[idx+2:])
			}
			if state.realName == "" && len(userParts) >= 4 {
				state.realName = strings.TrimLeft(strings.Join(userParts[3:], " "), ":")
			}
			if state.nick == "" {
				state.nick = state.user
				client.nick = state.nick
			}
			tryAuthenticate(client, authSvc)
			continue
		case "PING":
			target := strings.TrimSpace(strings.TrimPrefix(raw, ":"))
			if target == "" {
				target = serverName
			}
			_ = replyfConn(client, ":%s PONG :%s", serverName, target)
			continue
		case "PONG":
			// We do not enforce server-side state from PONG, but we accept it for compatibility.
			continue
		}

		if !state.authed {
			_ = replyfConn(client, ":%s 451 %s :You have not registered", serverName, nickOrStar(state.nick))
			continue
		}
		if !checkFlood(state) {
			_ = replyfConn(client, ":%s 432 %s :Slow down", serverName, nickOrStar(state.nick))
			continue
		}

		switch cmd {
		case "JOIN":
			channel := strings.TrimSpace(raw)
			if channel == "" {
				_ = replyfConn(client, ":%s 461 %s JOIN :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			if strings.Contains(channel, ",") {
				channel = strings.TrimSpace(strings.SplitN(channel, ",", 2)[0])
			}
			channel = chat.NormalizeChannel(strings.TrimPrefix(channel, ":"))
			if channel == "" {
				_ = replyfConn(client, ":%s 403 %s %s :No such channel", serverName, nickOrStar(state.nick), channel)
				continue
			}
			svc.JoinChannel(state.nick, channel)
			moveClient(client, channel)
			select {
			case linePoll <- channel:
			default:
			}
			_ = replyfConn(client, ":%s JOIN :%s", nickOrStar(state.nick), channel)
			members := channelMembers(channel)
			_ = replyfConn(client, ":%s 353 %s = %s :%s", serverName, nickOrStar(state.nick), channel, strings.Join(members, " "))
			_ = replyfConn(client, ":%s 366 %s %s :End of /NAMES list", serverName, nickOrStar(state.nick), channel)
			_ = replyfConn(client, ":%s 332 %s %s :WolfBBS channel topic", serverName, nickOrStar(state.nick), channel)
		case "PART":
			channel := strings.TrimSpace(raw)
			if channel == "" {
				channel = state.channel
			}
			channel = strings.TrimPrefix(channel, ":")
			channel = chat.NormalizeChannel(channel)
			if channel == "" {
				_ = replyfConn(client, ":%s 461 %s PART :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			svc.LeaveChannel(state.nick, channel)
			moveClient(client, "")
			select {
			case linePoll <- "":
			default:
			}
			_ = replyfConn(client, ":%s PART %s :left", nickOrStar(state.nick), channel)
		case "PRIVMSG", "NOTICE":
			target, message, ok := parseTargetAndBody(raw)
			if !ok {
				_ = replyfConn(client, ":%s 411 %s :No recipient given", serverName, nickOrStar(state.nick))
				continue
			}
			if message == "" {
				_ = replyfConn(client, ":%s 412 %s :No text to send", serverName, nickOrStar(state.nick))
				continue
			}
			if !strings.HasPrefix(target, "#") {
				targetClient, ok := clientByNick(target)
				if !ok {
					if cmd == "PRIVMSG" {
						_ = replyfConn(client, ":%s 401 %s %s :No such nick", serverName, nickOrStar(state.nick), target)
					}
					continue
				}
				if !sendDirectMessage(targetClient, ircMessageLine(cmd, state.nick, target, message), client) && cmd == "PRIVMSG" {
					_ = replyfConn(client, ":%s 401 %s %s :No such nick", serverName, nickOrStar(state.nick), target)
					continue
				}
				if cmd == "PRIVMSG" {
					away := strings.TrimSpace(targetClient.state.away)
					if away != "" {
						_ = replyfConn(client, ":%s 301 %s %s :%s", serverName, nickOrStar(state.nick), targetClient.state.nick, away)
					}
				}
				continue
			}
			target = chat.NormalizeChannel(target)
			if cmd == "NOTICE" {
				_ = broadcastToChannel(target, ircMessageLine(cmd, state.nick, target, message))
				continue
			}
			msg, err := svc.Post(state.nick, target, message)
			if err != nil {
				_ = replyfConn(client, ":%s 437 %s %s :%s", serverName, nickOrStar(state.nick), target, err.Error())
				continue
			}
			payload := ircMessageLine(cmd, state.nick, target, msg.Body)
			_ = broadcastToChannel(target, payload)
		case "NAMES":
			channel := strings.TrimPrefix(raw, ":")
			if channel == "" {
				channel = state.channel
			}
			channel = chat.NormalizeChannel(channel)
			if channel == "" {
				_ = replyfConn(client, ":%s 461 %s NAMES :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			members := channelMembers(channel)
			_ = replyfConn(client, ":%s 353 %s = %s :%s", serverName, nickOrStar(state.nick), channel, strings.Join(members, " "))
			_ = replyfConn(client, ":%s 366 %s %s :End of /NAMES list", serverName, nickOrStar(state.nick), channel)
		case "LIST":
			for _, channel := range svc.ListChannels() {
				_ = replyfConn(client, ":%s 322 %s %s 0 :WolfBBS channel", serverName, nickOrStar(state.nick), channel)
			}
			_ = replyfConn(client, ":%s 323 %s :End of /LIST", serverName, nickOrStar(state.nick))
		case "WHO":
			area := strings.TrimPrefix(raw, ":")
			filterNick := strings.ToLower(strings.TrimSpace(area))
			for _, p := range svc.Online() {
				name := strings.ToLower(p.Nick)
				if filterNick != "" && name != filterNick {
					continue
				}
				_ = replyfConn(client, ":%s 352 %s %s * * * * H :0 %s", serverName, nickOrStar(state.nick), chat.NormalizeChannel(state.channel), p.Nick)
			}
			if filterNick == "" {
				filterNick = "*"
			}
			_ = replyfConn(client, ":%s 315 %s %s :End of WHO list", serverName, nickOrStar(state.nick), filterNick)
		case "WHOIS":
			target := strings.TrimPrefix(raw, ":")
			if strings.Contains(target, ",") {
				target = strings.TrimSpace(strings.SplitN(target, ",", 2)[0])
			}
			if target == "" {
				_ = replyfConn(client, ":%s 431 %s :No nickname given", serverName, nickOrStar(state.nick))
				continue
			}
			if !writeWhois(client, target) {
				_ = replyfConn(client, ":%s 401 %s %s :No such nick", serverName, nickOrStar(state.nick), target)
				_ = replyfConn(client, ":%s 318 %s %s :End of WHOIS list", serverName, nickOrStar(state.nick), target)
			}
		case "AWAY":
			message := strings.TrimSpace(strings.TrimPrefix(raw, ":"))
			state.away = message
			if message == "" {
				_ = replyfConn(client, ":%s 305 %s :You are no longer marked as being away", serverName, nickOrStar(state.nick))
			} else {
				_ = replyfConn(client, ":%s 306 %s :You have been marked as being away", serverName, nickOrStar(state.nick))
			}
		case "TOPIC":
			channel := strings.TrimPrefix(raw, ":")
			if channel == "" {
				_ = replyfConn(client, ":%s 461 %s TOPIC :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			channel = chat.NormalizeChannel(channel)
			_ = replyfConn(client, ":%s 332 %s %s :WolfBBS channel topic", serverName, nickOrStar(state.nick), channel)
		case "MODE":
			channel := strings.Trim(raw, " ")
			if channel == "" {
				_ = replyfConn(client, ":%s 461 %s MODE :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			_ = replyfConn(client, ":%s 324 %s %s +nt", serverName, nickOrStar(state.nick), chat.NormalizeChannel(strings.TrimPrefix(channel, "#")))
		case "KICK":
			channel, target, ok := parseKick(raw)
			if !ok {
				_ = replyfConn(client, ":%s 461 %s KICK :Not enough parameters", serverName, nickOrStar(state.nick))
				continue
			}
			if !state.canModerate {
				_ = replyfConn(client, ":%s 482 %s %s :You're not channel operator", serverName, nickOrStar(state.nick), channel)
				continue
			}
			svc.Kick(channel, state.nick, target, "irc kick")
			_ = replyfConn(client, ":%s KICK %s %s :irc kick", nickOrStar(state.nick), channel, target)
		case "QUIT":
			reason := strings.TrimLeft(strings.TrimPrefix(raw, ":"), " ")
			if reason == "" {
				reason = "Client quit"
			}
			_ = replyfConn(client, ":%s QUIT :%s", nickOrStar(state.nick), reason)
			return
		default:
			_ = replyfConn(client, ":%s 421 %s %s :Unknown command", serverName, nickOrStar(state.nick), cmd)
		}
	}
}

func tryAuthenticate(client *ircClient, authSvc *auth.Service) {
	state := client.state
	if state.user == "" || !requirePass && state.pass == "" {
		if state.user != "" {
			// account auth with password disabled for this install mode.
			completeRegistration(client)
		}
		return
	}
	if state.pass == "" && requirePass {
		return
	}
	if !allowAlias && state.nick != "" && !strings.EqualFold(state.nick, state.user) {
		_ = replyfConn(client, ":%s 432 %s :Nick/account mismatch", serverName, nickOrStar(state.nick))
		return
	}
	userHandle := state.user
	if userHandle == "" {
		userHandle = state.nick
	}
	user, err := authSvc.Authenticate(userHandle, state.pass, "")
	if err != nil {
		_ = replyfConn(client, ":%s 464 %s :Password incorrect", serverName, nickOrStar(state.nick))
		return
	}
	state.authed = true
	if user != nil {
		state.canModerate = rbac.AtLeast(user.Role, rbac.RoleModerator)
	}
	if state.nick == "" {
		state.nick = userHandle
		client.nick = userHandle
	}
	completeRegistration(client)
}

func completeRegistration(client *ircClient) {
	state := client.state
	if state.registered {
		return
	}
	state.registered = true
	if state.nick == "" {
		state.nick = state.user
		client.nick = state.user
	}
	client.nick = state.nick
	clientsMu.Lock()
	if client.nick != "" {
		clientsByNick[strings.ToLower(client.nick)] = client
	}
	clientsMu.Unlock()
	_ = replyfConn(client, ":%s 001 %s :Welcome to WolfBBS IRC, %s", serverName, nickOrStar(state.nick), state.nick)
	_ = replyfConn(client, ":%s 002 %s :Your host is WolfBBS-IRCd", serverName, nickOrStar(state.nick))
	_ = replyfConn(client, ":%s 003 %s :This server accepts BBS accounts", serverName, nickOrStar(state.nick))
	_ = replyfConn(client, ":%s 004 %s WolfBBS "+ircVersion+" i", serverName, nickOrStar(state.nick))
	_ = replyfConn(client, ":%s 375 %s :- Welcome to the message-of-the-day file", serverName, nickOrStar(state.nick))
	_ = replyfConn(client, ":%s 372 %s :- Authenticated as %s", serverName, nickOrStar(state.nick), state.nick)
	_ = replyfConn(client, ":%s 376 %s :End of /MOTD", serverName, nickOrStar(state.nick))
}

func parseTargetAndBody(raw string) (string, string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", "", false
	}
	parts := strings.SplitN(raw, " :", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	target := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])
	if target == "" {
		return "", "", false
	}
	return target, body, true
}

func parseKick(raw string) (string, string, bool) {
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return "", "", false
	}
	channel := chat.NormalizeChannel(parts[0])
	target := strings.TrimSpace(parts[1])
	if channel == "" || target == "" {
		return "", "", false
	}
	return channel, target, true
}

func nickOrStar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "*"
	}
	return value
}

func moveClient(client *ircClient, channel string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	old := client.state.channel
	if old != "" {
		if peers, ok := channelPeers[old]; ok {
			delete(peers, strings.ToLower(client.nick))
			if len(peers) == 0 {
				delete(channelPeers, old)
			}
		}
	}
	client.state.channel = channel
	if channel == "" || client.nick == "" {
		return
	}
	if channelPeers[channel] == nil {
		channelPeers[channel] = map[string]*ircClient{}
	}
	channelPeers[channel][strings.ToLower(client.nick)] = client
}

func unregisterClientByConn(conn net.Conn) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	var target *ircClient
	for _, c := range clientsByNick {
		if c.conn == conn {
			target = c
			break
		}
	}
	if target == nil || target.nick == "" {
		return
	}
	delete(clientsByNick, strings.ToLower(target.nick))
	if old := target.state.channel; old != "" {
		if peers, ok := channelPeers[old]; ok {
			delete(peers, strings.ToLower(target.nick))
			if len(peers) == 0 {
				delete(channelPeers, old)
			}
		}
	}
}

func channelMembers(channel string) []string {
	members := map[string]string{}
	clientsMu.Lock()
	for nick, c := range channelPeers[channel] {
		if c != nil && strings.TrimSpace(c.nick) != "" && strings.TrimSpace(c.state.nick) != "" {
			display := nick
			if c.state != nil && c.state.canModerate {
				display = "@" + strings.TrimPrefix(display, "@")
			}
			members[nick] = display
		}
	}
	clientsMu.Unlock()

	out := make([]string, 0, len(members))
	for _, nick := range members {
		out = append(out, nick)
	}
	sort.Strings(out)
	return out
}

func broadcastToChannel(channel, line string) bool {
	clientsMu.Lock()
	members := make([]*ircClient, 0, len(channelPeers[channel]))
	for _, c := range channelPeers[channel] {
		members = append(members, c)
	}
	clientsMu.Unlock()
	if len(members) == 0 {
		return false
	}
	for _, c := range members {
		_ = replyfConn(c, "%s", line)
	}
	return true
}

func clientByNick(target string) (*ircClient, bool) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	targetClient := clientsByNick[strings.ToLower(strings.TrimSpace(target))]
	if targetClient == nil {
		return nil, false
	}
	return targetClient, true
}

func sendDirectMessage(targetClient *ircClient, line string, from *ircClient) bool {
	if targetClient == nil {
		return false
	}
	_ = replyfConn(targetClient, "%s", line)
	if from != targetClient {
		_ = replyfConn(from, "%s", line)
	}
	return true
}

func ircMessageLine(kind, from, target, body string) string {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind != "NOTICE" {
		kind = "PRIVMSG"
	}
	return fmt.Sprintf(":%s %s %s :%s", from, kind, target, body)
}

type whoisSnapshot struct {
	nick        string
	user        string
	realName    string
	host        string
	away        string
	channels    []string
	idleSeconds int64
	signon      int64
}

func writeWhois(client *ircClient, target string) bool {
	snapshot, ok := snapshotWhois(target)
	if !ok {
		return false
	}
	requester := nickOrStar(client.state.nick)
	realName := strings.TrimSpace(snapshot.realName)
	if realName == "" {
		realName = "WolfBBS user"
	}
	username := strings.TrimSpace(snapshot.user)
	if username == "" {
		username = snapshot.nick
	}
	host := strings.TrimSpace(snapshot.host)
	if host == "" {
		host = "localhost"
	}
	_ = replyfConn(client, ":%s 311 %s %s %s %s * :%s", serverName, requester, snapshot.nick, username, host, realName)
	if strings.TrimSpace(snapshot.away) != "" {
		_ = replyfConn(client, ":%s 301 %s %s :%s", serverName, requester, snapshot.nick, snapshot.away)
	}
	if len(snapshot.channels) > 0 {
		_ = replyfConn(client, ":%s 319 %s %s :%s", serverName, requester, snapshot.nick, strings.Join(snapshot.channels, " "))
	}
	_ = replyfConn(client, ":%s 312 %s %s %s :WolfBBS IRCd", serverName, requester, snapshot.nick, serverName)
	_ = replyfConn(client, ":%s 317 %s %s %d %d :seconds idle, signon time", serverName, requester, snapshot.nick, snapshot.idleSeconds, snapshot.signon)
	_ = replyfConn(client, ":%s 318 %s %s :End of WHOIS list", serverName, requester, snapshot.nick)
	return true
}

func snapshotWhois(target string) (whoisSnapshot, bool) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	targetClient := clientsByNick[strings.ToLower(strings.TrimSpace(target))]
	if targetClient == nil || targetClient.state == nil || strings.TrimSpace(targetClient.nick) == "" {
		return whoisSnapshot{}, false
	}

	idleSeconds := int64(time.Since(targetClient.state.lastActive).Seconds())
	if idleSeconds < 0 {
		idleSeconds = 0
	}
	snapshot := whoisSnapshot{
		nick:        strings.TrimSpace(targetClient.nick),
		user:        strings.TrimSpace(targetClient.state.user),
		realName:    strings.TrimSpace(targetClient.state.realName),
		host:        strings.TrimSpace(targetClient.state.remoteHost),
		away:        strings.TrimSpace(targetClient.state.away),
		idleSeconds: idleSeconds,
		signon:      targetClient.state.connectedAt.Unix(),
	}
	if channel := strings.TrimSpace(targetClient.state.channel); channel != "" {
		snapshot.channels = []string{channel}
	}
	return snapshot, true
}

func startChatPoller(client *ircClient, svc *chat.Service, channelUpdate <-chan string, done <-chan struct{}) {
	current := ""
	lastMessage := int64(0)
	var stream <-chan chat.Message
	var closeStream func()
	subscribe := func(channel string) {
		if closeStream != nil {
			closeStream()
			closeStream = nil
		}
		stream = nil
		if channel == "" {
			return
		}
		ch, closeFn := svc.Subscribe(channel, "")
		stream = ch
		closeStream = closeFn
	}
	subscribe(current)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	defer func() {
		if closeStream != nil {
			closeStream()
		}
	}()

	for {
		select {
		case ch := <-channelUpdate:
			current = ch
			subscribe(current)
		case msg, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			if current == "" {
				continue
			}
			if msg.Channel != current {
				continue
			}
			if msg.ID <= lastMessage {
				continue
			}
			lastMessage = msg.ID
			line := fmt.Sprintf(":%s PRIVMSG %s :%s", msg.From, msg.Channel, msg.Body)
			_ = replyfConn(client, "%s", line)
		case <-ticker.C:
			if current == "" {
				continue
			}
			messages := svc.HistorySince(current, lastMessage, 20)
			for _, msg := range messages {
				if msg.ID <= lastMessage {
					continue
				}
				lastMessage = msg.ID
				line := fmt.Sprintf(":%s PRIVMSG %s :%s", msg.From, msg.Channel, msg.Body)
				_ = replyfConn(client, "%s", line)
			}
		case <-done:
			return
		}
	}
}

func checkFlood(state *ircState) bool {
	now := time.Now()
	cutoff := now.Add(-floodWindow)
	kept := state.hitTimes[:0]
	for _, t := range state.hitTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= floodBurst {
		state.hitTimes = kept
		return false
	}
	kept = append(kept, now)
	state.hitTimes = kept
	return true
}

func checkFloodIP(ip string) bool {
	ipFloodMu.Lock()
	defer ipFloodMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-ipFloodWindow)
	window := ipHits[ip]
	kept := window[:0]
	for _, t := range window {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= ipFloodBurst {
		ipHits[ip] = kept
		return false
	}
	kept = append(kept, now)
	ipHits[ip] = kept
	return true
}

func replyfConn(client *ircClient, format string, args ...any) error {
	if client == nil || client.writer == nil {
		return nil
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	_, err := fmt.Fprintf(client.writer, format, args...)
	if err == nil {
		_, err = fmt.Fprint(client.writer, "\r\n")
	}
	if err == nil {
		err = client.writer.Flush()
	}
	return err
}

func replyf(conn net.Conn, format string, args ...any) error {
	if conn == nil {
		return nil
	}
	_, err := fmt.Fprintf(conn, format, args...)
	if err == nil {
		_, err = fmt.Fprint(conn, "\r\n")
	}
	return err
}
