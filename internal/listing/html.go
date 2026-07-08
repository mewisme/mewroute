package listing

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func Write(w http.ResponseWriter, dirPath string, urlPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	type item struct {
		name  string
		isDir bool
		size  int64
		mod   time.Time
	}
	var items []item
	for _, e := range entries {
		if e.Name() == ".routes.yml" || e.Name() == ".routes.yaml" ||
			e.Name() == ".config.yml" || e.Name() == ".config.yaml" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{
			name:  e.Name(),
			isDir: e.IsDir(),
			size:  info.Size(),
			mod:   info.ModTime(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].isDir != items[j].isDir {
			return items[i].isDir
		}
		return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
	})

	base := strings.TrimSuffix(urlPath, "/")
	if base == "" {
		base = "/"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err = io.WriteString(w, "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>Index of "+html.EscapeString(base)+"</title>")
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "<style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse;width:100%}th,td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid #ddd}th{background:#f5f5f5}a{text-decoration:none;color:#0366d6}</style></head><body>")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "<h1>Index of %s</h1><table><thead><tr><th>Name</th><th>Size</th><th>Modified</th></tr></thead><tbody>", html.EscapeString(base))
	if err != nil {
		return err
	}

	if base != "/" {
		parent := base
		if idx := strings.LastIndex(strings.TrimSuffix(parent, "/"), "/"); idx >= 0 {
			parent = parent[:idx+1]
			if parent == "" {
				parent = "/"
			}
		} else {
			parent = "/"
		}
		_, err = fmt.Fprintf(w, "<tr><td><a href=\"%s\">../</a></td><td>-</td><td>-</td></tr>", html.EscapeString(parent))
		if err != nil {
			return err
		}
	}

	for _, it := range items {
		href := joinURL(base, it.name, it.isDir)
		size := formatSize(it.size, it.isDir)
		mod := it.mod.UTC().Format(time.RFC1123)
		_, err = fmt.Fprintf(w, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>",
			html.EscapeString(href), html.EscapeString(it.name+dirSuffix(it.isDir)), size, html.EscapeString(mod))
		if err != nil {
			return err
		}
	}

	_, err = io.WriteString(w, "</tbody></table></body></html>")
	return err
}

func joinURL(base, name string, isDir bool) string {
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	href := base + filepath.ToSlash(name)
	if isDir && !strings.HasSuffix(href, "/") {
		href += "/"
	}
	return href
}

func dirSuffix(isDir bool) string {
	if isDir {
		return "/"
	}
	return ""
}

func formatSize(size int64, isDir bool) string {
	if isDir {
		return "-"
	}
	return fmt.Sprintf("%d", size)
}
