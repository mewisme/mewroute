package filesystem

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

const (
	ConfigFileName        = ".routes.yml"
	ConfigFileName2       = ".routes.yaml"
	GlobalConfigFileName  = ".config.yml"
	GlobalConfigFileName2 = ".config.yaml"
)

// IsBlockedFileName reports whether base is a mewroute config file that must
// never be served or mapped as a route (.routes.yml in any folder, .config.yml).
func IsBlockedFileName(base string) bool {
	switch strings.ToLower(base) {
	case ConfigFileName, ConfigFileName2, GlobalConfigFileName, GlobalConfigFileName2:
		return true
	default:
		return false
	}
}

// IsBlockedAbsPath reports whether an absolute filesystem path points at a
// blocked config file (by basename).
func IsBlockedAbsPath(abs string) bool {
	return IsBlockedFileName(filepath.Base(abs))
}

func IsBlockedPath(urlPath string) bool {
	clean := path.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	parts := strings.Split(strings.Trim(clean, "/"), "/")
	for _, p := range parts {
		if IsBlockedFileName(p) {
			return true
		}
	}
	return false
}

func Resolve(root, urlPath string) (absPath, relPath string, ok bool) {
	if IsBlockedPath(urlPath) {
		return "", "", false
	}

	if strings.Contains(urlPath, "\x00") {
		return "", "", false
	}

	cleanURL := path.Clean("/" + strings.TrimPrefix(urlPath, "/"))
	if cleanURL == "/" {
		relPath = "."
	} else {
		relPath = strings.TrimPrefix(cleanURL, "/")
		relPath = filepath.FromSlash(relPath)
	}

	absPath = filepath.Join(root, relPath)
	absPath = filepath.Clean(absPath)

	rootClean := filepath.Clean(root)
	rel, err := filepath.Rel(rootClean, absPath)
	if err != nil {
		return "", "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", false
	}

	if strings.Contains(urlPath, "%") {
		decoded, err := url.PathUnescape(urlPath)
		if err == nil && decoded != urlPath {
			return Resolve(root, decoded)
		}
	}

	if !WithinRoot(root, absPath) {
		return "", "", false
	}

	return absPath, relPath, true
}

// WithinRoot reports whether abs, after resolving symlinks, stays inside root.
// The deepest existing ancestor is resolved, so a missing leaf behind an
// escaping symlink is rejected too.
// ponytail: one EvalSymlinks per lookup; cache by mtime if this shows up in profiles.
func WithinRoot(root, abs string) bool {
	existing, tail := abs, ""
	var realAbs string
	for {
		real, err := filepath.EvalSymlinks(existing)
		if err == nil {
			realAbs = filepath.Join(real, tail)
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return false
		}
		tail = filepath.Join(filepath.Base(existing), tail)
		existing = parent
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, realAbs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func URLPathFromRel(rel string) string {
	if rel == "." || rel == "" {
		return "/"
	}
	return "/" + filepath.ToSlash(filepath.Clean(rel))
}
