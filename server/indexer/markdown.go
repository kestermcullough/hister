// SPDX-License-Identifier: AGPL-3.0-or-later

package indexer

import (
	"errors"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	"github.com/asciimoo/hister/server/document"
)

// AddMarkdown renders mdData to HTML, stores it in d.HTML, and stores the raw
// source in d.Text for full-text indexing.
func AddMarkdown(d *document.Document, mdData []byte) error {
	src := strings.TrimSpace(string(mdData))
	if src == "" {
		return errors.New("markdown file is empty")
	}
	d.HTML = renderMarkdown(mdData)
	d.Text = src
	d.Title = extractMarkdownTitle(src)
	d.AddMetadata("type", "markdown")
	return Add(d)
}

// extractMarkdownTitle returns the text of the first ATX H1 heading ("# ...").
// Returns an empty string if none is found.
func extractMarkdownTitle(src string) string {
	for _, line := range strings.SplitAfter(src, "\n") {
		t := strings.TrimRight(line, "\r\n")
		if after, ok := strings.CutPrefix(t, "# "); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func renderMarkdown(src []byte) string {
	p := parser.NewWithExtensions(
		parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock,
	)
	opts := html.RendererOptions{Flags: html.CommonFlags | html.HrefTargetBlank}
	r := html.NewRenderer(opts)
	return string(markdown.ToHTML(src, p, r))
}
