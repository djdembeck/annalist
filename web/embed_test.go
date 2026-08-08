//go:build webui

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestServeIndex covers the SPA fallback helper directly, including the
// missing-index 404 path. Runs only under `-tags webui`, where the helper
// exists.
func TestServeIndex(t *testing.T) {
	withIndex := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html>hello")},
	}
	w := httptest.NewRecorder()
	serveIndex(withIndex, w, httptest.NewRequest(http.MethodGet, "http://test/whatever", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hello") {
		t.Errorf("body = %q, want index content", w.Body.String())
	}

	missing := fstest.MapFS{}
	w2 := httptest.NewRecorder()
	serveIndex(missing, w2, httptest.NewRequest(http.MethodGet, "http://test/whatever", nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when index missing", w2.Code)
	}
}

// TestHandler exercises the embedded SPA handler end to end: unknown paths get
// the index fallback, /api/* and /webhooks/* are hard 404s. The webui tag
// implies the real web/build output is present (produced by `bun run build`
// or the Docker frontend stage).
func TestHandler(t *testing.T) {
	h := Handler()

	t.Run("unknown path serves index fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/some/spa/route", nil)
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "<html") {
			t.Errorf("body does not look like index.html: %s", w.Body.String())
		}
	})

	t.Run("missing static asset is a hard 404, not the html shell", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/fonts/missing.woff2", nil)
		req.Header.Set("Accept", "application/font-woff2;q=1.0,font/woff2;q=1.0,*/*;q=0.1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body starts %q (must not be the HTML shell)",
				w.Code, w.Body.String()[:min(len(w.Body.String()), 32)])
		}
	})

	t.Run("asset-shaped path serves 404 even with html accept", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/_app/immutable/nope.js", nil)
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 for missing asset path", w.Code)
		}
	})

	t.Run("unknown api path is not spa fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/api/does-not-exist", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})

	t.Run("unknown webhooks path is not spa fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://test/webhooks/nope", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", w.Code)
		}
	})
}
