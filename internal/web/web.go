package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embeddedFS embed.FS

// FS is the compiled React build, rooted at the dist directory.
// Pass this to api.NewServer.
var FS, _ = fs.Sub(embeddedFS, "dist")
