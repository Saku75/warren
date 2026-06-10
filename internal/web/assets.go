// Package web holds Warren's HTML components (templ) and static assets.
// Everything the browser needs ships inside the binary: replicas have no
// filesystem state and no external asset dependencies.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assets embed.FS

// AssetHandler serves the embedded static assets.
func AssetHandler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic("web: embedded assets missing: " + err.Error())
	}
	return http.FileServerFS(sub)
}
