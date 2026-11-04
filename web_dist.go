//go:build webdist

package main

import (
	"embed"
	"io/fs"
)

// The frontend build, embedded when the server is built with -tags webdist
// after `npm run build:web`, so one binary serves the whole site.
//
//go:embed all:dist
var webDist embed.FS

func embeddedFrontend() fs.FS {
	sub, err := fs.Sub(webDist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
