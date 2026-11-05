//go:build webdist

package bibliocell

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var webDist embed.FS

func init() {
	if sub, err := fs.Sub(webDist, "dist"); err == nil {
		WebDist = sub
	}
}
