package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// newSPAHandler serves an embedded SvelteKit static build (BACK-09's
// standalone frontend): a real file on disk (JS/CSS/images under
// _app/...) is served as-is; anything else — an unknown path — falls
// back to index.html so the client-side router can resolve deep links
// like /import that only exist in the SPA's own routing table, not as a
// file in distFS.
func newSPAHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "" || clean == "." {
			clean = "index.html"
		}
		if info, err := fs.Stat(distFS, clean); err != nil || info.IsDir() {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// standaloneSyncRejectedHandler is what POST /sync becomes in standalone
// mode: a clear, explicit rejection (BACK-09's acceptance criteria) —
// there is no ledger-service to push to, ever, in this mode.
func standaloneSyncRejectedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"standalone mode: there is no ledger-service to sync to"}`))
}

// noFrontendEmbeddedHandler is what standalone mode serves instead when
// nobody ran the real frontend build before compiling this binary (see
// internal/webui's doc comment) — a clear explanation beats a bare 404
// on every page.
func noFrontendEmbeddedHandler() http.Handler {
	const body = `<!DOCTYPE html><html><body style="font-family:sans-serif;max-width:40rem;margin:3rem auto;line-height:1.5">
<h1>financial-tracker (standalone)</h1>
<p>The API is running, but no frontend was baked into this binary.</p>
<p>Run <code>make web-build-standalone</code> before <code>go build</code> — see the root README's
"Running locally (standalone)" section.</p>
<p>The API itself is reachable normally, e.g. <code>GET /config</code>.</p>
</body></html>`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
}
