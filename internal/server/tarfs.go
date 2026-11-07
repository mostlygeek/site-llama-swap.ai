package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"path"
	"testing/fstest"
)

// maxTarFileBytes bounds one file in the source tarball. The largest
// llama-swap source file is well under this; anything bigger is a bad archive.
const maxTarFileBytes = 8 << 20

// tarGzFS unpacks a gzipped tar into an in-memory file system. The source
// behind the code__* tools is about 2 MB uncompressed, so holding it in memory
// is cheaper than writing it to disk.
func tarGzFS(data []byte) (fs.FS, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	files := fstest.MapFS{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(header.Name)
		if !fs.ValidPath(name) {
			return nil, fmt.Errorf("invalid path %q in tar", header.Name)
		}
		if header.Size > maxTarFileBytes {
			return nil, fmt.Errorf("%s is %d bytes, over the %d byte limit", name, header.Size, maxTarFileBytes)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		files[name] = &fstest.MapFile{Data: content, Mode: 0o444}
	}
	return files, nil
}
