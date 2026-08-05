//go:build webui

// Package web exposes the embedded SvelteKit static build (web/build) as an
// http.Handler. Build the real SPA with `cd web && bun run build`. The `webui`
// build tag is set by the Docker image and release builds so the compiled
// binary serves the frontend; without the tag, embed_stub.go provides a 404
// fallback so `go build ./...`, `go vet`, and `go test` work in CI without the
// frontend build present.
package web

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:build
var FS embed.FS

// Handler returns an HTTP handler for the embedded SPA: real static files
// (_app, assets, ...) served directly, index.html as the SPA fallback for any
// other path, and a hard 404 for anything under /api/ or /webhooks/ that
// reaches it.
func Handler() http.Handler {
	sub, err := fs.Sub(FS, "build")
	if err != nil {
		// Unreachable in practice: the embed guarantees build exists.
		panic(err)
	}
	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		// Defense-in-depth headers for the admin dashboard (the bearer token
		// lives in localStorage). CSP uses a per-request nonce for the one
		// inline SvelteKit bootstrap script in index.html.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// Real API and webhook misses must never fall back to the SPA shell.
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/webhooks/") {
			http.NotFound(w, r)
			return
		}

		// Serve real static files (/_app/..., /assets/..., etc.) directly.
		if f, err := sub.Open(strings.TrimPrefix(p, "/")); err == nil {
			if info, ierr := f.Stat(); ierr == nil && !info.IsDir() {
				f.Close()
				files.ServeHTTP(w, r)
				return
			}
			f.Close()
		}

		// SPA fallback for every other path.
		serveIndex(sub, w, r)
	})
}

// serveIndex serves index.html with a CSP nonce injected into the inline
// SvelteKit bootstrap script.
func serveIndex(sub fs.FS, w http.ResponseWriter, r *http.Request) {
	raw, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	nonceHex := hex.EncodeToString(nonce)

	patched := bytes.Replace(raw, []byte("<script>"), []byte(`<script nonce="`+nonceHex+`">`), 1)
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'nonce-"+nonceHex+"'; "+
			"style-src 'self' 'unsafe-inline'; img-src 'self' data:; "+
			"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(patched)
}
