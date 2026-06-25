package org

import (
	"fmt"
	"strings"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/sanitizer"
	"github.com/asciimoo/hister/server/types"
)

// OrgModeExtractor serves previews for locally indexed Org files.
// During indexing, indexer.AddOrg renders the source to HTML and stores
// it in doc.HTML, so Preview only needs to sanitize that HTML.
type OrgModeExtractor struct {
	cfg *config.Extractor
}

func (e *OrgModeExtractor) Name() string { return "OrgMode" }

func (e *OrgModeExtractor) Description() string {
	return "Renders locally indexed Org files (.org) as HTML for preview."
}

func (e *OrgModeExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{Enable: true, Options: map[string]any{}}
	}
	return e.cfg
}

func (e *OrgModeExtractor) SetConfig(c *config.Extractor) error {
	for k := range c.Options {
		return fmt.Errorf("unknown option %q", k)
	}
	e.cfg = c
	return nil
}

// Match returns true for file:// URLs with an  .org extension.
func (e *OrgModeExtractor) Match(d *document.Document) bool {
	if !strings.HasPrefix(d.URL, "file://") {
		return false
	}
	lower := strings.ToLower(d.URL)
	return strings.HasSuffix(lower, ".org")
}

// Extract is a no-op: indexing is handled by indexer.AddOrg.
func (e *OrgModeExtractor) Extract(_ *document.Document) (types.ExtractorState, error) {
	return types.ExtractorContinue, nil
}

// Preview sanitizes the rendered HTML stored in doc.HTML.
func (e *OrgModeExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	if d.HTML == "" {
		return types.PreviewResponse{}, types.ExtractorContinue, nil
	}
	return types.PreviewResponse{Content: sanitizer.SanitizeHTML(d.HTML)}, types.ExtractorStop, nil
}
