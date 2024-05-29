package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend/dist
var frontend embed.FS

func Frontend() (fs.FS, error) {
	return fs.Sub(frontend, "frontend/dist/spa")
}
