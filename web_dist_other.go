//go:build !webdist

package main

import "io/fs"

// Without -tags webdist the server reads the frontend from a folder (-dist,
// or ./dist), so `go test` and the desktop build need no frontend build.
func embeddedFrontend() fs.FS { return nil }
