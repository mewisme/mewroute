package markdown_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mewisme/mewroute/internal/markdown"
)

func TestFindReadmeCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.MD"), []byte("# Hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok := markdown.FindReadme(dir)
	if !ok {
		t.Fatal("expected readme")
	}
	if !strings.EqualFold(filepath.Base(path), "readme.md") {
		t.Fatalf("path=%s", path)
	}
}

func TestRenderMarkdown(t *testing.T) {
	out, err := markdown.Render([]byte("# Title\n\n**bold**"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if !strings.Contains(html, "<h1") || !strings.Contains(html, "<strong>bold</strong>") {
		t.Fatalf("html=%s", html)
	}
}
