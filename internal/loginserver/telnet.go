package loginserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/session"
)

type TelnetServer struct {
	addr   string
	logger *slog.Logger
	auth   *auth.Service
	nodes  *session.Manager

	mu sync.Mutex
	ln net.Listener
	wg sync.WaitGroup
}

func NewTelnetServer(addr string, logger *slog.Logger, authSvc *auth.Service, nodes *session.Manager) *TelnetServer {
	return &TelnetServer{
		addr:   strings.TrimSpace(addr),
		logger: logger,
		auth:   authSvc,
		nodes:  nodes,
	}
}

func (s *TelnetServer) ListenAndServe() error {
	if s.addr == "" {
		return fmt.Errorf("telnet listen address is required")
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

func (s *TelnetServer) Serve(listener net.Listener) error {
	if listener == nil {
		return fmt.Errorf("listener is required")
	}
	s.mu.Lock()
	s.ln = listener
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("starting telnet login server", "addr", listener.Addr().String())
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			runLoginSession("telnet", newTelnetPeer(c), s.auth, s.nodes, s.logger)
		}(conn)
	}
}

func (s *TelnetServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

type telnetPeer struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func newTelnetPeer(conn net.Conn) *telnetPeer {
	return &telnetPeer{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

func (p *telnetPeer) ReadLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimRight(line, "\r\n")), nil
}

func (p *telnetPeer) WriteLine(value string) error {
	if _, err := p.writer.WriteString(value + "\r\n"); err != nil {
		return err
	}
	return p.writer.Flush()
}

func (p *telnetPeer) RemoteAddr() string {
	if p.conn == nil || p.conn.RemoteAddr() == nil {
		return ""
	}
	return p.conn.RemoteAddr().String()
}

func (p *telnetPeer) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}
