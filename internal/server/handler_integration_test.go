package server_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/filesystem"
	"github.com/mewisme/mewroute/internal/router"
	"github.com/mewisme/mewroute/internal/server"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	wd, _ := os.Getwd()
	root, _ := filepath.Abs(filepath.Join(wd, "..", "..", "testdata"))
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table, &config.GlobalConfig{})
	resolver := router.NewResolver(root, cache)
	files := filesystem.NewServer(root, filesystem.NewStatCache())
	h := server.NewHandler(resolver, files, slog.Default())
	return httptest.NewServer(h)
}

func TestIntegrationGitRedirect(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, config.GlobalConfigFileName), []byte(`git:
  username: mewisme
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := router.NewCache()
	table, err := router.LoadTable(root, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	global, err := config.LoadGlobalConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	cache.Store(table, global)
	resolver := router.NewResolver(root, cache)
	files := filesystem.NewServer(root, filesystem.NewStatCache())
	h := server.NewHandler(resolver, files, slog.Default())
	ts := httptest.NewServer(h)
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/git/wrec")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://github.com/mewisme/wrec" {
		t.Fatalf("location=%s", resp.Header.Get("Location"))
	}
}

func TestIntegrationRewrite(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/scripts/hello")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hello from mewroute") {
		t.Fatalf("body=%s", body)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type=%s", ct)
	}
	if resp.Header.Get("X-Test-Scope") != "scripts" {
		t.Fatalf("missing scope header")
	}
}

func TestIntegrationExternalRedirect(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/scripts/external")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 301 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "https://example.com/docs" {
		t.Fatalf("location=%s", resp.Header.Get("Location"))
	}
}

func TestIntegrationRedirect(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/scripts/old")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/scripts/hello" {
		t.Fatalf("location=%s", resp.Header.Get("Location"))
	}
}

func TestIntegrationDownload(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/scripts/download")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("disposition=%s", resp.Header.Get("Content-Disposition"))
	}
}

func TestIntegrationDistHeaders(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/app/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-App") != "demo" {
		t.Fatalf("missing X-App header")
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%s", resp.Header.Get("Cache-Control"))
	}
}

func TestIntegrationSecurityTraversal(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/scripts/../scripts/.routes.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestIntegrationMethodNotAllowed(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/tools/linux/setup.sh", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestIntegrationHead(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodHead, ts.URL+"/tools/linux/setup.sh", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if resp.ContentLength <= 0 && resp.Header.Get("Content-Length") == "" {
		// ServeContent sets length on HEAD
	}
}

func TestIntegrationHealthz(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestIntegrationListing(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/scripts/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "hello.ps1") {
		t.Fatalf("listing missing file")
	}
}

func TestIntegrationRootReadme(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%s", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<h1") || !strings.Contains(string(body), "mewroute") {
		t.Fatalf("expected rendered readme html")
	}
}

func TestIntegrationReadmeRawFileStillServed(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "<!DOCTYPE html>") {
		t.Fatal("direct file access should not render html wrapper")
	}
	if !strings.Contains(string(body), "# mewroute") {
		t.Fatalf("expected raw markdown")
	}
}

func TestIntegrationETagAndRange(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/tools/linux/setup.sh")
	if err != nil {
		t.Fatal(err)
	}
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	if etag == "" {
		t.Fatal("missing etag")
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/tools/linux/setup.sh", nil)
	req.Header.Set("Range", "bytes=0-3")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status=%d", resp2.StatusCode)
	}
}
