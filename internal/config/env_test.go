package config_test

import (
	"os"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
)

func TestLoadEnvDefaults(t *testing.T) {
	t.Setenv("ROOT_DIR", "")
	t.Setenv("PORT", "")
	cfg, err := config.LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RootDir != "/data" || cfg.Port != 8080 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadEnvInvalidPort(t *testing.T) {
	t.Setenv("PORT", "bad")
	_, err := config.LoadEnv()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadEnvCustom(t *testing.T) {
	t.Setenv("ROOT_DIR", os.TempDir())
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	cfg, err := config.LoadEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9090 || cfg.LogLevel != "debug" {
		t.Fatalf("unexpected: %+v", cfg)
	}
}
