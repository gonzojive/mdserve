package main

import (
	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
)

// createMarkdownParser creates and configures the default Goldmark markdown parser.
// It includes extensions for GFM, front-matter metadata parsing, and footnotes,
// as well as custom renderer options.
func createMarkdownParser() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			meta.Meta,
			extension.GFM,
			extension.Footnote,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			htmlrenderer.WithUnsafe(),
		),
	)
}
