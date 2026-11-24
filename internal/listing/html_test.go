package listing_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mewisme/mewroute/internal/listing"
)

func TestWriteEscapesNames(t *testing.T) {
	dir := t.TempDir()
	name := "<script>alert(1)</script>.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Skipf("filename not supported here: %v", err)
	}
	w := httptest.NewRecorder()
	if err := listing.Write(w, dir, "/"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "<script>") {
		t.Fatalf("unescaped name in listing: %s", w.Body.String())
	}
}
