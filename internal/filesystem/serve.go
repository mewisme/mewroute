package filesystem

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var mimeOverrides = map[string]string{
	".ps1":  "text/plain; charset=utf-8",
	".sh":   "text/plain; charset=utf-8",
	".bat":  "text/plain; charset=utf-8",
	".cmd":  "text/plain; charset=utf-8",
	".exe":  "application/octet-stream",
	".py":   "text/plain; charset=utf-8",
	".js":   "text/plain; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".yml":  "text/yaml; charset=utf-8",
	".yaml": "text/yaml; charset=utf-8",
}

type Server struct {
	Root  string
	Cache *StatCache
}

func NewServer(root string, cache *StatCache) *Server {
	return &Server{Root: root, Cache: cache}
}

func (s *Server) Stat(absPath string) (os.FileInfo, error) {
	if s.Cache != nil {
		if meta, ok := s.Cache.Get(absPath); ok {
			return &cachedInfo{path: absPath, meta: meta}, nil
		}
	}
	st, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if s.Cache != nil {
		s.Cache.Set(absPath, MetaFromStat(st))
	}
	return st, nil
}

type cachedInfo struct {
	path string
	meta FileMeta
}

func (c *cachedInfo) Name() string       { return filepath.Base(c.path) }
func (c *cachedInfo) Size() int64        { return c.meta.Size }
func (c *cachedInfo) Mode() os.FileMode  { return 0 }
func (c *cachedInfo) ModTime() time.Time { return c.meta.ModTime }
func (c *cachedInfo) IsDir() bool        { return c.meta.IsDir }
func (c *cachedInfo) Sys() any           { return nil }

func ContentType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ct, ok := mimeOverrides[ext]; ok {
		return ct
	}
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}

func ApplyHeaders(w http.ResponseWriter, headers map[string]string) {
	for k, v := range headers {
		w.Header().Set(k, v)
	}
}

func ServeFile(w http.ResponseWriter, r *http.Request, absPath string, download bool) error {
	st, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("is directory")
	}

	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if download {
		name := filepath.Base(absPath)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	}

	etag := fmt.Sprintf(`"%x-%x"`, st.ModTime().Unix(), st.Size())
	w.Header().Set("ETag", etag)

	http.ServeContent(w, r, filepath.Base(absPath), st.ModTime(), f)
	return nil
}
