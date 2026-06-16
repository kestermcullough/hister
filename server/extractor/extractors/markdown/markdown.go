// SPDX-License-Identifier: AGPL-3.0-or-later

// Package markdown provides an extractor for locally indexed Markdown files.
package markdown

import (
	"fmt"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/sanitizer"
	"github.com/asciimoo/hister/server/types"
)

// MarkdownExtractor renders previews for locally indexed Markdown files.
// During indexing, files.go stores the raw Markdown source in doc.HTML so
// this extractor can convert it to sanitized HTML for the preview panel.
type MarkdownExtractor struct {
	cfg *config.Extractor
}

func (e *MarkdownExtractor) Name() string { return "Markdown" }

func (e *MarkdownExtractor) Description() string {
	return "Renders locally indexed Markdown files (.md, .markdown) as HTML for preview."
}

func (e *MarkdownExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{Enable: true, Options: map[string]any{}}
	}
	return e.cfg
}

func (e *MarkdownExtractor) SetConfig(c *config.Extractor) error {
	for k := range c.Options {
		return fmt.Errorf("unknown option %q", k)
	}
	e.cfg = c
	return nil
}

// Match returns true for file:// URLs with a .md or .markdown extension.
func (e *MarkdownExtractor) Match(d *document.Document) bool {
	if !strings.HasPrefix(d.URL, "file://") {
		return false
	}
	lower := strings.ToLower(d.URL)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// Extract is a no-op: indexing is handled by indexer.AddMarkdown which stores
// the raw source in doc.Text directly.
func (e *MarkdownExtractor) Extract(_ *document.Document) (types.ExtractorState, error) {
	return types.ExtractorContinue, nil
}

// Preview renders the Markdown source stored in doc.HTML to sanitized HTML.
func (e *MarkdownExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	if d.HTML == "" {
		return types.PreviewResponse{}, types.ExtractorContinue, nil
	}
	rendered := renderMarkdown([]byte(d.HTML))
	return types.PreviewResponse{Content: sanitizer.SanitizeHTML(rendered)}, types.ExtractorStop, nil
}

func renderMarkdown(src []byte) string {
	p := parser.NewWithExtensions(
		parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock,
	)
	opts := html.RendererOptions{Flags: html.CommonFlags | html.HrefTargetBlank}
	r := html.NewRenderer(opts)
	return string(markdown.ToHTML(src, p, r))
}
