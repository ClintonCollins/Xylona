package main

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:frontend/dist
var frontend embed.FS

// Frontend returns the embedded frontend filesystem rooted at the SPA build output.
func Frontend() (fs.FS, error) {
	frontendFS, errSub := fs.Sub(frontend, "frontend/dist/spa")
	if errSub != nil {
		return nil, fmt.Errorf("main: load embedded frontend: %w", errSub)
	}
	return frontendFS, nil
}
