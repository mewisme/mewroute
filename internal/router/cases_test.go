package router_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mewisme/mewroute/internal/router"
)

// testdata/cases.json is shared with worker/test/cases.test.ts so both
// implementations are pinned to the same routing behavior.
func TestSharedCases(t *testing.T) {
	root := testRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Path     string `json:"path"`
		Kind     string `json:"kind"`
		File     string `json:"file"`
		Location string `json:"location"`
		Status   int    `json:"status"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	kinds := map[router.Kind]string{
		router.KindNotFound: "notFound", router.KindRedirect: "redirect", router.KindServeFile: "file",
		router.KindListing: "listing", router.KindReadme: "readme",
	}
	r := newResolver(t, root)
	for _, c := range cases {
		d := r.Resolve(c.Path)
		if got := kinds[d.Kind]; got != c.Kind {
			t.Errorf("%s: kind=%s want %s", c.Path, got, c.Kind)
			continue
		}
		if c.File != "" {
			rel, _ := filepath.Rel(root, d.AbsPath)
			if got := "/" + filepath.ToSlash(rel); got != c.File {
				t.Errorf("%s: file=%s want %s", c.Path, got, c.File)
			}
		}
		if c.Location != "" && (d.Location != c.Location || d.Status != c.Status) {
			t.Errorf("%s: redirect=%d %s want %d %s", c.Path, d.Status, d.Location, c.Status, c.Location)
		}
	}
}
