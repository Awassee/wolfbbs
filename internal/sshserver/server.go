package sshserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/domain"
	"wolfbbs/internal/doors"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/repository"
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
	server  *gssh.Server
}

type screenState int

const (
	stateWelcome screenState = iota
	stateLogin
	stateBulletins
	stateMainMenu
	stateGateway
	stateLastCallers
	stateWhoOnline
	stateDoors
	stateExit
)

func New(address string, logger *slog.Logger, authSvc *auth.Service) *Server {
	s := &Server{address: address, logger: logger, auth: authSvc}
	s.server = &gssh.Server{
		Addr:            address,
		Handler:         s.handleSession,
		PasswordHandler: func(_ gssh.Context, _ string) bool { return true },
		IdleTimeout:     10 * time.Minute,
		MaxTimeout:      30 * time.Minute,
		PublicKeyHandler: func(_ gssh.Context, _ gssh.PublicKey) bool {
			return false
		},
	}
	return s
}

func (s *Server) SetRepositories(users repository.UserRepository, boards repository.BoardRepository, messages repository.MessageRepository, mail repository.PrivateMailRepository, admin repository.AdminRepository) {
	s.users = users
	s.boards = boards
	s.msgs = messages
	s.mail = mail
	s.admin = admin
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
	doors.SeedTrivia(doorRegistry, strings.TrimSpace(os.Getenv("WOLFBBS_TRIVIA_BINARY")))
	doors.SeedFromEnv(doorRegistry)
	offlineDir := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
	if offlineDir == "" {
		offlineDir = ".wolfbbs/offline"
	}
	emailGateway := gateway.NewEmailGateway(gateway.LoadEmailConfigFromEnv())
	th := ui.DefaultTheme()
	state := stateWelcome
	currentUser := "Guest"

	for {
		switch state {
		case stateWelcome:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS", currentUser, time.Now(), "Node 1", th)+"\r\n")
			io.WriteString(sess, "\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderWelcome(renderWidth))
			io.WriteString(sess, ui.FooterPrompt(renderWidth, "Press any key to continue")+"\r\n")
			if key, err := readKey(reader); err == nil {
				if key == "ESC" || key == "CTRL-C" || key == "Q" {
					state = stateExit
				} else {
					state = stateLogin
				}
			} else {
				return
			}
		case stateLogin:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS", "Guest", time.Now(), "Node 1", th)+"\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderLoginPrompt(renderWidth)+"\r\n")
			io.WriteString(sess, "Handle: ")
			handle, err := readLine(reader, 32)
			if err != nil {
				return
			}
			handle = strings.TrimSpace(handle)
			io.WriteString(sess, "Password: ")
			pass, err := readLine(reader, 64)
			if err != nil {
				return
			}
			pass = strings.TrimSpace(pass)
			if handle == "" || pass == "" {
				io.WriteString(sess, "\r\nMissing input. Press any key to retry.\r\n")
				_, _ = readKey(reader)
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
				if strings.EqualFold(strings.TrimSpace(choice), "Y") {
					createdAccount, createErr := s.auth.Register(handle, pass)
					if createErr != nil {
						io.WriteString(sess, fmt.Sprintf("\r\nCould not create account: %v\r\n", createErr))
						_, _ = readKey(reader)
						state = stateLogin
						continue
					}
					user = createdAccount
					created = true
					err = nil
				} else {
					io.WriteString(sess, "\r\nReturning to login.\r\n")
					_, _ = readKey(reader)
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
			}
			user, err = s.auth.Authenticate(handle, pass, strings.TrimSpace(secondFactor))
			if err != nil {
				if err == auth.ErrMissingSecondFactor || err == auth.ErrInvalidSecondFactor {
					io.WriteString(sess, "\r\nInvalid two-factor code. Press any key.\r\n")
				} else {
					io.WriteString(sess, "\r\nLogin failed. Press any key.\r\n")
				}
				_, _ = readKey(reader)
				state = stateLogin
				continue
			}
			if created {
				io.WriteString(sess, "\r\nWelcome to WolfBBS! Press any key to continue.\r\n")
				_, _ = readKey(reader)
			}

			currentUser = user.Handle
			s.logger.Info("user authenticated", "user", currentUser)
			state = stateBulletins
		case stateBulletins:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS", currentUser, time.Now(), "Node 1", th)+"\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderBulletinList(renderWidth, []string{
				"System maintenance window: Sundays 03:00 UTC.",
				"Gateways are rate-limited for abuse prevention.",
				"Use ? in menus for command help.",
			})+"\r\n")
			_, _ = readKey(reader)
			state = stateMainMenu
		case stateMainMenu:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS", currentUser, time.Now(), "Node 1", th)+"\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderMainMenu(renderWidth))
			io.WriteString(sess, ui.FooterPrompt(renderWidth, "Press Q to quit, ? for help")+"\r\n")
			io.WriteString(sess, "Enter selection: ")
			key, err := readKey(reader)
			if err != nil {
				return
			}
			s.logger.Info("menu selection", "user", currentUser, "selection", key)
			switch key {
			case "Q", "ESC":
				state = stateExit
			case "M":
				s.runBoards(sess, reader, termWidth, renderWidth, currentUser, th)
			case "P":
				s.runMail(sess, reader, termWidth, renderWidth, currentUser, th)
			case "G":
				state = stateGateway
			case "D":
				state = stateDoors
			case "F", "C", "S", "A":
				io.WriteString(sess, "\r\nSection scaffolded; implementation in next step. Press any key to continue.")
				_, _ = readKey(reader)
			case "L":
				state = stateLastCallers
			case "W":
				state = stateWhoOnline
			case "?":
				io.WriteString(sess, "\r\nHotkeys: M Message Boards, P Mail, F Files, C Chat, G Gateways, D Doors, S Settings, A Admin, L Last Callers, W Who's Online, Q Quit\r\n")
				_, _ = readKey(reader)
			default:
				if len(key) == 1 {
					io.WriteString(sess, "\r\nUse a single-letter hotkey listed in the menu.\r\n")
				} else {
					io.WriteString(sess, "\r\nUnknown key sequence.\r\n")
				}
				_, _ = readKey(reader)
			}
		case stateDoors:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS Doors", currentUser, time.Now(), "Node 1", th)+"\r\n")
			hotkeys := make([]string, 0)
			for _, door := range doorRegistry.Doors() {
				hotkeys = append(hotkeys, door.Hotkey)
			}
			renderFrame(sess, termWidth, renderWidth, ui.RenderDoorMenu(renderWidth, hotkeys)+"\r\n")
			io.WriteString(sess, "Selection: ")
			choice, err := readKey(reader)
			if err != nil {
				return
			}
			if choice == "R" || choice == "Q" || choice == "ESC" {
				state = stateMainMenu
				break
			}
			ctx, cancel := doors.DoorTimeout(context.Background(), 180*time.Second)
			err = doorRegistry.Launch(ctx, choice, sess, sess, sess, map[string]string{
				"WOLFBBS_HANDLE": currentUser,
				"WOLFBBS_NODE":   "1",
				"WOLFBBS_AREA":   "doors",
			})
			cancel()
			if err != nil {
				recordAudit(s.admin, currentUser, strings.ToUpper(choice), "door_failure", err.Error())
				io.WriteString(sess, "\r\nDoor launch failed: "+err.Error()+"\r\nPress any key to continue.")
				_, _ = readKey(reader)
			} else {
				recordAudit(s.admin, currentUser, strings.ToUpper(choice), "door_launch", "ok")
			}
			state = stateMainMenu
		case stateGateway:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS Gateway", currentUser, time.Now(), "Node 1", th)+"\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderGatewayMenu(renderWidth)+"\r\n")
			io.WriteString(sess, "Selection: ")
			gw, err := readKey(reader)
			if err != nil {
				return
			}
			switch gw {
			case "R", "Q", "ESC":
				state = stateMainMenu
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
					state = stateMainMenu
					continue
				}
				user, userErr := s.auth.GetUser(currentUser)
				if userErr != nil || user == nil {
					io.WriteString(sess, "\r\nCould not load current user. Press any key.\r\n")
					_, _ = readKey(reader)
					state = stateMainMenu
					continue
				}
				if !user.Verified {
					io.WriteString(sess, "\r\nAccount must be verified before external email. Press any key.\r\n")
					_, _ = readKey(reader)
					state = stateMainMenu
					continue
				}
				if s.admin != nil {
					policy, err := s.admin.GetMailOutboundPolicy(user.Handle)
					if err == nil && policy != nil && policy.OutboundDisabled {
						io.WriteString(sess, "\r\nOutbound email disabled for your account. Press any key.\r\n")
						_, _ = readKey(reader)
						state = stateMainMenu
						continue
					}
				}
				if err := emailGateway.SendOutbound(user.Handle, []string{to}, subject, body); err != nil {
					io.WriteString(sess, "\r\nOutbound failed: "+err.Error()+"\r\nPress any key.\r\n")
					_, _ = readKey(reader)
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
				state = stateMainMenu
			}
		case stateLastCallers:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS", currentUser, time.Now(), "Node 1", th)+"\r\n")
			io.WriteString(sess, "\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderLastCallers(renderWidth, []string{
				"01  riker      10:12  Main Lobby   00:14:22",
				"07  byteforge  09:44  Chat        00:02:17",
				"12  shells     09:33  Boards      00:43:05",
			}))
			_, _ = readKey(reader)
			state = stateMainMenu
		case stateWhoOnline:
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderWhoOnline(renderWidth, []string{
				"01  riker      2026-02-22 12:03   Main Lobby   00:01:31",
				"07  byteforge  2026-02-22 12:15   Messages    00:00:18",
			}))
			_, _ = readKey(reader)
			state = stateMainMenu
		case stateExit:
			io.WriteString(sess, ui.ClearScreen())
			io.WriteString(sess, "Signing off WolfBBS...\r\n")
			return
		}
	}
}

