package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// spaHandler serves the built frontend. Hashed assets are cached forever; everything
// else revalidates so a deploy is picked up immediately. Unknown paths get index.html.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		full := filepath.Join(dir, filepath.FromSlash(p))
		if st, err := os.Stat(full); err != nil || st.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		switch {
		case strings.HasPrefix(p, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case p == "/sw.js":
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Service-Worker-Allowed", "/")
		case strings.HasSuffix(p, ".webmanifest"):
			w.Header().Set("Content-Type", "application/manifest+json")
			w.Header().Set("Cache-Control", "no-cache")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fs.ServeHTTP(w, r)
	})
}
