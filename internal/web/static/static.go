// Package static provides embedded static assets (JS, CSS) for the MiGate WebUI.
package static

import (
	"embed"
)

// FS holds the embedded static files served by the MiGate WebUI.
// Files are rooted at the package directory.
//
//go:embed app.js
var FS embed.FS