package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mewisme/mewroute/internal/filesystem"
	"gopkg.in/yaml.v3"
)

const RoutesFileName = ".routes.yml"

var allowedRedirectStatus = map[int]bool{
	http.StatusMovedPermanently:  true,
	http.StatusFound:             true,
	http.StatusTemporaryRedirect: true,
	http.StatusPermanentRedirect: true,
}

type RouteType string

const (
	RouteRewrite  RouteType = "rewrite"
	RouteRedirect RouteType = "redirect"
	RouteFile     RouteType = "file"
)

type RouteDef struct {
	From     string               `yaml:"from"`
	To       string               `yaml:"to"`
	Type     RouteType            `yaml:"type"`
	Status   int                  `yaml:"status"`
	Download bool                 `yaml:"download"`
	Params   map[string]ParamRule `yaml:"params"`
}

type DistDef struct {
	Path     string `yaml:"path"`
	Fallback string `yaml:"fallback"`
}

type RoutesFile struct {
	Routes  []RouteDef        `yaml:"routes"`
	Dist    *DistDef          `yaml:"dist"`
	Listing *bool             `yaml:"listing"`
	Headers map[string]string `yaml:"headers"`
}

type ScopedConfig struct {
	DirPath   string
	URLPrefix string
	Routes    []RouteDef
	Dist      *DistDef
	Listing   bool
	Headers   map[string]string
}

func ParseRoutesFile(data []byte) (RoutesFile, error) {
	var rf RoutesFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return RoutesFile{}, fmt.Errorf("parse yaml: %w", err)
	}
	return rf, nil
}

func LoadScopedConfig(contentRoot, dirPath string) (ScopedConfig, error) {
	dirPath = filepath.Clean(dirPath)
	contentRoot = filepath.Clean(contentRoot)
	configPath := filepath.Join(dirPath, RoutesFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ScopedConfig{}, err
	}

	rf, err := ParseRoutesFile(data)
	if err != nil {
		return ScopedConfig{}, fmt.Errorf("%s: %w", configPath, err)
	}

	sc := ScopedConfig{DirPath: dirPath, Headers: copyHeaders(rf.Headers)}
	if rf.Listing != nil {
		sc.Listing = *rf.Listing
	}

	for i, route := range rf.Routes {
		if err := validateRoute(route, contentRoot, dirPath); err != nil {
			return ScopedConfig{}, fmt.Errorf("%s route[%d]: %w", configPath, i, err)
		}
		sc.Routes = append(sc.Routes, route)
	}

	if rf.Dist != nil {
		dist := *rf.Dist
		if dist.Path == "" {
			return ScopedConfig{}, fmt.Errorf("%s: dist.path is required", configPath)
		}
		absDist, err := ResolveTargetPath(contentRoot, dirPath, dist.Path)
		if err != nil {
			return ScopedConfig{}, fmt.Errorf("%s dist: %w", configPath, err)
		}
		if st, err := os.Stat(absDist); err != nil || !st.IsDir() {
			return ScopedConfig{}, fmt.Errorf("%s dist: path %q does not exist or is not a directory", configPath, dist.Path)
		}
		dist.Path = absDist
		if dist.Fallback != "" {
			fallback, err := ResolveTargetPath(contentRoot, absDist, dist.Fallback)
			if err != nil {
				return ScopedConfig{}, fmt.Errorf("%s dist fallback: %w", configPath, err)
			}
			dist.Fallback = fallback
		}
		sc.Dist = &dist
	}
	return sc, nil
}

