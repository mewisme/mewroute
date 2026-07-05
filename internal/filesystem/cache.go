package filesystem

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileMeta struct {
	ModTime time.Time
	Size    int64
	IsDir   bool
}

type StatCache struct {
	mu    sync.RWMutex
	items map[string]FileMeta
}

func NewStatCache() *StatCache {
	return &StatCache{items: make(map[string]FileMeta)}
}

func (c *StatCache) Get(absPath string) (FileMeta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.items[absPath]
	return m, ok
}

func (c *StatCache) Set(absPath string, meta FileMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[absPath] = meta
}

func (c *StatCache) InvalidatePrefix(absPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	absPath = filepath.Clean(absPath)
	for k := range c.items {
		if k == absPath || hasPathPrefix(k, absPath) {
			delete(c.items, k)
		}
	}
}

func (c *StatCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]FileMeta)
}

func hasPathPrefix(path, prefix string) bool {
	if prefix == "" {
		return false
	}
	if path == prefix {
		return true
	}
	return len(path) > len(prefix) && path[:len(prefix)] == prefix && path[len(prefix)] == filepath.Separator
}

func MetaFromStat(st os.FileInfo) FileMeta {
	return FileMeta{
		ModTime: st.ModTime(),
		Size:    st.Size(),
		IsDir:   st.IsDir(),
	}
}
