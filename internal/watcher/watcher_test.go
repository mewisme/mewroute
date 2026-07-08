package watcher

import "testing"

func TestIsGlobalConfigFile(t *testing.T) {
	cases := []string{
		"/data/.config.yml",
		"/data/.CONFIG.YAML",
	}
	for _, c := range cases {
		if !isGlobalConfigFile(c) {
			t.Fatalf("expected global config file: %s", c)
		}
	}
	if isGlobalConfigFile("/data/readme.md") {
		t.Fatal("unexpected match")
	}
}

func TestIsRoutesFileCaseInsensitive(t *testing.T) {
	cases := []string{
		"/data/.routes.yml",
		"/data/scripts/.ROUTES.YML",
		"/data/nested/.routes.yaml",
	}
	for _, c := range cases {
		if !isRoutesFile(c) {
			t.Fatalf("expected routes file: %s", c)
		}
	}
	if isRoutesFile("/data/readme.md") {
		t.Fatal("unexpected match")
	}
}
