package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
)

func TestParseRoutesFile(t *testing.T) {
	data := []byte(`
routes:
  - from: /hello
    to: ./hello.ps1
    type: rewrite
listing: true
headers:
  X-Test: "1"
`)
	rf, err := config.ParseRoutesFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rf.Routes) != 1 || rf.Routes[0].Type != config.RouteRewrite {
		t.Fatalf("unexpected routes: %+v", rf.Routes)
	}
	if rf.Listing == nil || !*rf.Listing {
		t.Fatal("expected listing true")
	}
	if rf.Headers["X-Test"] != "1" {
		t.Fatal("missing header")
	}
}

func TestLoadScopedConfigInvalidRedirect(t *testing.T) {
	dir := t.TempDir()
	content := `routes:
  - from: /x
    to: /y
    type: redirect
    status: 999
`
	if err := os.WriteFile(filepath.Join(dir, config.RoutesFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadScopedConfig(dir, dir)
	if err == nil {
		t.Fatal("expected error for invalid redirect status")
	}
}

func TestLoadScopedConfigRejectsConfigFileRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.RoutesFileName), []byte("routes: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.GlobalConfigFileName), []byte("git:\n  username: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		`routes:
  - from: /.routes.yml
    to: /ok
    type: redirect
    status: 302
`,
		`routes:
  - from: /leak
    to: ./.routes.yml
    type: file
`,
		`routes:
  - from: /leak
    to: /.config.yml
    type: rewrite
`,
		`routes:
  - from: /leak
    to: /.routes.yml
    type: redirect
    status: 302
`,
	}
	for i, content := range cases {
		path := filepath.Join(root, config.RoutesFileName)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := config.LoadScopedConfig(root, root); err == nil {
			t.Fatalf("case %d: expected error rejecting config file route", i)
		}
	}
}

func TestResolveTargetPathRootAnchored(t *testing.T) {
	root := t.TempDir()
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(scripts, "hello.ps1")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := config.ResolveTargetPath(root, root, "/scripts/hello.ps1")
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("got %q want %q", got, target)
	}
}

func TestResolveTargetPathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	_, err := config.ResolveTargetPath(root, root, "/../outside")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeRedirectTargetExternal(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://example.com/path", "https://example.com/path"},
		{"http://example.com", "http://example.com"},
		{"//cdn.example.com/asset.js", "//cdn.example.com/asset.js"},
		{"/local", "/local"},
		{"relative", "/relative"},
	}
	for _, tc := range cases {
		got, err := config.NormalizeRedirectTarget(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRedirectTargetRejectsUnsafe(t *testing.T) {
	cases := []string{
		"javascript:alert(1)",
		"ftp://example.com",
		"",
		"/../escape",
	}
	for _, c := range cases {
		if _, err := config.NormalizeRedirectTarget(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestJoinURLPrefix(t *testing.T) {
	got := config.JoinURLPrefix("/scripts", "/hello")
	if got != "/scripts/hello" {
		t.Fatalf("got %q", got)
	}
}
