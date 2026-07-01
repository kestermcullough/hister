// SPDX-License-Identifier: AGPL-3.0-or-later

// Package defuddle is a [fork] generic content extractor backed by a Node
// sidecar running Defuddle (the extraction engine behind Obsidian Web Clipper).
// It runs just before Readability in the chain: when the sidecar returns
// content it wins (Defuddle keeps discussion/comments — e.g. Reddit, HN — that
// Readability discards); otherwise it returns Continue and Readability handles
// the page. Disabled by default because it requires the sidecar service.
package defuddle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/sanitizer"
	"github.com/asciimoo/hister/server/types"
)

const (
	defaultEndpoint = "http://defuddle:3000/parse"
	defaultTimeout  = 15 // seconds
)

// DefuddleExtractor calls the Defuddle sidecar to extract content from any page.
type DefuddleExtractor struct {
	cfg      *config.Extractor
	endpoint string
	markdown bool
	client   *http.Client
}

// defuddleResult is the subset of the sidecar's JSON response we use.
type defuddleResult struct {
	Content     string `json:"content"`
	Title       string `json:"title"`
	Author      string `json:"author"`
	Description string `json:"description"`
	Published   string `json:"published"`
	Site        string `json:"site"`
	Image       string `json:"image"`
	WordCount   int    `json:"wordCount"`
}

func (e *DefuddleExtractor) Name() string { return "Defuddle" }

func (e *DefuddleExtractor) Description() string {
	return "Extracts main content and discussion (e.g. Reddit/HN comments) via Defuddle, the engine behind Obsidian Web Clipper, running in a Node sidecar. Runs before Readability as an improved generic extractor. Disabled by default; requires the 'defuddle' service."
}

func (e *DefuddleExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{
			Enable: false, // opt-in: requires the defuddle sidecar service
			Options: map[string]any{
				"endpoint": defaultEndpoint,
				"timeout":  defaultTimeout,
				"markdown": false,
			},
		}
	}
	return e.cfg
}

func (e *DefuddleExtractor) SetConfig(c *config.Extractor) error {
	endpoint := defaultEndpoint
	timeout := defaultTimeout
	markdown := false
	for k, v := range c.Options {
		switch k {
		case "endpoint":
			if s, ok := v.(string); ok && s != "" {
				endpoint = s
			}
		case "timeout":
			switch n := v.(type) {
			case int:
				timeout = n
			case int64:
				timeout = int(n)
			case float64:
				timeout = int(n)
			}
		case "markdown":
			if b, ok := v.(bool); ok {
				markdown = b
			}
		default:
			return fmt.Errorf("unknown option %q", k)
		}
	}
	e.cfg = c
	e.endpoint = endpoint
	e.markdown = markdown
	e.client = &http.Client{Timeout: time.Duration(timeout) * time.Second}
	return nil
}

// Match runs on every page; ordering (before Readability) makes Defuddle the
// preferred generic extractor, with Readability as the fallback via Continue.
func (e *DefuddleExtractor) Match(_ *document.Document) bool { return true }

// parse POSTs the document's rendered HTML to the sidecar and returns the result.
func (e *DefuddleExtractor) parse(d *document.Document) (*defuddleResult, error) {
	if strings.TrimSpace(d.HTML) == "" {
		return nil, fmt.Errorf("empty html")
	}
	body, err := json.Marshal(map[string]any{
		"html":     d.HTML,
		"url":      d.URL,
		"markdown": e.markdown,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := e.client
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("defuddle sidecar returned status %d", resp.StatusCode)
	}
	var res defuddleResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (e *DefuddleExtractor) Extract(d *document.Document) (types.ExtractorState, error) {
	res, err := e.parse(d)
	if err != nil {
		// Sidecar down/slow or bad input: let Readability handle it.
		return types.ExtractorContinue, err
	}
	text := sanitizer.SanitizeText(res.Content)
	if strings.TrimSpace(text) == "" {
		return types.ExtractorContinue, fmt.Errorf("defuddle returned no content")
	}
	d.Text = text
	if strings.TrimSpace(res.Title) != "" {
		d.Title = res.Title
	}
	if d.Metadata == nil {
		d.Metadata = make(map[string]any)
	}
	set := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			d.Metadata[k] = v
		}
	}
	set("author", res.Author)
	set("description", res.Description)
	set("published", res.Published)
	set("site_name", res.Site)
	set("image", res.Image)
	return types.ExtractorStop, nil
}

func (e *DefuddleExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	res, err := e.parse(d)
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}
	if strings.TrimSpace(res.Content) == "" {
		return types.PreviewResponse{}, types.ExtractorContinue, fmt.Errorf("defuddle returned no content")
	}
	return types.PreviewResponse{Content: sanitizer.SanitizeHTML(res.Content)}, types.ExtractorStop, nil
}
