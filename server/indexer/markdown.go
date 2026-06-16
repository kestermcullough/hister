// SPDX-License-Identifier: AGPL-3.0-or-later

package indexer

import (
	"errors"
	"strings"

	"github.com/asciimoo/hister/server/document"
)

// AddMarkdown stores the raw markdown source in d.HTML (so the markdown
// extractor can render it for preview) and plain text in d.Text for indexing.
func AddMarkdown(d *document.Document, mdData []byte) error {
	src := strings.TrimSpace(string(mdData))
	if src == "" {
		return errors.New("markdown file is empty")
	}
	d.HTML = src
	d.Text = src
	d.AddMetadata("type", "markdown")
	return Add(d)
}
