package router

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	scopeLen  int
	scope     *config.ScopedConfig
	route     config.RouteDef
	matchPath string
	pattern   string
	paramRE   *regexp.Regexp
	paramKeys []string
}

type distEntry struct {
	scopeLen int
	scope    *config.ScopedConfig
}

type Table struct {
	exact    []compiledRoute
	param    []compiledRoute
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
	v atomic.Value
}

func NewCache() *Cache {
	cache := &Cache{}
	cache.v.Store(&Snapshot{Table: &Table{byPrefix: map[string]*config.ScopedConfig{}}, Global: &config.GlobalConfig{}})
	return cache
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

func compileParamPattern(pattern string) (*regexp.Regexp, []string, error) {
	names, err := config.RouteParamNames(pattern)
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, nil
	}
	var builder strings.Builder
	builder.WriteString("^")
	cursor := 0
	re := regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_-]*)\}`)
	for _, loc := range re.FindAllStringSubmatchIndex(pattern, -1) {
		builder.WriteString(regexp.QuoteMeta(pattern[cursor:loc[0]]))
		builder.WriteString("([^/]+)")
		cursor = loc[1]
	}
	builder.WriteString(regexp.QuoteMeta(pattern[cursor:]))
	builder.WriteString("$")
	compiled, err := regexp.Compile(builder.String())
	if err != nil {
		return nil, nil, err
	}
	return compiled, names, nil
}

func LoadTable(root string, logger *slog.Logger) (*Table, error) {
	var scopes []*config.ScopedConfig
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != config.RoutesFileName {
			return nil
		}
		dir := filepath.Dir(filePath)
		scoped, err := config.LoadScopedConfig(root, dir)
		if err != nil {
			if logger != nil {
				logger.Warn("skipping invalid routes config", "path", filePath, "error", err)
			}
			return nil
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		if rel == "." {
			scoped.URLPrefix = ""
		} else {
			scoped.URLPrefix = "/" + filepath.ToSlash(rel)
		}
		scopes = append(scopes, &scoped)
		return nil
	})
	if err != nil {
		return nil, err
	}

	table := &Table{byPrefix: make(map[string]*config.ScopedConfig)}
	for _, scoped := range scopes {
		table.byPrefix[scoped.URLPrefix] = scoped
		for _, route := range scoped.Routes {
			compiled := compiledRoute{
				scopeLen:  len(scoped.URLPrefix),
				scope:     scoped,
				route:     route,
				matchPath: config.JoinURLPrefix(scoped.URLPrefix, route.From),
			}
			paramRE, paramKeys, err := compileParamPattern(compiled.matchPath)
			if err != nil {
				return nil, err
			}
			if paramRE != nil {
				compiled.paramRE = paramRE
				compiled.paramKeys = paramKeys
				table.param = append(table.param, compiled)
			} else if strings.Contains(route.From, "*") {
				compiled.pattern = compiled.matchPath
				table.wildcard = append(table.wildcard, compiled)
			} else {
				table.exact = append(table.exact, compiled)
			}
		}
		if scoped.Dist != nil {
			table.dist = append(table.dist, distEntry{scopeLen: len(scoped.URLPrefix), scope: scoped})
		}
		table.scopes = append(table.scopes, scoped)
	}

	sortCompiled := func(routes []compiledRoute) {
		sort.Slice(routes, func(i, j int) bool {
			if routes[i].scopeLen != routes[j].scopeLen {
				return routes[i].scopeLen > routes[j].scopeLen
			}
			return len(routes[i].matchPath) > len(routes[j].matchPath)
		})
	}
	sortCompiled(table.exact)
	sortCompiled(table.param)
	sortCompiled(table.wildcard)
	sort.Slice(table.dist, func(i, j int) bool { return table.dist[i].scopeLen > table.dist[j].scopeLen })

	if logger != nil {
		logger.Info("route table loaded",
			"scopes", len(scopes),
			"exact_routes", len(table.exact),
			"param_routes", len(table.param),
			"wildcard_routes", len(table.wildcard),
			"dist_routes", len(table.dist),
		)
	}
	return table, nil
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
	snapshot := r.Cache.Get()
	table := snapshot.Table
	if decision := r.matchExact(table, snapshot.Global, urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchParam(table, snapshot.Global, urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchWildcard(table, snapshot.Global, urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchGit(snapshot.Global, urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchDist(table, urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchStatic(urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchDirIndex(urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchReadme(urlPath); decision.Kind != KindNotFound {
		return decision
	}
	if decision := r.matchListing(table, urlPath); decision.Kind != KindNotFound {
		return decision
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchExact(table *Table, global *config.GlobalConfig, urlPath string) Decision {
	for _, route := range table.exact {
		if route.matchPath == urlPath {
			return r.decisionFromRoute(route, global, urlPath, nil)
		}
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchParam(table *Table, global *config.GlobalConfig, urlPath string) Decision {
	for _, route := range table.param {
		matches := route.paramRE.FindStringSubmatch(urlPath)
		if matches == nil {
			continue
		}
		params := make(map[string]string, len(route.paramKeys))
		allowed := true
		for i, key := range route.paramKeys {
			value := matches[i+1]
			if !config.ParamAllowed(route.route.Params[key], value) {
				allowed = false
				break
			}
			params[key] = value
		}
		if allowed {
			return r.decisionFromRoute(route, global, urlPath, params)
		}
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchGit(global *config.GlobalConfig, urlPath string) Decision {
	location, ok := config.ResolveGitRedirect(global, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	return Decision{Kind: KindRedirect, Status: http.StatusFound, Location: location}
}

func (r *Resolver) matchWildcard(table *Table, global *config.GlobalConfig, urlPath string) Decision {
	for _, route := range table.wildcard {
		if wildcardMatch(route.pattern, urlPath) {
			return r.decisionFromRoute(route, global, urlPath, nil)
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
		return urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/")
	}
	parts := strings.Split(pattern, "*")
	return len(parts) == 2 && strings.HasPrefix(urlPath, parts[0]) && strings.HasSuffix(urlPath, parts[1])
}

func (r *Resolver) decisionFromRoute(compiled compiledRoute, global *config.GlobalConfig, urlPath string, params map[string]string) Decision {
	route := compiled.route
	headers := copyHeaders(compiled.scope.Headers)
	target, err := config.InterpolateTarget(route.To, params, global)
	if err != nil {
		return Decision{Kind: KindNotFound}
	}

	switch route.Type {
	case config.RouteRedirect:
		location, err := config.NormalizeRedirectTarget(target)
		if err != nil {
			return Decision{Kind: KindNotFound}
		}
		if !strings.Contains(location, "://") && !strings.HasPrefix(location, "//") && filesystem.IsBlockedPath(location) {
			return Decision{Kind: KindNotFound}
		}
		status := route.Status
		if status == 0 {
			status = http.StatusFound
		}
		return Decision{Kind: KindRedirect, Status: status, Location: location, Headers: headers}
	case config.RouteRewrite, config.RouteFile:
		abs, err := config.ResolveTargetPath(r.Root, compiled.scope.DirPath, target)
		if err != nil || filesystem.IsBlockedAbsPath(abs) {
			return Decision{Kind: KindNotFound}
		}
		return Decision{
			Kind:     KindServeFile,
			AbsPath:  abs,
			Download: route.Type == config.RouteFile && route.Download,
			Headers:  headers,
			URLPath:  urlPath,
		}
	default:
		return Decision{Kind: KindNotFound}
	}
}

func (r *Resolver) matchDist(table *Table, urlPath string) Decision {
	for _, entry := range table.dist {
		scoped := entry.scope
		prefix := scoped.URLPrefix
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
			suffix = strings.TrimPrefix(strings.TrimPrefix(urlPath, prefix), "/")
		}
		var target string
		if suffix == "" {
			if scoped.Dist.Fallback != "" {
				target = scoped.Dist.Fallback
			} else {
				target = filepath.Join(scoped.Dist.Path, "index.html")
			}
		} else {
			target = filepath.Join(scoped.Dist.Path, filepath.FromSlash(suffix))
		}
		if stat, err := os.Stat(target); err == nil && !stat.IsDir() {
			return Decision{Kind: KindServeFile, AbsPath: target, Headers: copyHeaders(scoped.Headers), URLPath: urlPath}
		}
		if scoped.Dist.Fallback != "" {
			if stat, err := os.Stat(scoped.Dist.Fallback); err == nil && !stat.IsDir() {
				return Decision{Kind: KindServeFile, AbsPath: scoped.Dist.Fallback, Headers: copyHeaders(scoped.Headers), URLPath: urlPath}
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
	stat, err := os.Stat(abs)
	if err != nil || stat.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	return Decision{Kind: KindServeFile, AbsPath: abs, Headers: scopeHeadersForPath(r.Cache.Get().Table, urlPath), URLPath: urlPath}
}

func (r *Resolver) matchDirIndex(urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	stat, err := os.Stat(abs)
	if err != nil || !stat.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	for _, name := range []string{"index.html", "index.htm"} {
		indexPath := filepath.Join(abs, name)
		if stat, err := os.Stat(indexPath); err == nil && !stat.IsDir() {
			return Decision{Kind: KindServeFile, AbsPath: indexPath, Headers: scopeHeadersForPath(r.Cache.Get().Table, urlPath), URLPath: urlPath}
		}
	}
	return Decision{Kind: KindNotFound}
}

func (r *Resolver) matchReadme(urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	stat, err := os.Stat(abs)
	if err != nil || !stat.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	readmePath, found := markdown.FindReadme(abs)
	if !found {
		return Decision{Kind: KindNotFound}
	}
	return Decision{Kind: KindReadme, AbsPath: readmePath, Headers: scopeHeadersForPath(r.Cache.Get().Table, urlPath), URLPath: urlPath}
}

func (r *Resolver) matchListing(table *Table, urlPath string) Decision {
	abs, _, ok := filesystem.Resolve(r.Root, urlPath)
	if !ok {
		return Decision{Kind: KindNotFound}
	}
	stat, err := os.Stat(abs)
	if err != nil || !stat.IsDir() {
		return Decision{Kind: KindNotFound}
	}
	scoped := longestListingScope(table, urlPath)
	if scoped == nil || !scoped.Listing {
		return Decision{Kind: KindNotFound}
	}
	return Decision{Kind: KindListing, ListingDir: abs, Headers: copyHeaders(scoped.Headers), URLPath: urlPath}
}

func longestListingScope(table *Table, urlPath string) *config.ScopedConfig {
	var best *config.ScopedConfig
	bestLen := -1
	for _, scoped := range table.scopes {
		if !scoped.Listing {
			continue
		}
		prefix := scoped.URLPrefix
		if prefix == "" {
			prefix = "/"
		}
		if pathUnderPrefix(urlPath, prefix) || urlPath == strings.TrimSuffix(prefix, "/") {
			if len(scoped.URLPrefix) > bestLen {
				best = scoped
				bestLen = len(scoped.URLPrefix)
			}
		}
	}
	return best
}

func scopeHeadersForPath(table *Table, urlPath string) map[string]string {
	var best *config.ScopedConfig
	bestLen := -1
	for _, scoped := range table.scopes {
		prefix := scoped.URLPrefix
		if prefix == "" {
			prefix = "/"
		}
		if pathUnderPrefix(urlPath, prefix) || urlPath == strings.TrimSuffix(prefix, "/") {
			if len(scoped.URLPrefix) > bestLen {
				best = scoped
				bestLen = len(scoped.URLPrefix)
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
	return urlPath == prefix || strings.HasPrefix(urlPath, prefix+"/")
}

func normalizeURLPath(value string) string {
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	clean := path.Clean(value)
	if clean == "." {
		return "/"
	}
	return clean
}

func copyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}
