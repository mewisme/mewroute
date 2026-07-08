package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const GlobalConfigFileName = ".config.yml"

var gitSegmentPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type GlobalConfig struct {
	Git GitSettings `yaml:"git"`
}

type GitSettings struct {
	Username string `yaml:"username"`
	Route    string `yaml:"route"`
}

func (g *GlobalConfig) GitEnabled() bool {
	return g != nil && strings.TrimSpace(g.Git.Username) != ""
}

func (g *GlobalConfig) GitRoutePrefix() string {
	if g == nil {
		return "/git"
	}
	route := strings.TrimSpace(g.Git.Route)
	if route == "" {
		return "/git"
	}
	return NormalizeFromPath(route)
}

func LoadGlobalConfig(root string) (*GlobalConfig, error) {
	path := filepath.Join(root, GlobalConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cfg, err := ParseGlobalConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func ParseGlobalConfig(data []byte) (*GlobalConfig, error) {
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	cfg.Git.Username = strings.TrimSpace(cfg.Git.Username)
	cfg.Git.Route = strings.TrimSpace(cfg.Git.Route)
	if cfg.Git.Username != "" {
		if err := validateGitSegment(cfg.Git.Username); err != nil {
			return nil, fmt.Errorf("git.username: %w", err)
		}
	}
	if cfg.Git.Route != "" {
		normalized := NormalizeFromPath(cfg.Git.Route)
		if normalized == "/" {
			return nil, fmt.Errorf("git.route must not be /")
		}
		cfg.Git.Route = normalized
	}
	return &cfg, nil
}

// ResolveGitRedirect maps a URL path to a github.com redirect target.
// Returns ok=false when the path does not match or git is not configured.
func ResolveGitRedirect(cfg *GlobalConfig, urlPath string) (string, bool) {
	if !cfg.GitEnabled() {
		return "", false
	}

	prefix := cfg.GitRoutePrefix()
	urlPath = path.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	if urlPath == "." {
		urlPath = "/"
	}

	if urlPath != prefix && !strings.HasPrefix(urlPath, prefix+"/") {
		return "", false
	}

	suffix := strings.TrimPrefix(urlPath, prefix)
	suffix = strings.TrimPrefix(suffix, "/")
	if suffix == "" {
		return "https://github.com/" + cfg.Git.Username, true
	}

	segments := strings.Split(suffix, "/")
	if len(segments) > 2 {
		return "", false
	}
	for _, seg := range segments {
		if err := validateGitSegment(seg); err != nil {
			return "", false
		}
	}

	switch len(segments) {
	case 1:
		return "https://github.com/" + cfg.Git.Username + "/" + segments[0], true
	case 2:
		return "https://github.com/" + segments[0] + "/" + segments[1], true
	default:
		return "", false
	}
}

func validateGitSegment(seg string) error {
	if seg == "" {
		return fmt.Errorf("empty segment")
	}
	if !gitSegmentPattern.MatchString(seg) {
		return fmt.Errorf("invalid segment %q", seg)
	}
	return nil
}
