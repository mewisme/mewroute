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
	stat, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if s.Cache != nil {
		s.Cache.Set(absPath, MetaFromStat(stat))
	}
	return stat, nil
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
	if contentType, ok := mimeOverrides[ext]; ok {
		return contentType
	}
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func ApplyHeaders(w http.ResponseWriter, headers map[string]string) {
	for key, value := range headers {
		w.Header().Set(key, value)
	}
}

func ServeFile(w http.ResponseWriter, r *http.Request, absPath string, download bool) error {
	stat, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return fmt.Errorf("is directory")
	}

	file, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if download {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(absPath)))
	}
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", ContentType(absPath))
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, stat.ModTime().Unix(), stat.Size()))
	http.ServeContent(w, r, filepath.Base(absPath), stat.ModTime(), file)
	return nil
}
