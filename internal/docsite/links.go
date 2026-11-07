package docsite

import (
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// sourceKey carries the Markdown file's path in llama-swap's docs directory
// to the link rewriter, so relative links can be resolved.
var sourceKey = parser.NewContextKey()

// linkRewriter points links between documents at the site's own pages:
//
//	../routing/groups-and-matrix.md  -> /docs/guides/routing/groups-and-matrix
//	../../../config.example.yaml     -> /docs/reference/config
//
// A relative link to any other file in the repository goes to GitHub.
// Links that leave the site open in a new tab.
type linkRewriter struct{}

func (linkRewriter) Transform(doc *ast.Document, _ text.Reader, pc parser.Context) {
	source, _ := pc.Get(sourceKey).(string)
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch link := n.(type) {
		case *ast.Link:
			dest, external := rewriteLink(source, string(link.Destination))
			link.Destination = []byte(dest)
			if external {
				link.SetAttributeString("target", []byte("_blank"))
				link.SetAttributeString("rel", []byte("noopener"))
			}
		case *ast.AutoLink:
			// Bare URLs are always external.
			link.SetAttributeString("target", []byte("_blank"))
			link.SetAttributeString("rel", []byte("noopener"))
		}
		return ast.WalkContinue, nil
	})
}

// rewriteLink resolves dest, written in the file at source (a path inside
// llama-swap's docs directory), and reports whether it leaves the site.
func rewriteLink(source, dest string) (string, bool) {
	if dest == "" || strings.HasPrefix(dest, "#") {
		return dest, false
	}
	if u, err := url.Parse(dest); err != nil || u.Scheme != "" || strings.HasPrefix(dest, "//") {
		return dest, true
	}
	if strings.HasPrefix(dest, "/") {
		// Root-relative links in the repository mean the repository root.
		return repoBlob + strings.TrimPrefix(dest, "/"), true
	}

	file, fragment, _ := strings.Cut(dest, "#")
	if fragment != "" {
		fragment = "#" + fragment
	}
	resolved := path.Clean(path.Join(path.Dir(source), file))

	switch {
	case strings.HasPrefix(resolved, kbDir+"/") && strings.HasSuffix(resolved, ".md"):
		return "/docs/" + strings.TrimSuffix(strings.TrimPrefix(resolved, kbDir+"/"), ".md") + fragment, false
	case resolved == configExample:
		return "/docs/" + ConfigID + fragment, false
	case resolved == readme:
		return "/docs/" + readmeID + fragment, false
	}
	// Anything else is a file elsewhere in the repository.
	return repoBlob + path.Clean(path.Join("docs", resolved)) + fragment, true
}
