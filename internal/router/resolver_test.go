package router_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/router"
)

func testRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(wd, "..", "..", "testdata")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("testdata missing: %v", err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func newResolver(t *testing.T, root string) *router.Resolver {
	t.Helper()
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table)
	return router.NewResolver(root, cache)
}

func TestRewriteRoute(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/scripts/hello")
	if d.Kind != router.KindServeFile {
		t.Fatalf("kind=%v", d.Kind)
	}
	if filepath.Base(d.AbsPath) != "hello.ps1" {
		t.Fatalf("path=%s", d.AbsPath)
	}
}

func TestRedirectRoute(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/scripts/old")
	if d.Kind != router.KindRedirect || d.Location != "/scripts/hello" {
		t.Fatalf("unexpected: %+v", d)
	}
	if d.Status != 302 {
		t.Fatalf("status=%d", d.Status)
	}
}

func TestDistSPA(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/app/users/1")
	if d.Kind != router.KindServeFile {
		t.Fatalf("kind=%v", d.Kind)
	}
	if filepath.Base(d.AbsPath) != "index.html" {
		t.Fatalf("path=%s", d.AbsPath)
	}
}

func TestDistAsset(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/app/assets/main.js")
	if d.Kind != router.KindServeFile || filepath.Base(d.AbsPath) != "main.js" {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestStaticFileDefault(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/tools/linux/setup.sh")
	if d.Kind != router.KindServeFile {
		t.Fatalf("kind=%v", d.Kind)
	}
}

func TestNestedScopeRoute(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/nested/child/secret")
	if d.Kind != router.KindServeFile || filepath.Base(d.AbsPath) != "nested.txt" {
		t.Fatalf("unexpected: %+v", d)
	}
}

func TestBlockedConfigPath(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/scripts/.routes.yml")
	if d.Kind != router.KindNotFound {
		t.Fatalf("expected not found")
	}
}

func TestWildcardRoute(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/scripts/docs/readme")
	if d.Kind != router.KindServeFile {
		t.Fatalf("kind=%v", d.Kind)
	}
}

func TestExternalRedirectRoute(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/scripts/external")
	if d.Kind != router.KindRedirect || d.Location != "https://example.com/docs" {
		t.Fatalf("unexpected: %+v", d)
	}
	if d.Status != 301 {
		t.Fatalf("status=%d", d.Status)
	}
}

func TestListingScope(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/scripts")
	if d.Kind != router.KindListing {
		t.Fatalf("expected listing, got %v", d.Kind)
	}
}

func TestRootReadme(t *testing.T) {
	r := newResolver(t, testRoot(t))
	d := r.Resolve("/")
	if d.Kind != router.KindReadme {
		t.Fatalf("expected readme render at root, got %v", d.Kind)
	}
	if !strings.EqualFold(filepath.Base(d.AbsPath), "readme.md") {
		t.Fatalf("path=%s", d.AbsPath)
	}
}

func TestRootAnchoredTarget(t *testing.T) {
	root := t.TempDir()
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "hello.ps1"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := `routes:
  - from: /fetch
    to: /scripts/hello.ps1
    type: file
`
	if err := os.WriteFile(filepath.Join(root, config.RoutesFileName), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table)
	resolver := router.NewResolver(root, cache)
	d := resolver.Resolve("/fetch")
	if d.Kind != router.KindServeFile || filepath.Base(d.AbsPath) != "hello.ps1" {
		t.Fatalf("unexpected: %+v", d)
	}
}
