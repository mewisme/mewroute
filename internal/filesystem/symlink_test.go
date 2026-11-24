package filesystem_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/filesystem"
)

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := filesystem.Resolve(root, "/link/secret.txt"); ok {
		t.Fatal("symlink escaping root must be rejected")
	}
	if _, _, ok := filesystem.Resolve(root, "/ok.txt"); !ok {
		t.Fatal("regular file must resolve")
	}
}
