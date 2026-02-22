package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"wolfbbs/internal/chat"
)

type ircState struct {
	nick  string
	user  string
	auth  bool
	chann string
}

func main() {
	listen := flag.String("listen", ":6667", "IRC listen address")
	flag.Parse()

	svc := chat.NewService()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("WolfBBS IRC on", *listen)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleIRCConn(conn, svc)
	}
}

func handleIRCConn(conn net.Conn, svc *chat.Service) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	state := &ircState{}
	fmt.Fprintf(w, ":localhost 001 %s :Welcome to WolfBBS IRC\r\n", "*")
	fmt.Fprintf(w, ":localhost 002 %s :Your host is WolfBBS-IRCd\r\n", "*")
	fmt.Fprintf(w, ":localhost 003 %s :This server accepts BBS accounts\r\n", "*")
	fmt.Fprintf(w, ":localhost 004 %s WolfBBS :1.0 i\r\n", "*")
	fmt.Fprintf(w, ":localhost 375 %s :- WolfBBS IRC Message of the day\r\n", "*")
	fmt.Fprintf(w, ":localhost 372 %s :- Welcome to WolfBBS\r\n", "*")
	fmt.Fprintf(w, ":localhost 376 %s :End of MOTD\r\n", "*")
	_ = w.Flush()

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "NICK") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) < 2 {
				fmt.Fprintf(w, ":localhost 431 * :No nickname given\r\n")
				_ = w.Flush()
				continue
			}
			state.nick = strings.TrimSpace(parts[1])
			continue
		}
		if strings.HasPrefix(line, "USER") {
			parts := strings.SplitN(line, " ", 5)
			if len(parts) < 2 {
				fmt.Fprintf(w, ":localhost 461 %s USER :Not enough parameters\r\n", ensureNick(state.nick))
				_ = w.Flush()
				continue
			}
			state.user = parts[1]
			state.auth = true
			fmt.Fprintf(w, ":localhost 001 %s :Welcome %s\r\n", state.nick, state.nick)
			_ = w.Flush()
			continue
		}
		if strings.HasPrefix(line, "PASS") {
			continue
		}

		if strings.HasPrefix(line, "PING") {
			target := strings.TrimSpace(strings.TrimPrefix(line, "PING"))
			fmt.Fprintf(w, "PONG :%s\r\n", target)
			_ = w.Flush()
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		raw := ""
		if len(parts) == 2 {
			raw = parts[1]
		}

		if !state.auth || state.nick == "" {
			if cmd != "NICK" && cmd != "USER" {
				fmt.Fprintf(w, ":localhost 451 %s :You have not registered\r\n", ensureNick(state.nick))
				_ = w.Flush()
				continue
			}
		}

		switch cmd {
		case "JOIN":
			ch := strings.TrimPrefix(raw, ":")
			if ch == "" {
				fmt.Fprintf(w, ":localhost 403 %s %s :No such channel\r\n", state.nick, ch)
			} else {
				svc.JoinChannel(state.nick, ch)
				state.chann = ch
				fmt.Fprintf(w, ":%s JOIN :%s\r\n", state.nick, ch)
				fmt.Fprintf(w, ":localhost 353 %s = %s :%s\r\n", state.nick, ch, strings.Join(memberNames(svc), " "))
				fmt.Fprintf(w, ":localhost 366 %s %s :End of /NAMES list\r\n", state.nick, ch)
			}
		case "PART":
			ch := strings.Trim(raw, ":")
			if ch == "" {
				ch = state.chann
			}
			svc.LeaveChannel(state.nick, ch)
			fmt.Fprintf(w, ":%s PART :%s\r\n", state.nick, ch)
		case "PRIVMSG", "NOTICE":
			targetAndMsg := strings.SplitN(raw, " :", 2)
			if len(targetAndMsg) != 2 {
				fmt.Fprintf(w, ":localhost 421 %s %s :Unknown command\r\n", state.nick, cmd)
				break
			}
			target := targetAndMsg[0]
			msg := targetAndMsg[1]
			if _, err := svc.Post(state.nick, target, msg); err != nil {
				fmt.Fprintf(w, ":localhost 403 %s %s :Message rate limit\r\n", state.nick, target)
			} else {
				if cmd == "PRIVMSG" {
					fmt.Fprintf(w, ":%s PRIVMSG %s :%s\r\n", state.nick, target, msg)
				}
				for _, m := range svc.History(target, 3) {
					if m.ID == 0 {
						continue
					}
					fmt.Fprintf(w, ":%s PRIVMSG %s :%s\r\n", m.From, target, m.Body)
				}
			}
		case "NAMES":
			ch := strings.Trim(raw, ":")
			if ch == "" {
				ch = state.chann
			}
			fmt.Fprintf(w, ":localhost 353 %s = %s :%s\r\n", state.nick, ch, strings.Join(memberNames(svc), " "))
			fmt.Fprintf(w, ":localhost 366 %s %s :End of /NAMES list\r\n", state.nick, ch)
		case "LIST":
			for _, c := range svc.ListChannels() {
				fmt.Fprintf(w, ":localhost 322 %s %s 0 :Auto topics\r\n", state.nick, c)
			}
			fmt.Fprintf(w, ":localhost 323 %s :End of /LIST\r\n", state.nick)
		case "WHO":
			for _, p := range svc.Online() {
				fmt.Fprintf(w, ":localhost 352 %s %s %s %s %s %s H :0 %s\r\n", state.nick, p.Area, p.Node, "127.0.0.1", "*", p.Nick)
			}
			fmt.Fprintf(w, ":localhost 315 %s %s :End of WHO list\r\n", state.nick, strings.TrimPrefix(raw, ":"))
		case "WHOIS":
			target := strings.Trim(raw, ":")
			if target == "" {
				fmt.Fprintf(w, ":localhost 431 %s :No nickname given\r\n", state.nick)
			} else {
				fmt.Fprintf(w, ":localhost 311 %s %s localhost %s * :WolfBBS user\r\n", state.nick, target, target)
				fmt.Fprintf(w, ":localhost 318 %s %s :End of WHOIS\r\n", state.nick, target)
			}
		case "TOPIC":
			ch := strings.Trim(raw, ":")
			if ch == "" {
				fmt.Fprintf(w, ":localhost 403 %s %s :No such channel\r\n", state.nick, ch)
				break
			}
			fmt.Fprintf(w, ":localhost 332 %s %s :WolfBBS channel topic\r\n", state.nick, ch)
		case "MODE":
			fmt.Fprintf(w, ":localhost 221 %s +i\r\n", state.nick)
		case "KICK":
			fmt.Fprintf(w, ":localhost 482 %s :You are not channel operator\r\n", state.nick)
		case "QUIT":
			fmt.Fprintf(w, ":%s QUIT :%s\r\n", state.nick, strings.TrimPrefix(raw, ":"))
			_ = w.Flush()
			return
		default:
			fmt.Fprintf(w, ":localhost 421 %s %s :Unknown command\r\n", state.nick, cmd)
		}
		_ = w.Flush()
	}
}

func memberNames(svc *chat.Service) []string {
	p := svc.Online()
	out := make([]string, 0, len(p))
	for _, v := range p {
		out = append(out, v.Nick)
	}
	return out
}

func ensureNick(nick string) string {
	if nick == "" {
		return "*"
	}
	return nick
}
