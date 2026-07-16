package webserver

import (
	"embed"
	"io/fs"
	"net/http"
)

// webFS holds the self-contained SPA (index.html + app.js + style.css). It is
// embedded into the binary at build time — no CDN, no external fetch, no build
// step — so `aitop serve` is a single static binary, consistent with the
// repo's zero-dependency / CGO_ENABLED=0 discipline.
//
//go:embed web
var webFS embed.FS

// assetsHandler serves the embedded web/ directory at the site root. The
// http.FileServer already maps "/" to index.html, so no route logic is needed
// here — a missing asset returns a plain 404, which is correct for a fixed,
// known asset set.
func assetsHandler() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		// Only reachable if the embed directive and this path disagree — a
		// build-time bug, not a runtime condition.
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
