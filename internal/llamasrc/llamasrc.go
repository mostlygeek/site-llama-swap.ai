// Package llamasrc embeds llama-swap's source code for the Help agent's
// code__* tools.
//
// The archive is generated from the llama-swap version in go.mod. Run
// `go generate ./...` (or `make generate`) after updating llama-swap and
// commit the result.
package llamasrc

import _ "embed"

//go:generate go tool gensource -o llama-swap-src.tar.gz

// Archive is a gzipped tar of llama-swap's Go and TypeScript files, without
// tests. Load it with docbot.SourceFromArchive.
//
//go:embed llama-swap-src.tar.gz
var Archive []byte
