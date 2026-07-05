package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/filesystem"
	"github.com/mewisme/mewroute/internal/router"
)

type Reloader struct {
	Root       string
	RouteCache *router.Cache
	StatCache  *filesystem.StatCache
	Logger     *slog.Logger

	mu          sync.Mutex
	debounceDur time.Duration
}

func NewReloader(root string, routeCache *router.Cache, statCache *filesystem.StatCache, logger *slog.Logger) *Reloader {
	return &Reloader{
		Root:        root,
		RouteCache:  routeCache,
		StatCache:   statCache,
		Logger:      logger,
		debounceDur: 300 * time.Millisecond,
	}
}

func (r *Reloader) Start(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := addRecursive(w, r.Root); err != nil {
		_ = w.Close()
		return err
	}

	go r.loop(ctx, w)
	return nil
}

func addRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if err := w.Add(path); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Reloader) loop(ctx context.Context, w *fsnotify.Watcher) {
	defer w.Close()
	var timer *time.Timer

	flush := func() {
		table, err := router.LoadTable(r.Root, r.Logger)
		if err != nil {
			r.Logger.Error("route reload failed", "error", err)
			return
		}
		r.RouteCache.Store(table)
		r.Logger.Info("configuration reloaded")
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				if r.StatCache != nil {
					r.StatCache.InvalidatePrefix(ev.Name)
				}
				r.schedule(flush, &timer)
			}
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					_ = w.Add(ev.Name)
				}
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			r.Logger.Warn("watcher error", "error", err)
		}
	}
}

func (r *Reloader) schedule(flush func(), timer **time.Timer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if *timer != nil {
		(*timer).Stop()
	}
	*timer = time.AfterFunc(r.debounceDur, flush)
}

func isRoutesFile(path string) bool {
	base := filepath.Base(path)
	return base == config.RoutesFileName || base == ".routes.yaml"
}
