package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/chat"
	"wolfbbs/internal/config"
	"wolfbbs/internal/events"
	"wolfbbs/internal/logging"
	"wolfbbs/internal/loginserver"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/session"
	"wolfbbs/internal/sshserver"
	"wolfbbs/internal/ui"
)

type Config struct {
	ListenAddr string
	DBURL      string
}

func Run(cfg Config) error {
	logger := logging.NewLogger("wolfbbs-ssh")
	if err := ui.LoadThemesFromEnv(); err != nil {
		logger.Warn("theme config load failed; using built-in themes", "error", err)
	}

	storage, err := repository.OpenStorageFromEnv(cfg.DBURL)
	if err != nil {
		if cfg.DBURL == "" {
			logger.Warn("storage init failed; using in-memory repositories", "error", err)
			storage = &repository.Storage{
				Users:    repository.NewInMemoryUserRepository(),
				Boards:   repository.NewInMemoryBoardRepository(),
				Messages: repository.NewInMemoryMessageRepository(),
				Mail:     repository.NewInMemoryPrivateMailRepository(),
				Admin:    repository.NewInMemoryAdminRepository(),
				Doors:    repository.NewInMemoryDoorRepository(),
				Resets:   repository.NewInMemoryPasswordResetRepository(),
				Close:    func() {},
			}
		} else {
			return err
		}
	}
	defer storage.Close()

	authSvc := auth.NewService(storage.Users)
	authSvc.SetPasswordResetRepository(storage.Resets)
	bus := events.NewBus()
	bus.Subscribe("*", func(ev events.Event) {
		logger.Debug("event", "name", ev.Name, "fields", ev.Fields)
	})
	authSvc.SetEventBus(bus)
	server := sshserver.New(cfg.ListenAddr, logger, authSvc)
	server.SetEventBus(bus)
	nodeMgr := session.NewManager(255, 256)
	server.SetSessionManager(nodeMgr)
	server.SetRepositories(storage.Users, storage.Boards, storage.Messages, storage.Mail, storage.Admin, storage.Doors)
	chatSvc := chat.NewServiceWithStorage(cfg.DBURL)
	chatSvc.SetEventBus(bus)
	server.SetChatService(chatSvc)
	if cfg.DBURL != "" {
		logger.Info("using configured database", "url", cfg.DBURL)
	} else {
		logger.Info("using local repository fallback")
	}

	runtimeCfg, runtimeErr := config.CachedRuntime()
	if runtimeErr != nil {
		logger.Warn("runtime config invalid; using defaults for optional login transports", "error", runtimeErr)
		runtimeCfg = config.DefaultRuntime()
	}

	var telnetSrv *loginserver.TelnetServer
	if runtimeCfg.Login.Telnet.Enabled {
		telnetSrv = loginserver.NewTelnetServer(runtimeCfg.Login.Telnet.Listen, logger, authSvc, nodeMgr)
		telnetSrv.SetServices(storage.Boards, storage.Messages, storage.Mail, chatSvc, strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR")))
	}

	var wsSrv *loginserver.WebSocketServer
	if runtimeCfg.Login.WebSocket.Enabled {
		wsSrv, err = loginserver.NewWebSocketServer(
			runtimeCfg.Login.WebSocket.Listen,
			runtimeCfg.Login.WebSocket.Path,
			logger,
			authSvc,
			nodeMgr,
			parseCSV(runtimeCfg.Login.TrustedProxies),
		)
		if err != nil {
			return fmt.Errorf("websocket login server config: %w", err)
		}
		wsSrv.SetServices(storage.Boards, storage.Messages, storage.Mail, chatSvc, strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR")))
	}
	var wssSrv *loginserver.WebSocketServer
	if runtimeCfg.Login.WebSocketTLS.Enabled {
		wssSrv, err = loginserver.NewWebSocketServer(
			runtimeCfg.Login.WebSocketTLS.Listen,
			runtimeCfg.Login.WebSocketTLS.Path,
			logger,
			authSvc,
			nodeMgr,
			parseCSV(runtimeCfg.Login.TrustedProxies),
		)
		if err != nil {
			return fmt.Errorf("websocket tls login server config: %w", err)
		}
		wssSrv.SetServices(storage.Boards, storage.Messages, storage.Mail, chatSvc, strings.TrimSpace(os.Getenv("WOLFBBS_OFFLINE_DIR")))
	}

	errCh := make(chan error, 4)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("ssh server: %w", err)
		}
	}()
	if telnetSrv != nil {
		go func() {
			if err := telnetSrv.ListenAndServe(); err != nil {
				errCh <- fmt.Errorf("telnet login server: %w", err)
			}
		}()
	}
	if wsSrv != nil {
		go func() {
			if err := wsSrv.ListenAndServe(); err != nil {
				errCh <- fmt.Errorf("websocket login server: %w", err)
			}
		}()
	}
	if wssSrv != nil {
		go func() {
			if err := wssSrv.ListenAndServeTLS(runtimeCfg.Login.WebSocketTLS.Cert, runtimeCfg.Login.WebSocketTLS.Key); err != nil {
				errCh <- fmt.Errorf("websocket tls login server: %w", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var shutdownErr error
		if wssSrv != nil {
			if err := wssSrv.Shutdown(ctx); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		if wsSrv != nil {
			if err := wsSrv.Shutdown(ctx); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		if telnetSrv != nil {
			if err := telnetSrv.Shutdown(ctx); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		if err := server.Shutdown(ctx); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
		return shutdownErr
	case err := <-errCh:
		return err
	}
}

func parseCSV(raw string) []string {
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
