// SPDX-License-Identifier: AGPL-3.0-or-later

package indexer

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/niklasfasching/go-org/org"

	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/sanitizer"
)

// AddOrg renders Org files to HTML, stores it in d.HTML, and stores the raw
// source in d.Text for full-text indexing.
func AddOrg(d *document.Document, orgData []byte) error {
	src := strings.TrimSpace(string(orgData))
	if src == "" {
		return errors.New("org file empty")
	}
	html, title, err := renderOrg(orgData)
	if err != nil {
		return fmt.Errorf("rendering org: %w", err)
	}
	d.HTML = html
	d.Text = sanitizer.SanitizeText(d.HTML)
	d.Title = title
	d.AddMetadata("type", "org")
	return Add(d)
}

func renderOrg(src []byte) (string, string, error) {
	doc := org.New().Parse(bytes.NewReader(src), "./")

	html, error := doc.Write(org.NewHTMLWriter())
	if error != nil {
		return "", "", error
	}
	title := doc.Get("TITLE")
	return html, title, nil
}
