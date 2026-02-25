package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"

	"mini-atoms/internal/config"
	"mini-atoms/internal/httpapp"
	"mini-atoms/internal/store"
)

type Server struct {
	cfg        config.Config
	db         *gorm.DB
	sqlDB      *sql.DB
	httpServer *http.Server
}

func New(cfg config.Config) (*Server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("gorm db(): %w", err)
	}

	handler, err := httpapp.NewRouter(httpapp.Dependencies{
		Config: cfg,
		DB:     db,
	})
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("build router: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		cfg:        cfg,
		db:         db,
		sqlDB:      sqlDB,
		httpServer: httpSrv,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err, ok := <-errCh:
		if !ok {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var shutdownErr error
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownErr = fmt.Errorf("http shutdown: %w", err)
		}

		if err := s.sqlDB.Close(); err != nil && shutdownErr == nil {
			shutdownErr = fmt.Errorf("close db: %w", err)
		}

		if shutdownErr != nil {
			return shutdownErr
		}
		return nil
	}
}
