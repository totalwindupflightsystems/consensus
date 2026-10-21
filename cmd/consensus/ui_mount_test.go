// Package main: focused tests for the web admin UI mount (QA-CONSENSUS-29).
//
// Existing internal/web tests exercise only the INNER UI mux at bare paths
// (/dashboard etc.) and cannot see how cmd/consensus/main.go mounts it into
// the chi router. These tests drive the production mount shape:
//
//	chi route -> mountUI(r, webUI.Handler()) -> http.StripPrefix -> inner mux
//
// so a mount regression (lost wildcard, lost redirect, wrong prefix strip)
// fails here instead of only on a live serve.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/wojons/consensus/internal/web"
)

// newMountedUI returns an httptest-ready handler wired exactly like
// runServer wires the admin UI into the API router.
func newMountedUI() http.Handler {
	r := chi.NewRouter()
	webUI := web.NewServer("http://localhost:8090")
	mountUI(r, webUI.Handler())
	return r
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// TestMountUI_SubpagesUnderWildcard covers acceptance (1): every UI subpage
// answers 200 with an HTML content type through the production mount.
func TestMountUI_SubpagesUnderWildcard(t *testing.T) {
	h := newMountedUI()
	for _, path := range []string{"/ui/", "/ui/dashboard", "/ui/sessions", "/ui/memory", "/ui/health"} {
		t.Run(path, func(t *testing.T) {
			w := get(t, h, path)
			if w.Code != http.StatusOK {
				t.Fatalf("GET %s: expected 200, got %d", path, w.Code)
			}
			if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
				t.Errorf("GET %s: expected text/html content type, got %q", path, ct)
			}
		})
	}
}

// TestMountUI_IndexHasNoRootAbsoluteNavHref covers acceptance (2), first half:
// the index served at /ui/ must not link navigation outside the /ui prefix.
func TestMountUI_IndexHasNoRootAbsoluteNavHref(t *testing.T) {
	w := get(t, newMountedUI(), "/ui/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /ui/: expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, href := range []string{`href="/dashboard"`, `href="/sessions"`, `href="/memory"`} {
		if strings.Contains(body, href) {
			t.Errorf("index at /ui/ carries root-absolute nav %s; nav must stay inside /ui", href)
		}
	}
}

// TestMountUI_NavLinksResolveInsidePrefix covers acceptance (2), second half:
// fetching each index nav href AS THE BROWSER RESOLVES IT (relative against
// the page URL /ui/) must land on a 200 page under /ui/.
func TestMountUI_NavLinksResolveInsidePrefix(t *testing.T) {
	h := newMountedUI()
	w := get(t, h, "/ui/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /ui/: expected 200, got %d", w.Code)
	}
	body := w.Body.String()

	const navPrefix = `<a href="`
	var links []string
	for _, line := range strings.Split(body, "\n") {
		i := strings.Index(line, navPrefix)
		if i < 0 {
			continue
		}
		rest := line[i+len(navPrefix):]
		end := strings.Index(rest, `"`)
		if end > 0 {
			links = append(links, rest[:end])
		}
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 nav links in index, found %d in: %q", len(links), body)
	}

	// The index page URL is /ui/; a relative href resolves as /ui/<href>.
	for _, href := range links {
		if strings.HasPrefix(href, "/") {
			t.Errorf("nav href %q is root-absolute; expected prefix-relative", href)
		}
		resolved := "/ui/" + href
		nav := get(t, h, resolved)
		if nav.Code != http.StatusOK {
			t.Errorf("nav link %q (resolves to %s): expected 200, got %d", href, resolved, nav.Code)
			continue
		}
		if !strings.HasPrefix(resolved, "/ui/") {
			t.Errorf("nav link %q landed outside the /ui prefix (%s)", href, resolved)
		}
	}
}

// TestMountUI_BareUIRedirectsToIndex covers acceptance (3): the documented
// entrypoint without a trailing slash redirects to /ui/ instead of 404ing.
func TestMountUI_BareUIRedirectsToIndex(t *testing.T) {
	w := get(t, newMountedUI(), "/ui")
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /ui: expected %d, got %d", http.StatusMovedPermanently, w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/ui/" {
		t.Errorf("GET /ui: expected Location /ui/, got %q", loc)
	}
}

// TestMountUI_ProxyPathStripsUIPrefix verifies the UI's API proxy still works
// under the mount: the mount strips /ui and the inner proxy strips the first
// /api segment (existing contract: /api/X proxies to upstream /X), so
// /ui/api/v1/health must reach the upstream as /v1/health.
func TestMountUI_ProxyPathStripsUIPrefix(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	r := chi.NewRouter()
	webUI := web.NewServer(upstream.URL)
	mountUI(r, webUI.Handler())

	w := get(t, r, "/ui/api/v1/health")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /ui/api/v1/health: expected 200 via proxy, got %d", w.Code)
	}
	if gotPath != "/v1/health" {
		t.Errorf("proxy upstream saw path %q, want /v1/health (mount must strip /ui, proxy strips /api)", gotPath)
	}
}

// TestMountUI_UnknownSubpathStill404s keeps the negative arm: the wildcard
// mount must not swallow unrelated API routes (e.g. /uiary or /v1/*).
func TestMountUI_UnknownSubpathStill404s(t *testing.T) {
	h := newMountedUI()
	if w := get(t, h, "/ui/nonexistent-page"); w.Code != http.StatusNotFound {
		t.Errorf("GET /ui/nonexistent-page: expected inner-mux 404, got %d", w.Code)
	}
	// Mounting under a wildcard must not turn "/uiX" into a UI route.
	if w := get(t, h, "/uiX"); w.Code == http.StatusOK {
		t.Errorf("GET /uiX unexpectedly served 200; wildcard must be anchored at /ui/*")
	}
}

// ch:trace row=QA-CONSENSUS-29 spec=docs/README.md#admin-ui wave=none:serial-worker test=cmd/consensus/ui_mount_test.go::TestMountUI_SubpagesUnderWildcard doc=README.md#admin-ui prompt=/tmp/consensus-foreman-20261020-233143-QA-CONSENSUS-29-brief.txt evidence=docs/testing/evidence/qa-consensus-29/ui-mount-probe.txt witness=none:local fresh serve probe verdict=none:pending-foreman-judge commit=PENDING_SHA memory=none:worker-no-memory-write
// STATUS: complete
// TIER: T3 (state focused mount and live/httptest observations)
// EVIDENCE: docs/testing/evidence/qa-consensus-29/ui-mount-probe.txt

