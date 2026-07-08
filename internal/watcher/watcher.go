package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/filesystem"
	"github.com/mewisme/mewroute/internal/router"
)

type Reloader struct {
	Root         string
	RouteCache   *router.Cache
	StatCache    *filesystem.StatCache
	Logger       *slog.Logger
	PollInterval time.Duration

	mu           sync.Mutex
	debounceDur  time.Duration
	routesDirty  bool
	pendingPaths []string
}

func NewReloader(root string, routeCache *router.Cache, statCache *filesystem.StatCache, logger *slog.Logger, pollInterval time.Duration) *Reloader {
	return &Reloader{
		Root:         root,
		RouteCache:   routeCache,
		StatCache:    statCache,
		Logger:       logger,
		PollInterval: pollInterval,
		debounceDur:  300 * time.Millisecond,
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
			_ = w.Add(path)
		}
		return nil
	})
}

func rescanWatches(w *fsnotify.Watcher, root string) int {
	added := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if err := w.Add(path); err == nil {
			added++
		}
		return nil
	})
	return added
}

func (r *Reloader) loop(ctx context.Context, w *fsnotify.Watcher) {
	defer w.Close()
	var timer *time.Timer

	flush := func(reason string) {
		r.mu.Lock()
		routesDirty := r.routesDirty
		changed := append([]string(nil), r.pendingPaths...)
		r.routesDirty = false
		r.pendingPaths = nil
		r.mu.Unlock()

		if r.StatCache != nil {
			r.StatCache.Clear()
		}

		if routesDirty {
			table, err := router.LoadTable(r.Root, r.Logger)
			if err != nil {
				r.Logger.Error("route reload failed", "error", err, "reason", reason)
				return
			}
			global, err := config.LoadGlobalConfig(r.Root)
			if err != nil {
				r.Logger.Error("global config reload failed", "error", err, "reason", reason)
				return
			}
			r.RouteCache.Store(table, global)
			r.Logger.Info("routes reloaded", "reason", reason, "paths", changed)
			return
		}

		if len(changed) > 0 {
			r.Logger.Info("content updated", "reason", reason, "paths", changed)
		}
	}

	var poll <-chan time.Time
	if r.PollInterval > 0 {
		ticker := time.NewTicker(r.PollInterval)
		defer ticker.Stop()
		poll = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return

		case <-poll:
			if n := rescanWatches(w, r.Root); n > 0 {
				r.Logger.Debug("watching new directories", "count", n)
			}
			if r.StatCache != nil {
				r.StatCache.Clear()
			}

		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				r.noteChange(ev.Name)
				r.schedule(func() { flush("fsnotify") }, &timer)
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

func (r *Reloader) noteChange(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingPaths = append(r.pendingPaths, path)
	if isRoutesFile(path) || isRoutesInPath(path) || isGlobalConfigFile(path) {
		r.routesDirty = true
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
	return strings.EqualFold(base, config.RoutesFileName) || strings.EqualFold(base, ".routes.yaml")
}

func isGlobalConfigFile(path string) bool {
	base := filepath.Base(path)
	return strings.EqualFold(base, config.GlobalConfigFileName) ||
		strings.EqualFold(base, ".config.yaml")
}

func isRoutesInPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/.routes.yml") ||
		strings.Contains(filepath.ToSlash(path), "/.routes.yaml")
}