func renderFrame(out io.Writer, termWidth, contentWidth int, frame string) {
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
	switch ch {
	case 0x1b:
		next, err := reader.ReadByte()
		if err != nil {
			return "ESC", nil
		}
		if next != '[' {
			return "ESC", nil
		}
		next2, err := reader.ReadByte()
		if err != nil {
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
			p, _ := reader.ReadByte()
			if p == '~' {
				return "PGUP", nil
			}
		case '6':
			p, _ := reader.ReadByte()
			if p == '~' {
				return "PGDN", nil
			}
		case '1':
			mid, _ := reader.ReadByte()
			if mid != ';' {
				return "UNKNOWN", nil
			}
			tail, _ := reader.ReadByte()
			next, _ := reader.ReadByte()
			if tail == '5' && next == '~' {
				return "HOME", nil
			}
			if tail == '6' && next == '~' {
				return "END", nil
			}
		case '3':
			t, _ := reader.ReadByte()
			if t == '~' {
				return "DEL", nil
			}
		case '4':
			t, _ := reader.ReadByte()
			if t == '~' {
				return "END", nil
			}
		case '2':
			t, _ := reader.ReadByte()
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

func (s *Server) runBoards(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme) {
	if s.boards == nil || s.msgs == nil {
		io.WriteString(sess, "\r\nMessage board storage is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	currentUser, err := s.auth.GetUser(handle)
	if err != nil || currentUser == nil {
		io.WriteString(sess, "\r\nCould not load account for posting. Press any key.")
		_, _ = readKey(reader)
		return
	}
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

		names := make([]string, 0, len(boards))
		for _, board := range boards {
			names = append(names, fmt.Sprintf("[%d] %s", board.ID, board.Name))
		}

		io.WriteString(sess, ui.ClearScreen())
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS Boards", handle, time.Now(), "Node 1", th)+"\r\n")
		renderFrame(sess, termWidth, renderWidth, ui.RenderMessageBoardList(renderWidth, names)+"\r\n")
		io.WriteString(sess, "Select board ID (or Q to return): ")
		raw, err := readLine(reader, 16)
		if err != nil {
			return
		}
		raw = strings.TrimSpace(raw)
		if strings.EqualFold(raw, "q") {
			return
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

		for {
			msgs, err := s.msgs.ListByBoard(boardID)
			if err != nil {
				io.WriteString(sess, "\r\nCould not load messages. Press any key.")
				_, _ = readKey(reader)
				break
			}
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, selected.Name, handle, time.Now(), "Node 1", th)+"\r\n")
			if len(msgs) == 0 {
				io.WriteString(sess, "No messages yet.\r\n")
			} else {
				for _, msg := range msgs {
					threadMarker := " "
					if msg.ParentID > 0 {
						threadMarker = ">"
					}
					io.WriteString(sess, fmt.Sprintf("%4d %s %-28s  %s\r\n", msg.ID, threadMarker, clampForTTY(msg.Subject, 28), msg.CreatedAt.Format("2006-01-02 15:04")))
				}
			}
			io.WriteString(sess, "\r\nCommands: (N)ew, (R)ead, (Q)uit board, (?)help\r\nSelection: ")
			choice, err := readLine(reader, 24)
			if err != nil {
				return
			}
			choice = strings.TrimSpace(choice)
			switch strings.ToUpper(choice) {
			case "Q":
				goto nextBoard
			case "N":
				io.WriteString(sess, ui.ClearScreen())
				renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, selected.Name+" / New Post", handle, time.Now(), "Node 1", th)+"\r\n")
				renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, "")+"\r\n")
				io.WriteString(sess, "Subject: ")
				subject, err := readLine(reader, 120)
				if err != nil {
					return
				}
				body, err := readMessageBody(reader, 80, 2048)
				if err != nil {
					return
				}
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
				}
			case "R":
				if len(msgs) == 0 {
					io.WriteString(sess, "\r\nNo messages to read. Press any key.")
					_, _ = readKey(reader)
					continue
				}
				io.WriteString(sess, "Message ID (blank = first): ")
				rawID, err := readLine(reader, 20)
				if err != nil {
					return
				}
				startIndex := 0
				if trimmed := strings.TrimSpace(rawID); trimmed != "" {
					msgID, parseErr := strconv.ParseInt(trimmed, 10, 64)
					if parseErr != nil {
						io.WriteString(sess, "\r\nInvalid message ID. Press any key.")
						_, _ = readKey(reader)
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
						continue
					}
					startIndex = found
				}
				posted, err := s.runBoardReader(sess, reader, termWidth, renderWidth, selected.Name, handle, currentUser, boardID, msgs, startIndex, th)
				if err != nil {
					return
				}
				if posted {
					continue
				}
			case "?":
				io.WriteString(sess, "\r\nReader keys: (R)eply, (N)ext, (P)rev, (Q)uit, (?)help.\r\nPress any key.")
				_, _ = readKey(reader)
			default:
				io.WriteString(sess, "\r\nUnknown command. Press any key.")
				_, _ = readKey(reader)
			}
		}
	nextBoard:
	}
}

