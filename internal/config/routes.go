package config

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

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
	From     string    `yaml:"from"`
	To       string    `yaml:"to"`
	Type     RouteType `yaml:"type"`
	Status   int       `yaml:"status"`
	Download bool      `yaml:"download"`
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
	path := filepath.Join(dirPath, RoutesFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return ScopedConfig{}, err
	}

	rf, err := ParseRoutesFile(data)
	if err != nil {
		return ScopedConfig{}, fmt.Errorf("%s: %w", path, err)
	}

	sc := ScopedConfig{
		DirPath: dirPath,
		Headers: copyHeaders(rf.Headers),
		Listing: false,
	}

	if rf.Listing != nil {
		sc.Listing = *rf.Listing
	}

	for i, route := range rf.Routes {
		if err := validateRoute(route, contentRoot, dirPath); err != nil {
			return ScopedConfig{}, fmt.Errorf("%s route[%d]: %w", path, i, err)
		}
		sc.Routes = append(sc.Routes, route)
	}

	if rf.Dist != nil {
		dist := *rf.Dist
		if dist.Path == "" {
			return ScopedConfig{}, fmt.Errorf("%s: dist.path is required", path)
		}
		absDist, err := ResolveTargetPath(contentRoot, dirPath, dist.Path)
		if err != nil {
			return ScopedConfig{}, fmt.Errorf("%s dist: %w", path, err)
		}
		if st, err := os.Stat(absDist); err != nil || !st.IsDir() {
			return ScopedConfig{}, fmt.Errorf("%s dist: path %q does not exist or is not a directory", path, dist.Path)
		}
		dist.Path = absDist
		if dist.Fallback != "" {
			fb, err := ResolveTargetPath(contentRoot, absDist, dist.Fallback)
			if err != nil {
				return ScopedConfig{}, fmt.Errorf("%s dist fallback: %w", path, err)
			}
			dist.Fallback = fb
		}
		sc.Dist = &dist
	}

	return sc, nil
}

func validateRoute(r RouteDef, contentRoot, dirPath string) error {
	if strings.TrimSpace(r.From) == "" {
		return fmt.Errorf("from is required")
	}
	if strings.TrimSpace(r.To) == "" {
		return fmt.Errorf("to is required")
	}
	switch r.Type {
	case RouteRewrite, RouteFile:
		if _, err := ResolveTargetPath(contentRoot, dirPath, r.To); err != nil {
			return fmt.Errorf("to: %w", err)
		}
	case RouteRedirect:
		if r.Status != 0 && !allowedRedirectStatus[r.Status] {
			return fmt.Errorf("invalid redirect status %d", r.Status)
		}
		if _, err := NormalizeRedirectTarget(r.To); err != nil {
			return fmt.Errorf("to: %w", err)
		}
	default:
		return fmt.Errorf("invalid type %q", r.Type)
	}
	return nil
}

// NormalizeRedirectTarget validates and normalizes redirect targets.
// Supports same-site paths (/foo), relative paths (foo), and external URLs
// (http://, https://, or protocol-relative //host/path).
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

// ResolveTargetPath resolves a route target path.
// Relative paths are resolved from the config directory.
// Paths starting with "/" are resolved from the content root (ROOT_DIR).
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

func NormalizeFromPath(from string) string {
	from = strings.TrimSpace(from)
	if !strings.HasPrefix(from, "/") {
		from = "/" + from
	}
	return pathClean(from)
}

func pathClean(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
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
