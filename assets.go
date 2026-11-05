// Package bibliocell holds the files the app embeds from the project root.
// go:embed only reaches files at or below a package's directory, so the
// tray icon and the frontend build are embedded here rather than in the
// main package under cmd/bibliocell.
package bibliocell

import (
	_ "embed"
	"io/fs"
)

// TrayIcon is the template image of the menu bar and system tray icon.
//
//go:embed resources/tray.png
var TrayIcon []byte

// WebDist is the frontend build that `web` serves: dist/ embedded when the
// server is built with -tags webdist after `npm run build:web`, so one
// binary serves the whole site. It is nil otherwise, and the server reads
// the frontend from a folder (-dist, or ./dist), so `go test` and the
// desktop build need no frontend build.
var WebDist fs.FS