func (s *Server) runBoardReader(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, boardName, handle string, currentUser *domain.User, boardID int64, msgs []domain.Message, startIndex int, th ui.Theme) (bool, error) {
	if len(msgs) == 0 {
		return false, nil
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
		lines := strings.Split(strings.ReplaceAll(msg.Body, "\r\n", "\n"), "\n")
		title := fmt.Sprintf("#%d %s", msg.ID, msg.Subject)
		io.WriteString(sess, ui.ClearScreen())
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, boardName, handle, time.Now(), "Node 1", th)+"\r\n")
		renderFrame(sess, termWidth, renderWidth, ui.RenderMessageReader(renderWidth, title, lines, index+1, len(msgs))+"\r\n")
		io.WriteString(sess, "\r\nCommand (R/N/P/Q/?): ")
		key, err := readKey(reader)
		if err != nil {
			return posted, err
		}
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
			defaultSubject := "Re: " + msg.Subject
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, boardName+" / Reply", handle, time.Now(), "Node 1", th)+"\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, defaultSubject)+"\r\n")
			io.WriteString(sess, "\r\nReply subject ["+defaultSubject+"]: ")
			replySubject, err := readLine(reader, 120)
			if err != nil {
				return posted, err
			}
			replySubject = strings.TrimSpace(replySubject)
			if replySubject == "" {
				replySubject = defaultSubject
			}
			io.WriteString(sess, "\r\nEnter reply body, end with blank line then '.'\r\n")
			replyBody, err := readMessageBody(reader, 80, 4096)
			if err != nil {
				return posted, err
			}
			replyBody = strings.TrimSpace(replyBody)
			if replyBody == "" {
				io.WriteString(sess, "\r\nReply cancelled (empty body). Press any key.")
				_, _ = readKey(reader)
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
				continue
			}
			posted = true
			return posted, nil
		case "?", "H":
			io.WriteString(sess, "\r\n(R)eply (N)ext (P)rev (Q)uit (?)Help\r\nPress any key.")
			_, _ = readKey(reader)
		case "Q", "ESC":
			return posted, nil
		default:
			io.WriteString(sess, "\r\nUnknown command. Press any key.")
			_, _ = readKey(reader)
		}
	}
}

