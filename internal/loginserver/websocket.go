package loginserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/netutil"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/session"
)

type WebSocketServer struct {
	addr     string
	path     string
	logger   *slog.Logger
	auth     *auth.Service
	nodes    *session.Manager
	services sessionServices

	resolver *netutil.ProxyResolver
	upgrader websocket.Upgrader

	mu     sync.Mutex
	server *http.Server
	ln     net.Listener
	wg     sync.WaitGroup
}

func NewWebSocketServer(addr, path string, logger *slog.Logger, authSvc *auth.Service, nodes *session.Manager, trustedProxyCIDRs []string) (*WebSocketServer, error) {
	resolver, err := netutil.NewProxyResolver(trustedProxyCIDRs)
	if err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/ws-login"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return &WebSocketServer{
		addr:     strings.TrimSpace(addr),
		path:     path,
		logger:   logger,
		auth:     authSvc,
		nodes:    nodes,
		services: defaultSessionServices(),
		resolver: resolver,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return websocketOriginAllowed(r)
			},
		},
	}, nil
}

func websocketOriginAllowed(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil || strings.TrimSpace(originURL.Host) == "" {
		return false
	}
	requestHost := strings.TrimSpace(r.Host)
	if requestHost == "" {
		return false
	}
	originHost := originURL.Hostname()
	requestHostName := requestHost
	if parsed, err := url.Parse("//" + requestHost); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
		requestHostName = parsed.Hostname()
	}
	if !strings.EqualFold(originHost, requestHostName) {
		return false
	}
	if originURL.Scheme != "http" && originURL.Scheme != "https" {
		return false
	}
	return true
}

func (s *WebSocketServer) SetServices(boards repository.BoardRepository, messages repository.MessageRepository, mail repository.PrivateMailRepository, chatSvc *chat.Service, offlineDir string) {
	s.services.set(boards, messages, mail, chatSvc, offlineDir)
}

func (s *WebSocketServer) ListenAndServe() error {
	if s.addr == "" {
		return fmt.Errorf("websocket listen address is required")
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

func (s *WebSocketServer) ListenAndServeTLS(certFile, keyFile string) error {
	if s.addr == "" {
		return fmt.Errorf("websocket tls listen address is required")
	}
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" || keyFile == "" {
		return fmt.Errorf("websocket tls cert and key are required")
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.ServeTLS(ln, certFile, keyFile)
}

func (s *WebSocketServer) Serve(listener net.Listener) error {
	if listener == nil {
		return fmt.Errorf("listener is required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc(s.path, s.handleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.mu.Lock()
	s.server = srv
	s.ln = listener
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("starting websocket login server", "addr", listener.Addr().String(), "path", s.path)
	}
	err := srv.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (s *WebSocketServer) ServeTLS(listener net.Listener, certFile, keyFile string) error {
	if listener == nil {
		return fmt.Errorf("listener is required")
	}
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" || keyFile == "" {
		return fmt.Errorf("cert and key are required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc(s.path, s.handleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.mu.Lock()
	s.server = srv
	s.ln = listener
	s.mu.Unlock()

	if s.logger != nil {
		s.logger.Info("starting websocket tls login server", "addr", listener.Addr().String(), "path", s.path)
	}
	err := srv.ServeTLS(listener, certFile, keyFile)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (s *WebSocketServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		peer := &wsPeer{
			conn:       conn,
			remoteAddr: s.resolver.Resolve(r.RemoteAddr, r.Header),
		}
		runLoginSession("websocket", peer, s.auth, s.nodes, s.logger, s.services)
	}()
}

func (s *WebSocketServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.server
	ln := s.ln
	s.mu.Unlock()

	if ln != nil {
		_ = ln.Close()
	}
	if srv != nil {
		_ = srv.Shutdown(ctx)
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

type wsPeer struct {
	conn       *websocket.Conn
	remoteAddr string
	lineBuf    []rune
	pending    []string
}

func (p *wsPeer) ReadLine() (string, error) {
	if p.conn == nil {
		return "", io.EOF
	}
	if len(p.pending) > 0 {
		line := p.pending[0]
		p.pending = append([]string{}, p.pending[1:]...)
		return line, nil
	}
	for {
		mt, payload, err := p.conn.ReadMessage()
		if err != nil {
			return "", err
		}
		if mt != websocket.TextMessage {
			continue
		}
		if p.handleFrame(payload) {
			if len(p.pending) > 0 {
				line := p.pending[0]
				p.pending = append([]string{}, p.pending[1:]...)
				return line, nil
			}
			continue
		}
		return strings.TrimSpace(string(payload)), nil
	}
}

func (p *wsPeer) WriteLine(value string) error {
	if p.conn == nil {
		return io.EOF
	}
	return p.conn.WriteMessage(websocket.TextMessage, []byte(value))
}

func (p *wsPeer) RemoteAddr() string {
	return strings.TrimSpace(p.remoteAddr)
}

func (p *wsPeer) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

type wsFrame struct {
	Type string `json:"t"`
	Data string `json:"d"`
}

func (p *wsPeer) handleFrame(payload []byte) bool {
	var frame wsFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(frame.Type)) {
	case "ping":
		return true
	case "key":
		p.consumeKeyStream(frame.Data)
		return true
	default:
		return false
	}
}

func (p *wsPeer) consumeKeyStream(data string) {
	for _, r := range data {
		switch r {
		case '\r':
			continue
		case '\n':
			p.pending = append(p.pending, strings.TrimSpace(string(p.lineBuf)))
			p.lineBuf = p.lineBuf[:0]
		case '\b', 0x7f:
			if len(p.lineBuf) > 0 {
				p.lineBuf = p.lineBuf[:len(p.lineBuf)-1]
			}
		default:
			p.lineBuf = append(p.lineBuf, r)
		}
	}
}
