package loginserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"wolfbbs/internal/auth"
	"wolfbbs/internal/netutil"
	"wolfbbs/internal/session"
)

type WebSocketServer struct {
	addr   string
	path   string
	logger *slog.Logger
	auth   *auth.Service
	nodes  *session.Manager

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
		resolver: resolver,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}, nil
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
		runLoginSession("websocket", peer, s.auth, s.nodes, s.logger)
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
}

func (p *wsPeer) ReadLine() (string, error) {
	if p.conn == nil {
		return "", io.EOF
	}
	for {
		mt, payload, err := p.conn.ReadMessage()
		if err != nil {
			return "", err
		}
		if mt != websocket.TextMessage {
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
