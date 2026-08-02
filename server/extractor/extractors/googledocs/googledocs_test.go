package googledocs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asciimoo/hister/server/extractor/sdk"
)

func TestMatchGoogleDocsURLs(t *testing.T) {
	e := &GoogleDocsExtractor{}
	cases := []struct {
		url  string
		want bool
	}{
		{"https://docs.google.com/document/d/1AbC-dEf_123/edit", true},
		{"https://docs.google.com/document/d/1AbC-dEf_123/edit?tab=t.0", true},
		{"https://docs.google.com/document/d/1AbC-dEf_123/view", true},
		{"https://docs.google.com/document/d/1AbC-dEf_123", true},
		{"https://docs.google.com/document/u/0/d/1AbC-dEf_123/edit", true},
		{"https://docs.google.com/spreadsheets/d/1AbC/edit", true},
		{"https://docs.google.com/spreadsheets/u/1/d/1AbC/edit#gid=42", true},
		{"https://docs.google.com/presentation/d/1AbC/edit", false},
		{"https://docs.google.com/document/", false},
		{"https://docs.google.com/", false},
		{"https://drive.google.com/document/d/1AbC/edit", false},
		{"https://example.com/document/d/1AbC/edit", false},
	}
	for _, tc := range cases {
		d := &sdk.Document{URL: tc.url}
		if got := e.Match(d); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestSheetIDAndGID(t *testing.T) {
	cases := []struct {
		url, id, gid string
	}{
		{"https://docs.google.com/spreadsheets/d/abc_123/edit#gid=42", "abc_123", "42"},
		{"https://docs.google.com/spreadsheets/u/0/d/xyz/edit?gid=7", "xyz", "7"},
		{"https://docs.google.com/document/d/abc/edit", "", ""},
	}
	for _, tc := range cases {
		if got := sheetID(tc.url); got != tc.id {
			t.Errorf("sheetID(%q) = %q, want %q", tc.url, got, tc.id)
		}
		if got := sheetGID(tc.url); got != tc.gid {
			t.Errorf("sheetGID(%q) = %q, want %q", tc.url, got, tc.gid)
		}
	}
}

func TestDocID(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://docs.google.com/document/d/1AbC-dEf_123/edit#heading=h.abc", "1AbC-dEf_123"},
		{"https://docs.google.com/document/u/2/d/xyz/edit", "xyz"},
		{"https://docs.google.com/document/d/", ""},
		{"https://docs.google.com/spreadsheets/d/1AbC/edit", ""},
	}
	for _, tc := range cases {
		if got := docID(tc.url); got != tc.want {
			t.Errorf("docID(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// exportFixture mimics the structure of a Google Docs HTML export: styles in
// the head, class-annotated paragraph spans, headings, lists and a table.
const exportFixture = `<html><head><meta content="text/html; charset=UTF-8" http-equiv="content-type"><style type="text/css">.c0{font-size:11pt}</style><title>Project Plan</title></head><body class="doc-content">
<h1 class="c5"><span class="c3">Project Plan</span></h1>
<p class="c0"><span class="c1">This document describes the plan in detail.</span></p>
<h2 class="c5"><span class="c3">Milestones</span></h2>
<ul class="c6"><li class="c0"><span class="c1">First milestone</span></li><li class="c0"><span class="c1">Second milestone</span></li></ul>
<table class="c7"><tr><td class="c8"><p class="c0"><span class="c1">Cell text</span></p></td></tr></table>
<p class="c0"><span class="c1"></span></p>
</body></html>`

const sheetFixture = `<html><head><title>Quarterly Budget</title></head><body>
<table><thead><tr><th>Category</th><th>Amount</th></tr></thead><tbody>
<tr><td>Hardware</td><td>$1,200</td></tr><tr><td>Software</td><td>$300</td></tr>
</tbody></table></body></html>`

func TestExportContent(t *testing.T) {
	title, text, bodyHTML, err := exportContent(exportFixture)
	if err != nil {
		t.Fatalf("exportContent: %v", err)
	}
	if title != "Project Plan" {
		t.Errorf("title = %q, want %q", title, "Project Plan")
	}
	for _, want := range []string{
		"Project Plan",
		"This document describes the plan in detail.",
		"Milestones",
		"First milestone",
		"Second milestone",
		"Cell text",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q; text = %q", want, text)
		}
	}
	if got := strings.Count(text, "Cell text"); got != 1 {
		t.Errorf("nested table cell text extracted %d times, want 1", got)
	}
	if !strings.Contains(bodyHTML, "<h1") || !strings.Contains(bodyHTML, "First milestone") {
		t.Errorf("bodyHTML missing expected content: %q", bodyHTML)
	}
}

func TestSheetContent(t *testing.T) {
	title, text, bodyHTML, err := sheetContent(sheetFixture)
	if err != nil {
		t.Fatalf("sheetContent: %v", err)
	}
	if title != "Quarterly Budget" {
		t.Errorf("title = %q, want Quarterly Budget", title)
	}
	for _, want := range []string{"Category\tAmount", "Hardware\t$1,200", "Software\t$300"} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q; text = %q", want, text)
		}
	}
	if !strings.Contains(bodyHTML, "<table") {
		t.Errorf("bodyHTML missing table: %q", bodyHTML)
	}
}

// newExportServer returns a test server that serves the fixture for public
// document IDs and simulates the login redirect for anything else.
func newExportServer(t *testing.T, publicID string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fmt.Sprintf("/document/d/%s/export", publicID) && r.URL.Query().Get("format") == "html" {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte(exportFixture))
			return
		}
		if r.URL.Path == fmt.Sprintf("/spreadsheets/d/%s/gviz/tq", publicID) && r.URL.Query().Get("tqx") == "out:html" {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			_, _ = w.Write([]byte(sheetFixture))
			return
		}
		// Private/unknown docs: Google answers the export URL with a redirect
		// chain ending on a 200 login page; a plain non-200 covers the same
		// "not public" branch without needing a second host in the test.
		http.Error(w, "login required", http.StatusForbidden)
	})
	return httptest.NewServer(mux)
}

