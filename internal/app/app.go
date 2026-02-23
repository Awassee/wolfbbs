package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wolfbbs/internal/auth"
	"wolfbbs/internal/repository"
	"wolfbbs/internal/sshserver"
)

type Config struct {
	ListenAddr string
	DBURL      string
}

func Run(cfg Config) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

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
				Close:    func() {},
			}
		} else {
			return err
		}
	}
	defer storage.Close()

	authSvc := auth.NewService(storage.Users)
	server := sshserver.New(cfg.ListenAddr, logger, authSvc)
	server.SetRepositories(storage.Users, storage.Boards, storage.Messages, storage.Mail, storage.Admin)
	if cfg.DBURL != "" {
		logger.Info("using configured database", "url", cfg.DBURL)
	} else {
		logger.Info("using local repository fallback")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		return err
	}
}
