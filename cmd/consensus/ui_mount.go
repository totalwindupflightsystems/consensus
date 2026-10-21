package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// mountUI wires the web admin UI into the router exactly as production
// serves it (QA-CONSENSUS-29).
//
// Two properties matter:
//
//  1. The subtree mount uses chi's /* wildcard. A bare "/ui/" pattern
//     matches only the exact path "/ui/", so every UI subpage
//     (/ui/dashboard, /ui/sessions, /ui/memory, /ui/health) previously
//     fell through to the router's NotFound handler while /ui/ itself
//     served the index. Wildcards are also what every sibling mount uses
//     (/mcp/*, /webhooks/*, /chronicle/*).
//  2. The bare "/ui" path redirects to "/ui/" so the documented entrypoint
//     works with or without the trailing slash (mirroring the /chronicle
//     mount pair).
//
// The UI's own handler receives request paths with the /ui prefix stripped.
func mountUI(r chi.Router, ui http.Handler) {
	r.Handle("/ui/*", http.StripPrefix("/ui", ui))
	r.Handle("/ui", http.RedirectHandler("/ui/", http.StatusMovedPermanently))
}
