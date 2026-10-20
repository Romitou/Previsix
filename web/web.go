// Package web embeds the Previsix web interface.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var static embed.FS

// Files returns the web interface files, index.html being at the root.
func Files() fs.FS {
	files, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	return files
}
