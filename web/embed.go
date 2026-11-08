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

// Features holds the Features tour's slides, one Markdown file each.
var Features = func() fs.FS {
	sub, err := fs.Sub(featureFiles, "features")
	if err != nil {
		panic(err)
	}
	return sub
}()

//go:embed features
var featureFiles embed.FS

// Static is the static directory, served under /static/.
var Static = func() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return sub
}()
