package router

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/filesystem"
	"github.com/mewisme/mewroute/internal/markdown"
)

type Kind int

const (
	KindNotFound Kind = iota
	KindRedirect
	KindServeFile
	KindListing
	KindReadme
)

type Decision struct {
	Kind       Kind
	AbsPath    string
	Status     int
	Location   string
	Download   bool
	Headers    map[string]string
	URLPath    string
	ScopeDir   string
	ListingDir string
}

type compiledRoute struct {
	scopeLen   int
	scope      *config.ScopedConfig
	route      config.RouteDef
	matchPath  string
	wildcard   bool
	pattern    string
	prefixPart string
}

type distEntry struct {
	scopeLen int
	scope    *config.ScopedConfig
}

type Table struct {
	exact    []compiledRoute
	wildcard []compiledRoute
	dist     []distEntry
	scopes   []*config.ScopedConfig
	byPrefix map[string]*config.ScopedConfig
}

type Snapshot struct {
	Table  *Table
	Global *config.GlobalConfig
}

type Cache struct {
	v atomic.Value // *Snapshot
}

func NewCache() *Cache {
	c := &Cache{}
	c.v.Store(&Snapshot{
		Table:  &Table{byPrefix: map[string]*config.ScopedConfig{}},
		Global: &config.GlobalConfig{},
	})
	return c
}

func (c *Cache) Get() *Snapshot {
	return c.v.Load().(*Snapshot)
}

func (c *Cache) Store(table *Table, global *config.GlobalConfig) {
	if global == nil {
		global = &config.GlobalConfig{}
	}
	c.v.Store(&Snapshot{Table: table, Global: global})
}

func (t *Table) Scopes() []*config.ScopedConfig {
	return t.scopes
}

func LoadTable(root string, logger *slog.Logger) (*Table, error) {
	var scopes []*config.ScopedConfig

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != config.RoutesFileName {
			return nil
		}
		dir := filepath.Dir(p)
		sc, err := config.LoadScopedConfig(root, dir)
		if err != nil {
			if logger != nil {
				logger.Warn("skipping invalid routes config", "path", p, "error", err)
			}
			return nil
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		if rel == "." {
			sc.URLPrefix = ""
		} else {
			sc.URLPrefix = "/" + filepath.ToSlash(rel)
		}
		scopes = append(scopes, &sc)
		return nil
	})
	if err != nil {
		return nil, err
	}

	t := &Table{byPrefix: make(map[string]*config.ScopedConfig)}
	for _, sc := range scopes {
		t.byPrefix[sc.URLPrefix] = sc
		for _, r := range sc.Routes {
			cr := compiledRoute{
				scopeLen:  len(sc.URLPrefix),
				scope:     sc,
				route:     r,
				matchPath: config.JoinURLPrefix(sc.URLPrefix, r.From),
			}
			if strings.Contains(r.From, "*") {
				cr.wildcard = true
				cr.pattern = cr.matchPath
				if idx := strings.Index(cr.matchPath, "*"); idx >= 0 {
					cr.prefixPart = cr.matchPath[:idx]
				}
				t.wildcard = append(t.wildcard, cr)
			} else {
				t.exact = append(t.exact, cr)
			}
		}
		if sc.Dist != nil {
			t.dist = append(t.dist, distEntry{scopeLen: len(sc.URLPrefix), scope: sc})
		}
		t.scopes = append(t.scopes, sc)
	}

	sort.Slice(t.exact, func(i, j int) bool {
		if t.exact[i].scopeLen != t.exact[j].scopeLen {
			return t.exact[i].scopeLen > t.exact[j].scopeLen
		}
		return len(t.exact[i].matchPath) > len(t.exact[j].matchPath)
	})
	sort.Slice(t.wildcard, func(i, j int) bool {
		if t.wildcard[i].scopeLen != t.wildcard[j].scopeLen {
			return t.wildcard[i].scopeLen > t.wildcard[j].scopeLen
		}
		return len(t.wildcard[i].matchPath) > len(t.wildcard[j].matchPath)
	})
	sort.Slice(t.dist, func(i, j int) bool {
		return t.dist[i].scopeLen > t.dist[j].scopeLen
	})

	if logger != nil {
		logger.Info("route table loaded",
			"scopes", len(scopes),
			"exact_routes", len(t.exact),
			"wildcard_routes", len(t.wildcard),
			"dist_routes", len(t.dist),
		)
	}
	return t, nil
}