func (s *Server) runMail(sess gssh.Session, reader *bufio.Reader, termWidth, renderWidth int, handle string, th ui.Theme) {
	if s.mail == nil {
		io.WriteString(sess, "\r\nMail storage is unavailable. Press any key.")
		_, _ = readKey(reader)
		return
	}
	currentUser, err := s.auth.GetUser(handle)
	if err != nil || currentUser == nil {
		io.WriteString(sess, "\r\nCould not load account. Press any key.")
		_, _ = readKey(reader)
		return
	}
	for {
		inbox, _ := s.mail.ListInbox(currentUser.ID, 50)
		outbox, _ := s.mail.ListOutbox(currentUser.ID, 50)

		io.WriteString(sess, ui.ClearScreen())
		renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS Mail", handle, time.Now(), "Node 1", th)+"\r\n")
		io.WriteString(sess, "Inbox:\r\n")
		if len(inbox) == 0 {
			io.WriteString(sess, "  (empty)\r\n")
		}
		for _, row := range inbox {
			status := "new"
			if row.ReadAt != nil {
				status = "read"
			}
			io.WriteString(sess, fmt.Sprintf("  %4d  %-26s  %s  %s\r\n", row.ID, clampForTTY(row.Subject, 26), row.CreatedAt.Format("2006-01-02 15:04"), status))
		}
		io.WriteString(sess, "\r\nOutbox:\r\n")
		if len(outbox) == 0 {
			io.WriteString(sess, "  (empty)\r\n")
		}
		for _, row := range outbox {
			target := fmt.Sprintf("uid:%d", row.ToUserID)
			if row.ExternalTo != nil {
				target = *row.ExternalTo
			}
			io.WriteString(sess, fmt.Sprintf("  %4d  %-22s  %-18s\r\n", row.ID, clampForTTY(row.Subject, 22), clampForTTY(target, 18)))
		}

		io.WriteString(sess, "\r\nCommands: (C)ompose, (R)ead, (Q)uit mail\r\nSelection: ")
		choice, err := readLine(reader, 24)
		if err != nil {
			return
		}
		switch strings.ToUpper(strings.TrimSpace(choice)) {
		case "Q":
			return
		case "C":
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "WolfBBS Mail Compose", handle, time.Now(), "Node 1", th)+"\r\n")
			renderFrame(sess, termWidth, renderWidth, ui.RenderPostEditor(renderWidth, "")+"\r\n")
			io.WriteString(sess, "To handle or external email: ")
			to, err := readLine(reader, 128)
			if err != nil {
				return
			}
			io.WriteString(sess, "Subject: ")
			subject, err := readLine(reader, 120)
			if err != nil {
				return
			}
			body, err := readMessageBody(reader, 80, 4096)
			if err != nil {
				return
			}
			to = strings.TrimSpace(to)
			subject = strings.TrimSpace(subject)
			body = strings.TrimSpace(body)
			if to == "" || subject == "" || body == "" {
				io.WriteString(sess, "\r\nTo/subject/body are required. Press any key.")
				_, _ = readKey(reader)
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
					continue
				}
				msg.ToUserID = targetUser.ID
			}
			if err := s.mail.CreateMail(&msg); err != nil {
				io.WriteString(sess, "\r\nCould not send mail: "+err.Error()+"\r\nPress any key.")
				_, _ = readKey(reader)
				continue
			}
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
				continue
			}
			row, err := s.mail.GetMail(mailID)
			if err != nil {
				io.WriteString(sess, "\r\nMail not found. Press any key.")
				_, _ = readKey(reader)
				continue
			}
			if row.ToUserID != currentUser.ID && row.FromUserID != currentUser.ID {
				io.WriteString(sess, "\r\nNot authorized to read that mail. Press any key.")
				_, _ = readKey(reader)
				continue
			}
			if row.ToUserID == currentUser.ID {
				_ = s.mail.MarkRead(row.ID, time.Now().UTC())
			}
			io.WriteString(sess, ui.ClearScreen())
			renderFrame(sess, termWidth, renderWidth, ui.RenderTopBar(renderWidth, "Private Mail", handle, time.Now(), "Node 1", th)+"\r\n")
			io.WriteString(sess, "Subject: "+row.Subject+"\r\n")
			io.WriteString(sess, "Sent: "+row.CreatedAt.Format("2006-01-02 15:04:05")+"\r\n")
			if row.ExternalTo != nil {
				io.WriteString(sess, "External To: "+*row.ExternalTo+"\r\n")
			}
			io.WriteString(sess, "\r\n"+row.Body+"\r\n")
			io.WriteString(sess, "\r\nPress any key.")
			_, _ = readKey(reader)
		default:
			io.WriteString(sess, "\r\nUnknown command. Press any key.")
			_, _ = readKey(reader)
		}
	}
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
