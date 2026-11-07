// Package web holds the site's page templates and static files.
package web

import (
	"embed"
	"io/fs"
)

//go:embed templates
var Templates embed.FS

//go:embed static
var staticFiles embed.FS

// Static is the static directory, served under /static/.
var Static = func() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return sub
}()
