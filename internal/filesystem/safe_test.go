package filesystem_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mewisme/mewroute/internal/filesystem"
)

func TestResolveStaticPath(t *testing.T) {
	root := t.TempDir()
	abs, rel, ok := filesystem.Resolve(root, "/tools/linux/setup.sh")
	if !ok {
		t.Fatal("expected ok")
	}
	if filepath.ToSlash(rel) != "tools/linux/setup.sh" {
		t.Fatalf("rel=%q", rel)
	}
	if abs == "" {
		t.Fatal("empty abs")
	}
}

func TestResolveStaysWithinRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"/../etc/passwd",
		"/foo/../../bar",
		"/%2e%2e/%2e%2e/x",
		"/./safe.txt",
		"/safe.txt",
	}
	for _, p := range paths {
		abs, _, ok := filesystem.Resolve(root, p)
		if !ok {
			continue
		}
		rel, err := filepath.Rel(filepath.Clean(root), abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("escaped root for %s -> %s (rel=%s)", p, abs, rel)
		}
	}
}

func TestBlockedRoutesConfig(t *testing.T) {
	cases := []string{
		"/.routes.yml",
		"/scripts/.routes.yml",
		"/foo/.routes.yaml",
		"/.config.yml",
		"/scripts/.config.yaml",
		"/.ROUTES.YML",
	}
	for _, c := range cases {
		if !filesystem.IsBlockedPath(c) {
			t.Fatalf("expected blocked: %s", c)
		}
	}
	if !filesystem.IsBlockedFileName(".routes.yml") {
		t.Fatal("expected blocked filename")
	}
	if !filesystem.IsBlockedAbsPath(filepath.Join("data", "scripts", ".routes.yml")) {
		t.Fatal("expected blocked abs path")
	}
	if filesystem.IsBlockedFileName("hello.ps1") {
		t.Fatal("hello.ps1 should not be blocked")
	}
}

func TestStatCacheInvalidate(t *testing.T) {
	c := filesystem.NewStatCache()
	base := filepath.Join("data", "a")
	child := filepath.Join(base, "b")
	c.Set(base, filesystem.FileMeta{})
	c.Set(child, filesystem.FileMeta{})
	c.InvalidatePrefix(base)
	if _, ok := c.Get(base); ok {
		t.Fatal("expected invalidated")
	}
	if _, ok := c.Get(child); ok {
		t.Fatal("expected child invalidated")
	}
}
