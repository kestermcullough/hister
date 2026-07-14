// SPDX-License-Identifier: AGPL-3.0-or-later

// Package googledocs is a [fork] extractor for public Google Docs documents.
// The editor URL (docs.google.com/document/d/<id>/edit) renders its content
// entirely client-side, so the http crawler backend only sees an empty SPA
// shell. For public documents Google serves static HTML from the export
// endpoint (/document/d/<id>/export?format=html), so this extractor re-fetches
// the document from there and replaces the shell with the exported content —
// both for the search index and for the stored snapshot. Private documents
// redirect the export URL to a Google login page; in that case the extractor
// returns Continue and the generic chain handles the shell as before.
package googledocs

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/asciimoo/hister/server/extractor/sdk"
	"github.com/asciimoo/hister/server/sanitizer"
)

const (
	defaultTimeout = 15 // seconds
	defaultBase    = "https://docs.google.com"
	// maxExportSize caps the export response body (export HTML of very large
	// documents can embed base64 images).
	maxExportSize = 32 << 20 // 32 MiB
)

// docPathRe extracts the document ID from an editor or viewer URL path.
var docPathRe = regexp.MustCompile(`^/document(?:/u/\d+)?/d/([a-zA-Z0-9_-]+)`)

// GoogleDocsExtractor fetches public Google Docs via the static export endpoint.
type GoogleDocsExtractor struct {
	cfg    *sdk.Config
	client *http.Client
	// base is the scheme+host the export URL is built on; overridable in tests.
	base string
}

func (e *GoogleDocsExtractor) Name() string {
	return "GoogleDocs"
}

func (e *GoogleDocsExtractor) Description() string {
	return "Extracts public Google Docs documents via the static export endpoint (export?format=html), which needs no JavaScript. Private documents fall through to the generic extractors."
}

func (e *GoogleDocsExtractor) Capabilities() sdk.Capabilities {
	return sdk.Capabilities{Extract: true, Preview: true}
}

func (e *GoogleDocsExtractor) GetConfig() *sdk.Config {
	if e.cfg == nil {
		return &sdk.Config{
			Enable: true,
			Options: map[string]any{
				"timeout": defaultTimeout,
			},
		}
	}
	return e.cfg
}

func (e *GoogleDocsExtractor) SetConfig(c *sdk.Config) error {
	timeout := defaultTimeout
	for k, v := range c.Options {
		switch k {
		case "timeout":
			switch n := v.(type) {
			case int:
				timeout = n
			case int64:
				timeout = int(n)
			case float64:
				timeout = int(n)
			}
		default:
			return fmt.Errorf("unknown option %q", k)
		}
	}
	e.cfg = c
	e.client = &http.Client{Timeout: time.Duration(timeout) * time.Second}
	return nil
}

// Match accepts docs.google.com document URLs (/document/d/<id>/…, including
// the /u/<n>/ account-switcher form). Sheets and Slides are out of scope for
// now; their export endpoints differ.
func (e *GoogleDocsExtractor) Match(d *sdk.Document) bool {
	u, err := url.Parse(d.URL)
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Hostname(), "docs.google.com") {
		return false
	}
	return docPathRe.MatchString(u.Path)
}

// docID returns the document ID embedded in the URL path, or "".
func docID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := docPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return ""
	}
	return m[1]
}

// fetchExport downloads the static HTML export of the document. It returns an
// error when the document is not publicly exportable (login redirect or
// non-200 status).
func (e *GoogleDocsExtractor) fetchExport(rawURL string) (string, error) {
	id := docID(rawURL)
	if id == "" {
		return "", fmt.Errorf("no document id in URL")
	}
	base := e.base
	if base == "" {
		base = defaultBase
	}
	client := e.client
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout * time.Second}
	}
	exportURL := fmt.Sprintf("%s/document/d/%s/export?format=html", base, id)
	resp, err := client.Get(exportURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	// Private documents redirect to accounts.google.com; the login page itself
	// answers 200, so the final host is the reliable signal.
	if h := resp.Request.URL.Hostname(); strings.EqualFold(h, "accounts.google.com") {
		return "", fmt.Errorf("document is not public (redirected to login)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("export returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxExportSize))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// exportContent parses exported HTML into a title, plain text and the body's
// inner HTML.
func exportContent(exportHTML string) (title, text, bodyHTML string, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(exportHTML))
	if err != nil {
		return "", "", "", err
	}
	title = strings.TrimSpace(doc.Find("title").First().Text())

	var b strings.Builder
	doc.Find("p, h1, h2, h3, h4, h5, h6, li").Each(func(_ int, s *goquery.Selection) {
		// Skip nested matches (e.g. a p inside a table cell that is itself
		// inside a matched element) so text is not duplicated.
		if s.ParentsFiltered("p, h1, h2, h3, h4, h5, h6, li").Length() > 0 {
			return
		}
		if t := strings.TrimSpace(s.Text()); t != "" {
			b.WriteString(t)
			b.WriteString("\n")
		}
	})
	text = strings.TrimSpace(b.String())

	bodyHTML, err = doc.Find("body").First().Html()
	if err != nil {
		return "", "", "", err
	}
	return title, text, strings.TrimSpace(bodyHTML), nil
}

func (e *GoogleDocsExtractor) Extract(d *sdk.Document) sdk.ExtractResult {
	exportHTML, err := e.fetchExport(d.URL)
	if err != nil {
		// Not public / fetch failed: let the generic chain handle the shell.
		return sdk.ExtractFallback(err)
	}
	title, text, _, err := exportContent(exportHTML)
	if err != nil {
		return sdk.ExtractFallback(err)
	}
	if text == "" && title == "" {
		return sdk.ExtractFallback(fmt.Errorf("export contained no content"))
	}
	// Replace the SPA shell so the stored snapshot holds the real document.
	d.HTML = exportHTML
	if title != "" {
		d.Title = title
	}
	d.Text = text
	if d.Metadata == nil {
		d.Metadata = make(map[string]any)
	}
	d.Metadata["site_name"] = "Google Docs"
	return sdk.Extracted()
}

func (e *GoogleDocsExtractor) Preview(d *sdk.Document) sdk.PreviewResult {
	// Documents indexed through Extract already store the export HTML, so the
	// snapshot renders directly. Older stub snapshots (empty SPA shell) yield
	// no text; re-fetch the export for those.
	if _, text, bodyHTML, err := exportContent(d.HTML); err == nil && text != "" {
		return sdk.Previewed(sdk.PreviewResponse{Content: sanitizer.SanitizeHTML(bodyHTML)})
	}
	exportHTML, err := e.fetchExport(d.URL)
	if err != nil {
		return sdk.PreviewFallback(err)
	}
	_, text, bodyHTML, err := exportContent(exportHTML)
	if err != nil || text == "" {
		return sdk.PreviewFallback(fmt.Errorf("no preview content"))
	}
	return sdk.Previewed(sdk.PreviewResponse{Content: sanitizer.SanitizeHTML(bodyHTML)})
}
