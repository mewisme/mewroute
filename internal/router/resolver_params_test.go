package router_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/router"
)

func TestNamedParamRedirectWithGlobalUsername(t *testing.T) {
	root := t.TempDir()
	routes := `routes:
  - from: /{repo}.{ext}
    to: https://raw.githubusercontent.com/{git.username}/{repo}/refs/heads/main/install.{ext}
    type: redirect
    params:
      ext:
        allow: [sh, ps1]
`
	if err := os.WriteFile(filepath.Join(root, config.RoutesFileName), []byte(routes), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table, &config.GlobalConfig{Git: config.GitSettings{Username: "mewisme"}})
	resolver := router.NewResolver(root, cache)

	decision := resolver.Resolve("/agentrule.ps1")
	want := "https://raw.githubusercontent.com/mewisme/agentrule/refs/heads/main/install.ps1"
	if decision.Kind != router.KindRedirect || decision.Location != want {
		t.Fatalf("unexpected: %+v", decision)
	}
}

func TestNamedParamAllowListRejectsValue(t *testing.T) {
	root := t.TempDir()
	routes := `routes:
  - from: /{repo}.{ext}
    to: https://example.com/{repo}.{ext}
    type: redirect
    params:
      ext:
        allow: [sh, ps1]
`
	if err := os.WriteFile(filepath.Join(root, config.RoutesFileName), []byte(routes), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table, &config.GlobalConfig{})
	resolver := router.NewResolver(root, cache)
	if decision := resolver.Resolve("/agentrule.exe"); decision.Kind != router.KindNotFound {
		t.Fatalf("expected not found, got %+v", decision)
	}
}

func TestExactRouteWinsOverParamRoute(t *testing.T) {
	root := t.TempDir()
	routes := `routes:
  - from: /fixed.ps1
    to: https://example.com/exact
    type: redirect
  - from: /{repo}.{ext}
    to: https://example.com/{repo}.{ext}
    type: redirect
`
	if err := os.WriteFile(filepath.Join(root, config.RoutesFileName), []byte(routes), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table, &config.GlobalConfig{})
	resolver := router.NewResolver(root, cache)
	decision := resolver.Resolve("/fixed.ps1")
	if decision.Location != "https://example.com/exact" {
		t.Fatalf("unexpected: %+v", decision)
	}
}
