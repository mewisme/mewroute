package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
)

func TestParseGlobalConfig(t *testing.T) {
	data := []byte(`
git:
  username: mewisme
  route: /git
`)
	cfg, err := config.ParseGlobalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Git.Username != "mewisme" {
		t.Fatalf("username=%q", cfg.Git.Username)
	}
	if cfg.Git.Route != "/git" {
		t.Fatalf("route=%q", cfg.Git.Route)
	}
}

func TestParseGlobalConfigDefaultRoute(t *testing.T) {
	data := []byte(`git:
  username: mewisme
`)
	cfg, err := config.ParseGlobalConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitRoutePrefix() != "/git" {
		t.Fatalf("prefix=%q", cfg.GitRoutePrefix())
	}
}

func TestLoadGlobalConfigMissingFile(t *testing.T) {
	cfg, err := config.LoadGlobalConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitEnabled() {
		t.Fatal("expected git disabled")
	}
}

func TestResolveGitRedirect(t *testing.T) {
	cfg := &config.GlobalConfig{Git: config.GitSettings{Username: "mewisme"}}

	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"/git", "https://github.com/mewisme", true},
		{"/git/wrec", "https://github.com/mewisme/wrec", true},
		{"/git/mewisme/wrec", "https://github.com/mewisme/wrec", true},
		{"/git/foo/bar/baz", "", false},
		{"/other/wrec", "", false},
		{"/git/../escape", "", false},
	}
	for _, tc := range cases {
		got, ok := config.ResolveGitRedirect(cfg, tc.path)
		if ok != tc.ok {
			t.Fatalf("%q: ok=%v want %v", tc.path, ok, tc.ok)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.path, got, tc.want)
		}
	}
}

func TestResolveGitRedirectCustomPrefix(t *testing.T) {
	cfg := &config.GlobalConfig{Git: config.GitSettings{Username: "mewisme", Route: "/gh"}}
	got, ok := config.ResolveGitRedirect(cfg, "/gh/myrepo")
	if !ok || got != "https://github.com/mewisme/myrepo" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveGitRedirectDisabled(t *testing.T) {
	cfg := &config.GlobalConfig{}
	if _, ok := config.ResolveGitRedirect(cfg, "/git/wrec"); ok {
		t.Fatal("expected disabled")
	}
}

func TestParseGlobalConfigInvalidUsername(t *testing.T) {
	_, err := config.ParseGlobalConfig([]byte(`git:
  username: "bad/user"
`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadGlobalConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	content := `git:
  username: mewisme
`
	if err := os.WriteFile(filepath.Join(dir, config.GlobalConfigFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadGlobalConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GitEnabled() {
		t.Fatal("expected enabled")
	}
}
