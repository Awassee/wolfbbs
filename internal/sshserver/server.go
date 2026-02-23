package sshserver

import (
	"bufio"
	"context"
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
	"wolfbbs/internal/doors"
	"wolfbbs/internal/gateway"
	"wolfbbs/internal/ui"
)

type Server struct {
	address string
	logger  *slog.Logger
	auth    *auth.Service
	server  *gssh.Server
}

type screenState int

const (
	stateWelcome screenState = iota
	stateLogin
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
	offlineDir := strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR"))
	if offlineDir == "" {
		offlineDir = ".wolfbbs/offline"
	}
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
			renderFrame(sess, termWidth, renderWidth, ui.RenderRegisterPrompt(renderWidth)+"\r\n")
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
			case "G":
				state = stateGateway
			case "D":
				state = stateDoors
			case "M", "P", "F", "C", "S", "A":
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
			renderFrame(sess, termWidth, renderWidth, ui.RenderDoorMenu(renderWidth, []string{"T"})+"\r\n")
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
				io.WriteString(sess, "\r\nDoor launch failed: "+err.Error()+"\r\nPress any key to continue.")
				_, _ = readKey(reader)
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
				io.WriteString(sess, "\r\nGateway mailbox integration is a placeholder in this build. Press any key.\r\n")
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
