package dashboard

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/metabbe3/Kyoci-Agent/web"
)

// SPAHandler serves the embedded dashboard SPA. Any unknown path falls back
// to index.html so client-side routing (react-router) works on hard reload.
// API paths (/api/*, /health) must be registered before this handler on the
// parent mux — they never reach here.
func SPAHandler() http.HandlerFunc {
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// distFS will be non-nil because web/embed.go embeds `all:dist`. If
		// this ever fails it's a build-time issue, not a runtime one.
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "SPA assets not embedded — run `npm run build` in web/", http.StatusNotFound)
		}
	}
	fileServer := http.FileServer(http.FS(distFS))

	return func(w http.ResponseWriter, r *http.Request) {
		// Trim leading slash; fs.Sub paths are relative.
		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

		// If the path maps to a real file, serve it.
		if clean != "" {
			if f, err := distFS.Open(clean); err == nil {
				stat, _ := f.Stat()
				f.Close()
				if !stat.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}

		// Otherwise fall back to index.html for client-side routing.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	}
}
