// Package llamasrc embeds llama-swap's source code for the Help agent's
// code__* tools.
//
// The archive is not committed. `make` (or `go generate ./...`) writes it from
// the llama-swap version in go.mod, which Go downloads from GitHub like any
// other dependency. A plain `go build` without it still works; the site then
// serves the docs__* tools only.
package llamasrc

import "embed"

//go:generate go tool gensource -o archive/llama-swap-src.tar.gz

// The directory always holds README.md, so the embed pattern matches even
// before the archive is generated.
//
//go:embed archive
var files embed.FS

const archiveName = "archive/llama-swap-src.tar.gz"

// Archive returns the generated archive of llama-swap's Go and TypeScript
// files, or nil when it was not generated before the build. Load it with
// docbot.SourceFromArchive.
func Archive() []byte {
	data, err := files.ReadFile(archiveName)
	if err != nil {
		return nil
	}
	return data
}
