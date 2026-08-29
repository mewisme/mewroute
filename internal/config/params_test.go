package config_test

import (
	"testing"

	"github.com/mewisme/mewroute/internal/config"
)

func TestRouteParamNames(t *testing.T) {
	names, err := config.RouteParamNames("/{repo}.{ext}")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "repo" || names[1] != "ext" {
		t.Fatalf("unexpected names: %#v", names)
	}
}

func TestValidateRouteParamsRejectsUnknownTargetParam(t *testing.T) {
	route := config.RouteDef{From: "/{repo}", To: "https://example.com/{missing}", Type: config.RouteRedirect}
	if err := config.ValidateRouteParams(route); err == nil {
		t.Fatal("expected error")
	}
}

func TestInterpolateTargetWithGlobalGitUsername(t *testing.T) {
	target, err := config.InterpolateTarget(
		"https://raw.githubusercontent.com/{git.username}/{repo}/refs/heads/main/install.{ext}",
		map[string]string{"repo": "agentrule", "ext": "ps1"},
		&config.GlobalConfig{Git: config.GitSettings{Username: "mewisme"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://raw.githubusercontent.com/mewisme/agentrule/refs/heads/main/install.ps1"
	if target != want {
		t.Fatalf("got %q want %q", target, want)
	}
}