func validateRoute(route RouteDef, contentRoot, dirPath string) error {
	if strings.TrimSpace(route.From) == "" {
		return fmt.Errorf("from is required")
	}
	if strings.TrimSpace(route.To) == "" {
		return fmt.Errorf("to is required")
	}
	if err := ValidateRouteParams(route); err != nil {
		return err
	}
	if filesystem.IsBlockedPath(NormalizeFromPath(route.From)) {
		return fmt.Errorf("cannot map config file as route from")
	}

	dynamicTarget := strings.Contains(route.To, "{")
	switch route.Type {
	case RouteRewrite, RouteFile:
		if dynamicTarget {
			return nil
		}
		abs, err := ResolveTargetPath(contentRoot, dirPath, route.To)
		if err != nil {
			return fmt.Errorf("to: %w", err)
		}
		if filesystem.IsBlockedAbsPath(abs) {
			return fmt.Errorf("cannot map config file as route target")
		}
	case RouteRedirect:
		if route.Status != 0 && !allowedRedirectStatus[route.Status] {
			return fmt.Errorf("invalid redirect status %d", route.Status)
		}
		if dynamicTarget {
			return nil
		}
		location, err := NormalizeRedirectTarget(route.To)
		if err != nil {
			return fmt.Errorf("to: %w", err)
		}
		if isSameSiteRedirect(location) && filesystem.IsBlockedPath(location) {
			return fmt.Errorf("cannot redirect to config file")
		}
	default:
		return fmt.Errorf("invalid type %q", route.Type)
	}
	return nil
}

func isSameSiteRedirect(location string) bool {
	return !strings.Contains(location, "://") && !strings.HasPrefix(location, "//")
}

func NormalizeRedirectTarget(to string) (string, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return "", fmt.Errorf("empty redirect target")
	}
	switch {
	case strings.Contains(to, "://"):
		u, err := url.Parse(to)
		if err != nil {
			return "", fmt.Errorf("invalid redirect URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", fmt.Errorf("redirect URL scheme must be http or https")
		}
		if u.Host == "" {
			return "", fmt.Errorf("invalid redirect URL: missing host")
		}
		return u.String(), nil
	case strings.HasPrefix(to, "//"):
		u, err := url.Parse("https:" + to)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid protocol-relative redirect URL")
		}
		return to, nil
	case strings.HasPrefix(to, "/"):
		if strings.Contains(to, "..") {
			return "", fmt.Errorf("path traversal not allowed")
		}
		clean := path.Clean(to)
		if clean == "." {
			return "/", nil
		}
		return clean, nil
	default:
		if strings.Contains(to, ":") {
			return "", fmt.Errorf("unsupported redirect target")
		}
		return "/" + strings.TrimPrefix(to, "/"), nil
	}
}

func ResolveTargetPath(contentRoot, configDir, to string) (string, error) {
	to = strings.TrimSpace(to)
	if strings.HasPrefix(to, "/") {
		return resolveRootPath(contentRoot, to)
	}
	if filepath.IsAbs(to) {
		return "", fmt.Errorf("absolute filesystem paths not allowed: %s", to)
	}
	return resolveLocalPath(configDir, to)
}

func resolveRootPath(contentRoot, urlPath string) (string, error) {
	if strings.Contains(urlPath, "..") {
		return "", fmt.Errorf("path traversal not allowed")
	}
	clean := path.Clean(urlPath)
	if clean == "/" {
		return "", fmt.Errorf("invalid root path")
	}
	rel := strings.TrimPrefix(clean, "/")
	joined := filepath.Join(contentRoot, filepath.FromSlash(rel))
	return ensureUnderRoot(contentRoot, filepath.Clean(joined))
}

func ensureUnderRoot(root, abs string) (string, error) {
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes content root")
	}
	return abs, nil
}

func resolveLocalPath(base, rel string) (string, error) {
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal not allowed: %s", rel)
	}
	joined := filepath.Join(base, clean)
	relCheck, err := filepath.Rel(base, joined)
	if err != nil {
		return "", err
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes base directory")
	}
	return joined, nil
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

func NormalizeFromPath(from string) string {
	from = strings.TrimSpace(from)
	if !strings.HasPrefix(from, "/") {
		from = "/" + from
	}
	return pathClean(from)
}

func pathClean(value string) string {
	if value == "" || value == "/" {
		return "/"
	}
	value = strings.TrimSuffix(value, "/")
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func JoinURLPrefix(prefix, from string) string {
	from = NormalizeFromPath(from)
	if prefix == "" || prefix == "/" {
		return from
	}
	prefix = pathClean(prefix)
	if from == "/" {
		return prefix
	}
	return prefix + from
}
