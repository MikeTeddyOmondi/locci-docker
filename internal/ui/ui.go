// Package ui serves the SvelteKit single-page app embedded in the binary.
//
// `just web` builds web/ into internal/ui/dist before `go build`.
package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const notBuilt = `<!doctype html><title>locci</title>
<body style="font-family:system-ui;padding:3rem">
<h1>locci</h1><p>The web UI isn't built into this binary.
Run <code>just web</code> and rebuild, or use the CLI / API.</p></body>`

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// InlineScriptHashes returns " 'sha256-...'" for each inline script in
// index.html (SvelteKit's bootstrap), for the Content-Security-Policy.
func InlineScriptHashes() string {
	index, err := fs.ReadFile(dist, "dist/index.html")
	if err != nil {
		return ""
	}
	var out strings.Builder
	for _, m := range inlineScript.FindAllSubmatch(index, -1) {
		sum := sha256.Sum256(m[1])
		out.WriteString(" 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'")
	}
	return out.String()
}

// Built reports whether the web UI was built into this binary.
func Built() bool {
	_, err := fs.Stat(dist, "dist/index.html")
	return err == nil
}

// Handler serves static files and falls back to index.html for client-side routes.
func Handler() http.Handler {
	files, _ := fs.Sub(dist, "dist")
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		index = []byte(notBuilt)
	}
	static := http.FileServer(http.FS(files))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if f, err := files.Open(name); err == nil {
				st, _ := f.Stat()
				f.Close()
				if st != nil && !st.IsDir() {
					if strings.HasPrefix(name, "_app/immutable/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					static.ServeHTTP(w, r)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		// Explicit: under gin's NoRoute the status is preset to 404, and
		// client-side routes such as /login are real pages.
		w.WriteHeader(http.StatusOK)
		w.Write(index)
	})
}