func TestExtractPublicSheet(t *testing.T) {
	srv := newExportServer(t, "sheet123")
	defer srv.Close()

	e := &GoogleDocsExtractor{base: srv.URL}
	d := &sdk.Document{
		URL:  "https://docs.google.com/spreadsheets/d/sheet123/edit#gid=42",
		HTML: "<html><body><script>spa shell</script></body></html>",
	}
	result := e.Extract(d)
	if result.Err() != nil {
		t.Fatalf("Extract: %v", result.Err())
	}
	if result.Decision() != sdk.ExtractorSuccess {
		t.Fatalf("decision = %v, want success", result.Decision())
	}
	if d.Title != "Quarterly Budget" || !strings.Contains(d.Text, "Hardware\t$1,200") {
		t.Errorf("unexpected extracted sheet: title=%q text=%q", d.Title, d.Text)
	}
	if d.Metadata["site_name"] != "Google Sheets" {
		t.Errorf("site_name = %v, want Google Sheets", d.Metadata["site_name"])
	}
}

func TestExtractPublicDoc(t *testing.T) {
	srv := newExportServer(t, "pub123")
	defer srv.Close()

	e := &GoogleDocsExtractor{base: srv.URL}
	d := &sdk.Document{
		URL:  "https://docs.google.com/document/d/pub123/edit",
		HTML: "<html><body><script>spa shell</script></body></html>",
	}
	result := e.Extract(d)
	if result.Err() != nil {
		t.Fatalf("Extract: %v", result.Err())
	}
	if result.Decision() != sdk.ExtractorSuccess {
		t.Fatalf("decision = %v, want success", result.Decision())
	}
	if d.Title != "Project Plan" {
		t.Errorf("Title = %q, want %q", d.Title, "Project Plan")
	}
	if !strings.Contains(d.Text, "First milestone") {
		t.Errorf("Text missing export content: %q", d.Text)
	}
	if !strings.Contains(d.HTML, "doc-content") {
		t.Errorf("HTML was not replaced with export HTML")
	}
	if d.Metadata["site_name"] != "Google Docs" {
		t.Errorf("site_name = %v, want Google Docs", d.Metadata["site_name"])
	}
}

func TestExtractPrivateDocContinues(t *testing.T) {
	srv := newExportServer(t, "pub123")
	defer srv.Close()

	e := &GoogleDocsExtractor{base: srv.URL}
	d := &sdk.Document{
		URL:  "https://docs.google.com/document/d/private456/edit",
		HTML: "<html><body>shell</body></html>",
	}
	result := e.Extract(d)
	if result.Decision() != sdk.ExtractorFallback {
		t.Fatalf("decision = %v, want fallback", result.Decision())
	}
	if result.Err() == nil {
		t.Fatal("expected error for private doc")
	}
	if d.Text != "" || d.Title != "" {
		t.Errorf("document should be untouched on failure, got Title=%q Text=%q", d.Title, d.Text)
	}
	if !strings.Contains(d.HTML, "shell") {
		t.Errorf("HTML should be untouched on failure")
	}
}

func TestPreviewFromStoredExport(t *testing.T) {
	e := &GoogleDocsExtractor{}
	d := &sdk.Document{
		URL:  "https://docs.google.com/document/d/pub123/edit",
		HTML: exportFixture, // as stored by Extract
	}
	result := e.Preview(d)
	if result.Err() != nil {
		t.Fatalf("Preview: %v", result.Err())
	}
	if result.Decision() != sdk.ExtractorSuccess {
		t.Fatalf("decision = %v, want success", result.Decision())
	}
	resp := result.Response()
	if !strings.Contains(resp.Content, "First milestone") {
		t.Errorf("preview missing content: %q", resp.Content)
	}
	if strings.Contains(resp.Content, "<script") {
		t.Errorf("preview must be sanitized")
	}
}

func TestPreviewRefetchesStubSnapshot(t *testing.T) {
	srv := newExportServer(t, "pub123")
	defer srv.Close()

	e := &GoogleDocsExtractor{base: srv.URL}
	d := &sdk.Document{
		URL:  "https://docs.google.com/document/d/pub123/edit",
		HTML: "<html><body><script>spa shell</script></body></html>", // pre-fork stub
	}
	result := e.Preview(d)
	if result.Err() != nil {
		t.Fatalf("Preview: %v", result.Err())
	}
	if result.Decision() != sdk.ExtractorSuccess {
		t.Fatalf("decision = %v, want success", result.Decision())
	}
	resp := result.Response()
	if !strings.Contains(resp.Content, "First milestone") {
		t.Errorf("preview missing refetched content: %q", resp.Content)
	}
}

func TestSetConfigRejectsUnknownOption(t *testing.T) {
	e := &GoogleDocsExtractor{}
	cfg := e.GetConfig()
	cfg.Options["bogus"] = true
	if err := e.SetConfig(cfg); err == nil {
		t.Fatal("expected error for unknown option")
	}
}
