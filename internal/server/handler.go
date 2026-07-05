package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/mewisme/mewroute/internal/filesystem"
	"github.com/mewisme/mewroute/internal/listing"
	"github.com/mewisme/mewroute/internal/markdown"
	"github.com/mewisme/mewroute/internal/router"
)

type Handler struct {
	Resolver *router.Resolver
	Files    *filesystem.Server
	Logger   *slog.Logger
}

func NewHandler(resolver *router.Resolver, files *filesystem.Server, logger *slog.Logger) *Handler {
	return &Handler{Resolver: resolver, Files: files, Logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("ok"))
		}
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	decision := h.Resolver.Resolve(r.URL.Path)
	filesystem.ApplyHeaders(w, decision.Headers)

	switch decision.Kind {
	case router.KindRedirect:
		if r.Method == http.MethodHead {
			w.Header().Set("Location", decision.Location)
			w.WriteHeader(decision.Status)
			return
		}
		http.Redirect(w, r, decision.Location, decision.Status)
	case router.KindServeFile:
		if err := filesystem.ServeFile(w, r, decision.AbsPath, decision.Download); err != nil {
			h.Logger.Debug("serve file failed", "path", decision.AbsPath, "error", err)
			http.NotFound(w, r)
		}
	case router.KindListing:
		if err := listing.Write(w, decision.ListingDir, decision.URLPath); err != nil {
			h.Logger.Warn("listing failed", "path", decision.ListingDir, "error", err)
			http.NotFound(w, r)
		}
	case router.KindReadme:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := markdown.Write(w, decision.AbsPath, decision.URLPath); err != nil {
			h.Logger.Warn("readme render failed", "path", decision.AbsPath, "error", err)
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func MethodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
	w.WriteHeader(http.StatusMethodNotAllowed)
}
