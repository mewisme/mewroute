package router_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/router"
)

func TestLoadTableSkipsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	validDir := filepath.Join(root, "ok")
	if err := os.MkdirAll(validDir, 0o755); err != nil {
		t.Fatal(err)
	}
	valid := `routes:
  - from: /x
    to: ./x.txt
    type: rewrite
`
	if err := os.WriteFile(filepath.Join(validDir, config.RoutesFileName), []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "x.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	badDir := filepath.Join(root, "bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, config.RoutesFileName), []byte("not: [yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Scopes()) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(table.Scopes()))
	}
}
