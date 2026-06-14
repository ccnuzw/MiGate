// Package static provides embedded frontend assets for the MiGate WebUI.
package static

import (
	"embed"
	"io/fs"
)

// FS holds the embedded Vite build output served by the MiGate WebUI.
//
//go:embed dist
var FS embed.FS

// Dist returns an fs.FS rooted at the embedded Vite dist directory.
func Dist() fs.FS {
	sub, err := fs.Sub(FS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Assets returns an fs.FS rooted at the embedded Vite assets directory.
func Assets() fs.FS {
	sub, err := fs.Sub(FS, "dist/assets")
	if err != nil {
		panic(err)
	}
	return sub
}

// ReadIndex returns the SPA entrypoint.
func ReadIndex() ([]byte, error) {
	return FS.ReadFile("dist/index.html")
}