type Resolver struct {
	Root  string
	Cache *Cache
}

func NewResolver(root string, cache *Cache) *Resolver {
	return &Resolver{Root: root, Cache: cache}
}

func (r *Resolver) Resolve(urlPath string) Decision {
	urlPath = normalizeURLPath(urlPath)
	if filesystem.IsBlockedPath(urlPath) {
		return Decision{Kind: KindNotFound}
	}

	snap := r.Cache.Get()
	t := snap.Table

	if d := r.matchExact(t, urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchWildcard(t, urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchGit(snap.Global, urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchDist(t, urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchStatic(urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchDirIndex(urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchReadme(urlPath); d.Kind != KindNotFound {
		return d
	}
	if d := r.matchListing(t, urlPath); d.Kind != KindNotFound {
		return d
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchExact(t *Table, urlPath string) Decision {
	for _, cr := range t.exact {
		if cr.matchPath == urlPath {
			return r.decisionFromRoute(cr, urlPath)
		}
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchGit(global *config.GlobalConfig, urlPath string) Decision {
	loc, ok := config.ResolveGitRedirect(global, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	return Decision{Kind: KindRedirect, Status: http.StatusFound, Location: loc}
}

func (r *Resolver) matchWildcard(t *Table, urlPath string) Decision {
	for _, cr := range t.wildcard {
		if wildcardMatch(cr.pattern, urlPath) {
			return r.decisionFromRoute(cr, urlPath)
		}
	}
	return Decision{Kind: KindNotFound}
}

func wildcardMatch(pattern, urlPath string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == urlPath
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if urlPath == prefix {
			return true
		}
		return strings.HasPrefix(urlPath, prefix+"/")
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		return strings.HasPrefix(urlPath, parts[0]) && strings.HasSuffix(urlPath, parts[1])
	}
	return false
}

func (r *Resolver) decisionFromRoute(cr compiledRoute, urlPath string) Decision {
	route := cr.route
	headers := copyHeaders(cr.scope.Headers)

	switch route.Type {
	case config.RouteRedirect:
		loc, err := config.NormalizeRedirectTarget(route.To)
		if err != nil {
			return Decision{Kind: KindNotFound}
		}
		if !strings.Contains(loc, "://") && !strings.HasPrefix(loc, "//") && filesystem.IsBlockedPath(loc) {
			return Decision{Kind: KindNotFound}
		}
		status := route.Status
		if status == 0 {
			status = http.StatusFound
		}
		return Decision{Kind: KindRedirect, Status: status, Location: loc, Headers: headers}
	case config.RouteRewrite:
		abs, err := config.ResolveTargetPath(r.Root, cr.scope.DirPath, route.To)
		if err != nil || filesystem.IsBlockedAbsPath(abs) {
			return Decision{Kind: KindNotFound}
		}
		return Decision{Kind: KindServeFile, AbsPath: abs, Headers: headers, URLPath: urlPath}
	case config.RouteFile:
		abs, err := config.ResolveTargetPath(r.Root, cr.scope.DirPath, route.To)
		if err != nil || filesystem.IsBlockedAbsPath(abs) {
			return Decision{Kind: KindNotFound}
		}
		return Decision{Kind: KindServeFile, AbsPath: abs, Download: route.Download, Headers: headers, URLPath: urlPath}
	default:
		return Decision{Kind: KindNotFound}
	}
}

func (r *Resolver) matchDist(t *Table, urlPath string) Decision {
	for _, de := range t.dist {
		sc := de.scope
		prefix := sc.URLPrefix
		if prefix == "" {
			// dist at root applies to all paths only if explicitly at root - skip unless url matches root subtree
			prefix = ""
		}

		if prefix != "" {
			trimmed := strings.TrimSuffix(prefix, "/")
			if urlPath != trimmed && urlPath != prefix && !strings.HasPrefix(urlPath, prefix+"/") {
				continue
			}
		}

		var suffix string
		if prefix == "" {
			suffix = strings.TrimPrefix(urlPath, "/")
		} else {
			suffix = strings.TrimPrefix(urlPath, prefix)
			suffix = strings.TrimPrefix(suffix, "/")
		}

		var target string
		if suffix == "" {
			if sc.Dist.Fallback != "" {
				target = sc.Dist.Fallback
			} else {
				target = filepath.Join(sc.Dist.Path, "index.html")
			}
		} else {
			target = filepath.Join(sc.Dist.Path, filepath.FromSlash(suffix))
		}

		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			return Decision{Kind: KindServeFile, AbsPath: target, Headers: copyHeaders(sc.Headers), URLPath: urlPath}
		}

		if sc.Dist.Fallback != "" {
			if st, err := os.Stat(sc.Dist.Fallback); err == nil && !st.IsDir() {
				return Decision{Kind: KindServeFile, AbsPath: sc.Dist.Fallback, Headers: copyHeaders(sc.Headers), URLPath: urlPath}
			}
		}
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchStatic(urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	st, err := os.Stat(abs)
	if err != nil || st.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	headers := scopeHeadersForPath(r.Cache.Get().Table, urlPath)
	return Decision{Kind: KindServeFile, AbsPath: abs, Headers: headers, URLPath: urlPath}
}

func (r *Resolver) matchDirIndex(urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	for _, name := range []string{"index.html", "index.htm"} {
		idx := filepath.Join(abs, name)
		if st, err := os.Stat(idx); err == nil && !st.IsDir() {
			headers := scopeHeadersForPath(r.Cache.Get().Table, urlPath)
			return Decision{Kind: KindServeFile, AbsPath: idx, Headers: headers, URLPath: urlPath}
		}
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchReadme(urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	readmePath, found := markdown.FindReadme(abs)
	if !found {
		return Decision{Kind: KindNotFound}
	}
	headers := scopeHeadersForPath(r.Cache.Get().Table, urlPath)
	return Decision{
		Kind:    KindReadme,
		AbsPath: readmePath,
		Headers: headers,
		URLPath: urlPath,
	}
}

func (r *Resolver) matchListing(t *Table, urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return Decision{Kind: KindNotFound}
	}

	sc := longestListingScope(t, urlPath)
	if sc == nil || !sc.Listing {
		return Decision{Kind: KindNotFound}
	}
	return Decision{
		Kind:       KindListing,
		ListingDir: abs,
		Headers:    copyHeaders(sc.Headers),
		URLPath:    urlPath,
	}
}

func longestListingScope(t *Table, urlPath string) *config.ScopedConfig {
	var best *config.ScopedConfig
	bestLen := -1
	for _, sc := range t.scopes {
		if !sc.Listing {
			continue
		}
		prefix := sc.URLPrefix
		if prefix == "" {
			prefix = "/"
		}
		if pathUnderPrefix(urlPath, prefix) || urlPath == strings.TrimSuffix(prefix, "/") {
			if len(sc.URLPrefix) > bestLen {
				best = sc
				bestLen = len(sc.URLPrefix)
			}
		}
	}
	return best
}

func scopeHeadersForPath(t *Table, urlPath string) map[string]string {
	var best *config.ScopedConfig
	bestLen := -1
	for _, sc := range t.scopes {
		prefix := sc.URLPrefix
		if prefix == "" {
			prefix = "/"
		}
		if pathUnderPrefix(urlPath, prefix) || urlPath == strings.TrimSuffix(prefix, "/") {
			if len(sc.URLPrefix) > bestLen {
				best = sc
				bestLen = len(sc.URLPrefix)
			}
		}
	}
	if best == nil {
		return nil
	}
	return copyHeaders(best.Headers)
}

func pathUnderPrefix(urlPath, prefix string) bool {
	if prefix == "" || prefix == "/" {
		return true
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if urlPath == prefix {
		return true
	}
	return strings.HasPrefix(urlPath, prefix+"/")
}

func normalizeURLPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." {
		return "/"
	}
	return clean
}

func copyHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}
