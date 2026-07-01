package defuddle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asciimoo/hister/server/extractor/sdk"
)

func TestExtractUsesSidecarResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/parse" {
			t.Fatalf("request = %s %s, want POST /parse", r.Method, r.URL.Path)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["url"] != "https://example.com/story" {
			t.Errorf("sidecar URL = %#v", request["url"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":"<article><h1>Extracted</h1><script>bad()</script><p>Discussion text</p></article>",
			"title":"Sidecar title",
			"author":"A. Writer",
			"site":"Example"
		}`))
	}))
	defer server.Close()

	extractor := &DefuddleExtractor{}
	if err := extractor.SetConfig(&sdk.Config{
		Enable: true,
		Options: map[string]any{
			"endpoint": server.URL + "/parse",
			"timeout":  2,
			"markdown": false,
		},
	}); err != nil {
		t.Fatal(err)
	}
	doc := &sdk.Document{
		URL:  "https://example.com/story",
		HTML: "<html><body>source</body></html>",
	}

	result := extractor.Extract(doc)
	if result.Err() != nil || result.Decision() != sdk.ExtractorSuccess {
		t.Fatalf("Extract() = (%v, %v), want success", result.Decision(), result.Err())
	}
	if doc.Title != "Sidecar title" || !strings.Contains(doc.Text, "Discussion text") {
		t.Fatalf("document not populated from sidecar: %+v", doc)
	}
	if strings.Contains(doc.Text, "bad()") {
		t.Fatalf("document text retained script content: %q", doc.Text)
	}
	if doc.Metadata["author"] != "A. Writer" || doc.Metadata["site_name"] != "Example" {
		t.Fatalf("metadata = %#v", doc.Metadata)
	}

	preview := extractor.Preview(&sdk.Document{URL: doc.URL, HTML: doc.HTML})
	if preview.Err() != nil || preview.Decision() != sdk.ExtractorSuccess {
		t.Fatalf("Preview() = (%v, %v), want success", preview.Decision(), preview.Err())
	}
	if strings.Contains(preview.Response().Content, "<script") {
		t.Fatalf("preview retained script: %q", preview.Response().Content)
	}
}

func TestExtractFallsBackWhenSidecarUnavailable(t *testing.T) {
	extractor := &DefuddleExtractor{}
	if err := extractor.SetConfig(&sdk.Config{
		Enable: true,
		Options: map[string]any{
			"endpoint": "http://127.0.0.1:1/parse",
			"timeout":  1,
			"markdown": false,
		},
	}); err != nil {
		t.Fatal(err)
	}

	result := extractor.Extract(&sdk.Document{HTML: "<p>source</p>"})
	if result.Decision() != sdk.ExtractorFallback || result.Err() == nil {
		t.Fatalf("Extract() = (%v, %v), want fallback with error", result.Decision(), result.Err())
	}
}
