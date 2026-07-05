package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/filesystem"
	"github.com/mewisme/mewroute/internal/router"
	"github.com/mewisme/mewroute/internal/server"
	"github.com/mewisme/mewroute/internal/watcher"
)

func Run(ctx context.Context, env config.Env, logger *slog.Logger) error {
	root, err := filepath.Abs(env.RootDir)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	if st, err := os.Stat(root); err != nil {
		return fmt.Errorf("root directory %q: %w", root, err)
	} else if !st.IsDir() {
		return fmt.Errorf("root %q is not a directory", root)
	}

	statCache := filesystem.NewStatCache()
	routeCache := router.NewCache()

	table, err := router.LoadTable(root, logger)
	if err != nil {
		return fmt.Errorf("load routes: %w", err)
	}
	routeCache.Store(table)

	resolver := router.NewResolver(root, routeCache)
	fileServer := filesystem.NewServer(root, statCache)
	handler := server.NewHandler(resolver, fileServer, logger)

	reloader := watcher.NewReloader(root, routeCache, statCache, logger, env.WatchPollInterval)
	if err := reloader.Start(ctx); err != nil {
		logger.Warn("file watcher unavailable", "error", err)
	}

	addr := fmt.Sprintf(":%d", env.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  env.ReadTimeout,
		WriteTimeout: env.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", addr, "root", root)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
