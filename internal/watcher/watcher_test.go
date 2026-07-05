package watcher

import "testing"

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
