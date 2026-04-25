// Package webui provides the embedded frontend build output.
package webui

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:dist
var frontend embed.FS

// Frontend returns the embedded frontend filesystem rooted at the SPA build output.
func Frontend() (fs.FS, error) {
	frontendFS, errSub := fs.Sub(frontend, "dist/spa")
	if errSub != nil {
		return nil, fmt.Errorf("webui: load embedded frontend: %w", errSub)
	}
	return frontendFS, nil
}
