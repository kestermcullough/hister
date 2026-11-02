// SPDX-License-Identifier: AGPL-3.0-or-later

// Package googledocs is a [fork] extractor for public Google Docs and Sheets.
// Their editor URLs render content client-side, so the http crawler backend
// only sees an empty SPA shell. Public resources expose static HTML through a
// Docs export endpoint or the Sheets visualization endpoint; this extractor
// stores that content for search and preview. Private resources fall through to
// the generic extractor chain.
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

var (
	docPathRe   = regexp.MustCompile(`^/document(?:/u/\d+)?/d/([a-zA-Z0-9_-]+)`)
	sheetPathRe = regexp.MustCompile(`^/spreadsheets(?:/u/\d+)?/d/([a-zA-Z0-9_-]+)`)
)

// GoogleDocsExtractor fetches public Google Docs and Sheets via static endpoints.
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
	return "Extracts public Google Docs and Sheets through static HTML endpoints that need no JavaScript. Private resources fall through to the generic extractors."
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

// Match accepts docs.google.com document and spreadsheet URLs, including the
// /u/<n>/ account-switcher form.
func (e *GoogleDocsExtractor) Match(d *sdk.Document) bool {
	u, err := url.Parse(d.URL)
	if err != nil {
		return false
	}
	if !strings.EqualFold(u.Hostname(), "docs.google.com") {
		return false
	}
	return docPathRe.MatchString(u.Path) || sheetPathRe.MatchString(u.Path)
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

func sheetID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := sheetPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return ""
	}
	return m[1]
}

// sheetGID returns the selected worksheet gid from either the query or fragment.
func sheetGID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if gid := u.Query().Get("gid"); gid != "" {
		return gid
	}
	fragment, err := url.ParseQuery(u.Fragment)
	if err != nil {
		return ""
	}
	return fragment.Get("gid")
}

// fetchExport downloads the static HTML export of the document. It returns an
// error when the document is not publicly exportable (login redirect or
// non-200 status).
func (e *GoogleDocsExtractor) fetchExport(rawURL string) (string, error) {
	base := e.base
	if base == "" {
		base = defaultBase
	}
	client := e.client
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout * time.Second}
	}
	var exportURL string
	if id := docID(rawURL); id != "" {
		exportURL = fmt.Sprintf("%s/document/d/%s/export?format=html", base, id)
	} else if id := sheetID(rawURL); id != "" {
		q := url.Values{"tqx": {"out:html"}}
		if gid := sheetGID(rawURL); gid != "" {
			q.Set("gid", gid)
		}
		exportURL = fmt.Sprintf("%s/spreadsheets/d/%s/gviz/tq?%s", base, id, q.Encode())
	} else {
		return "", fmt.Errorf("no document or spreadsheet id in URL")
	}
	resp, err := client.Get(exportURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	// Private documents redirect to accounts.google.com; the login page itself
	// answers 200, so the final host is the reliable signal.
	if h := resp.Request.URL.Hostname(); strings.EqualFold(h, "accounts.google.com") {
		return "", fmt.Errorf("resource is not public (redirected to login)")
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

// sheetContent converts the visualization endpoint's table into tab-separated
// rows, while retaining its HTML body for offline preview.
func sheetContent(exportHTML string) (title, text, bodyHTML string, err error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(exportHTML))
	if err != nil {
		return "", "", "", err
	}
	title = strings.TrimSpace(doc.Find("title").First().Text())

	var rows []string
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		var cells []string
		row.Find("th, td").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, strings.TrimSpace(cell.Text()))
		})
		if joined := strings.TrimSpace(strings.Join(cells, "\t")); joined != "" {
			rows = append(rows, joined)
		}
	})
	if len(rows) == 0 {
		return "", "", "", fmt.Errorf("sheet export contained no tabular rows")
	}
	text = strings.Join(rows, "\n")
	bodyHTML, err = doc.Find("body").First().Html()
	if err != nil {
		return "", "", "", err
	}
	return title, strings.TrimSpace(text), strings.TrimSpace(bodyHTML), nil
}

func contentForURL(rawURL, exportHTML string) (title, text, bodyHTML string, err error) {
	if sheetID(rawURL) != "" {
		return sheetContent(exportHTML)
	}
	return exportContent(exportHTML)
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

// shellTitle returns the editor page's <title> without the product suffix.
func shellTitle(shellHTML string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(shellHTML))
	if err != nil {
		return ""
	}
	title := strings.TrimSpace(doc.Find("title").First().Text())
	for _, suffix := range []string{" - Google Docs", " - Google Sheets"} {
		title = strings.TrimSpace(strings.TrimSuffix(title, suffix))
	}
	return title
}

func (e *GoogleDocsExtractor) Extract(d *sdk.Document) sdk.ExtractResult {
	exportHTML, err := e.fetchExport(d.URL)
	if err != nil {
		// Not public / fetch failed: let the generic chain handle the shell.
		return sdk.ExtractFallback(err)
	}
	title, text, _, err := contentForURL(d.URL, exportHTML)
	if err != nil {
		return sdk.ExtractFallback(err)
	}
	if text == "" && title == "" {
		return sdk.ExtractFallback(fmt.Errorf("export contained no content"))
	}
	// Docs exports carry no <title>. Keep a caller-provided title, or fall back
	// to the editor shell's title before the shell is replaced.
	if title == "" && strings.TrimSpace(d.Title) == "" {
		title = shellTitle(d.HTML)
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
	if sheetID(d.URL) != "" {
		d.Metadata["site_name"] = "Google Sheets"
	} else {
		d.Metadata["site_name"] = "Google Docs"
	}
	return sdk.Extracted()
}

func (e *GoogleDocsExtractor) Preview(d *sdk.Document) sdk.PreviewResult {
	// Documents indexed through Extract already store the export HTML, so the
	// snapshot renders directly. Older stub snapshots (empty SPA shell) yield
	// no text; re-fetch the export for those.
	if _, text, bodyHTML, err := contentForURL(d.URL, d.HTML); err == nil && text != "" {
		return sdk.Previewed(sdk.PreviewResponse{Content: sanitizer.SanitizeHTML(bodyHTML)})
	}
	exportHTML, err := e.fetchExport(d.URL)
	if err != nil {
		return sdk.PreviewFallback(err)
	}
	_, text, bodyHTML, err := contentForURL(d.URL, exportHTML)
	if err != nil || text == "" {
		return sdk.PreviewFallback(fmt.Errorf("no preview content"))
	}
	return sdk.Previewed(sdk.PreviewResponse{Content: sanitizer.SanitizeHTML(bodyHTML)})
}
