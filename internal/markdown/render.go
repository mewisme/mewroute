package markdown

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// FindReadme returns the path to a readme markdown file in dir, if present.
// Matches readme.md case-insensitively (README.md, readme.MD, etc.).
func FindReadme(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(e.Name(), "readme.md") {
			return filepath.Join(dir, e.Name()), true
		}
	}
	return "", false
}

func Render(source []byte) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
	)
	var buf bytes.Buffer
	if err := md.Convert(source, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Write(w io.Writer, absPath, urlPath string) error {
	source, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	body, err := Render(source)
	if err != nil {
		return err
	}

	title := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	if urlPath != "" && urlPath != "/" {
		title = strings.Trim(urlPath, "/")
	}

	_, err = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;line-height:1.6;max-width:48rem;margin:2rem auto;padding:0 1rem;color:#1f2328}
h1,h2,h3,h4{border-bottom:1px solid #d8dee4;padding-bottom:.3em}
a{color:#0969da;text-decoration:none}
a:hover{text-decoration:underline}
code,pre{font-family:ui-monospace,monospace;font-size:.9em}
pre{background:#f6f8fa;padding:1rem;overflow:auto;border-radius:6px}
code{background:#f6f8fa;padding:.2em .4em;border-radius:4px}
pre code{background:none;padding:0}
table{border-collapse:collapse;width:100%%;margin:1rem 0}
th,td{border:1px solid #d8dee4;padding:.5rem .75rem}
blockquote{color:#59636e;border-left:.25em solid #d8dee4;margin:0;padding-left:1rem}
img{max-width:100%%}
</style>
</head>
<body>
`, html.EscapeString(title))
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n</body>\n</html>\n")
	return err
}
