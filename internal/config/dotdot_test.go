package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
)

func TestResolveTargetPathDotDot(t *testing.T) {
	root := t.TempDir()
	if _, err := config.ResolveTargetPath(root, root, "/files/a..b.txt"); err != nil {
		t.Fatalf("a..b.txt is a valid name: %v", err)
	}
	if _, err := config.ResolveTargetPath(root, root, "/files/../x"); err == nil {
		t.Fatal("traversal must be rejected")
	}
}

func TestResolveTargetPathSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := config.ResolveTargetPath(root, root, "/link/secret.txt"); err == nil {
		t.Fatal("symlink escape must be rejected")
	}
}
