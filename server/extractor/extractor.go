// Package extractor provides HTML content extraction for documents.
package extractor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"

	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/document"
	"github.com/asciimoo/hister/server/extractor/extractors/embeddedvideo"
	"github.com/asciimoo/hister/server/extractor/extractors/github"
	"github.com/asciimoo/hister/server/extractor/extractors/godoc"
	"github.com/asciimoo/hister/server/extractor/extractors/jsonld"
	"github.com/asciimoo/hister/server/extractor/extractors/lobsters"
	"github.com/asciimoo/hister/server/extractor/extractors/markdown"
	"github.com/asciimoo/hister/server/extractor/extractors/mastodon"
	"github.com/asciimoo/hister/server/extractor/extractors/notion"
	"github.com/asciimoo/hister/server/extractor/extractors/stackexchange"
	"github.com/asciimoo/hister/server/extractor/extractors/wikipedia"
	"github.com/asciimoo/hister/server/extractor/extractors/ytdlp"
	"github.com/asciimoo/hister/server/types"
)

// Extractor extracts content from a Document.
type Extractor interface {
	// Name returns a human-readable identifier for the extractor.
	Name() string

	// Description returns a short human-readable summary of what the extractor does.
	Description() string

	// Match reports whether this extractor is applicable to the given document.
	// Extract and Preview will only be called when Match returns true.
	Match(*document.Document) bool

	// Extract rewrites documents before the documents are added to the index.
	// The returned ExtractorState signals how the chain should proceed:
	// ExtractorStop means success, ExtractorContinue means try the next extractor,
	// ExtractorAbort means stop immediately and return the error.
	Extract(*document.Document) (types.ExtractorState, error)

	// Preview returns a rendered representation of the document suitable for
	// display (e.g. readable HTML or plain text).
	// The returned ExtractorState signals how the chain should proceed:
	// ExtractorStop means success, ExtractorContinue means try the next extractor,
	// ExtractorAbort means stop immediately and return the error.
	Preview(*document.Document) (types.PreviewResponse, types.ExtractorState, error)

	// GetConfig returns the extractor's current configuration. Before
	// SetConfig is called, implementations must return their default config.
	GetConfig() *config.Extractor

	// SetConfig applies cfg to the extractor, overwriting defaults.
	// Implementations should return an error for unrecognised option keys.
	SetConfig(*config.Extractor) error
}

// ErrNoExtractor is returned when no extractor can handle the document.
var ErrNoExtractor = errors.New("no extractor found")

// ErrExtractorAbort is returned when an extractor signals ExtractorAbort.
var ErrExtractorAbort = errors.New("extractor aborted")

// ExtractorInfo holds a summary of an extractor's identity and current state.
type ExtractorInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Enabled     bool           `json:"enabled"`
	Options     map[string]any `json:"options,omitempty"`
}

// ListMatching returns an ExtractorInfo entry for every enabled extractor that
// matches d, in chain order. Options is omitted so the result is safe to send
// to clients.
func ListMatching(d *document.Document) []ExtractorInfo {
	infos := make([]ExtractorInfo, 0)
	for _, e := range extractors {
		if !e.GetConfig().Enable {
			continue
		}
		if e.Match(d) {
			infos = append(infos, ExtractorInfo{
				Name:        e.Name(),
				Description: e.Description(),
				Enabled:     true,
			})
		}
	}
	return infos
}

// ListEnabled returns an ExtractorInfo entry for every enabled extractor in
// chain order. Options is omitted so the result is safe to send to clients.
func ListEnabled() []ExtractorInfo {
	infos := make([]ExtractorInfo, 0, len(extractors))
	for _, e := range extractors {
		if !e.GetConfig().Enable {
			continue
		}
		infos = append(infos, ExtractorInfo{
			Name:        e.Name(),
			Description: e.Description(),
			Enabled:     true,
		})
	}
	return infos
}

// List returns an ExtractorInfo entry for every registered extractor in chain
// order. Options is always populated; callers that should not expose
// configuration must clear or omit it before sending to clients.
func List() []ExtractorInfo {
	infos := make([]ExtractorInfo, 0, len(extractors))
	for _, e := range extractors {
		cfg := e.GetConfig()
		infos = append(infos, ExtractorInfo{
			Name:        e.Name(),
			Description: e.Description(),
			Enabled:     cfg.Enable,
			Options:     cfg.Options,
		})
	}
	return infos
}

var extractors = []Extractor{
	&markdown.MarkdownExtractor{},
	&embeddedvideo.EmbeddedVideoExtractor{},
	&jsonld.JSONLDExtractor{},
	&stackexchange.StackExchangeExtractor{},
	&godoc.GoDocExtractor{},
	&github.GitHubExtractor{},
	&lobsters.LobstersExtractor{},
	&wikipedia.WikipediaExtractor{},
	&mastodon.MastodonExtractor{},
	&notion.NotionExtractor{},
	&ytdlp.YtdlpExtractor{},
	&readabilityExtractor{},
	&basicExtractor{},
}

// Init applies user-supplied extractor configurations on top of each
// extractor's defaults. It must be called before Extract or Preview.
// cfgs is keyed by lowercased extractor name (as Viper lowercases YAML keys).
func Init(cfgs map[string]*config.Extractor) error {
	for _, e := range extractors {
		def := e.GetConfig()
		merged := &config.Extractor{
			Enable:  def.Enable,
			Options: make(map[string]any, len(def.Options)),
		}
		maps.Copy(merged.Options, def.Options)
		if user, ok := cfgs[strings.ToLower(e.Name())]; ok && user != nil {
			merged.Enable = user.Enable
			maps.Copy(merged.Options, user.Options)
		}
		if err := e.SetConfig(merged); err != nil {
			return fmt.Errorf("extractor %s: %w", e.Name(), err)
		}
	}
	return nil
}

// Extract tries each registered extractor in order and returns the first
// successful result. Returns ErrNoExtractor if none succeed.
func Extract(d *document.Document) error {
	for _, e := range extractors {
		if !e.GetConfig().Enable {
			continue
		}
		if e.Match(d) {
			state, err := e.Extract(d)
			log.Debug().Str("URL", d.URL).Str("Extractor", e.Name()).Msg("Extracting data")
			switch state {
			case types.ExtractorStop:
				return nil
			case types.ExtractorAbort:
				return fmt.Errorf("extractor %s: %w: %w", e.Name(), ErrExtractorAbort, err)
			default:
				if err != nil {
					log.Warn().Err(err).Str("URL", d.URL).Str("Extractor", e.Name()).Msg("Failed to extract content")
				}
			}
		}
	}
	return ErrNoExtractor
}

// Preview returns a rendered preview of the document. When name is empty the
// first matching enabled extractor in the chain is used. When name is
// non-empty the extractor with that name (case-insensitive) is used directly,
// bypassing Match() and the enabled check. ErrNoExtractor is returned when
// name is non-empty but not found.
func Preview(d *document.Document, name string) (types.PreviewResponse, error) {
	if name != "" {
		lower := strings.ToLower(name)
		for _, e := range extractors {
			if strings.ToLower(e.Name()) == lower {
				log.Debug().Str("URL", d.URL).Str("Extractor", e.Name()).Msg("Creating preview with explicit extractor")
				resp, state, err := e.Preview(d)
				if state == types.ExtractorAbort {
					return types.PreviewResponse{}, fmt.Errorf("extractor %s: %w", e.Name(), err)
				}
				return resp, nil
			}
		}
		return types.PreviewResponse{}, fmt.Errorf("%w: %s", ErrNoExtractor, name)
	}
	for _, e := range extractors {
		if !e.GetConfig().Enable {
			continue
		}
		if e.Match(d) {
			log.Debug().Str("URL", d.URL).Str("Extractor", e.Name()).Msg("Creating preview")
			resp, state, err := e.Preview(d)
			switch state {
			case types.ExtractorStop:
				return resp, nil
			case types.ExtractorAbort:
				return types.PreviewResponse{}, fmt.Errorf("extractor %s: %w", e.Name(), err)
			default:
				if err != nil {
					log.Warn().Err(err).Str("URL", d.URL).Str("Extractor", e.Name()).Msg("Failed to preview content")
				}
			}
		}
	}
	return types.PreviewResponse{}, ErrNoExtractor
}

type basicExtractor struct {
	cfg *config.Extractor
}

type readabilityExtractor struct {
	cfg *config.Extractor
}

func (e *basicExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{Enable: true, Options: map[string]any{}}
	}
	return e.cfg
}

func (e *basicExtractor) SetConfig(c *config.Extractor) error {
	for k := range c.Options {
		return fmt.Errorf("unknown option %q", k)
	}
	e.cfg = c
	return nil
}

func (e *readabilityExtractor) GetConfig() *config.Extractor {
	if e.cfg == nil {
		return &config.Extractor{Enable: true, Options: map[string]any{}}
	}
	return e.cfg
}

func (e *readabilityExtractor) SetConfig(c *config.Extractor) error {
	for k := range c.Options {
		return fmt.Errorf("unknown option %q", k)
	}
	e.cfg = c
	return nil
}

func (e *basicExtractor) Name() string {
	return "Basic"
}

func (e *basicExtractor) Description() string {
	return "Fallback extractor that strips HTML tags and extracts plain text from any web page."
}

func (e *basicExtractor) Match(_ *document.Document) bool {
	return true
}

func (e *basicExtractor) Extract(d *document.Document) (types.ExtractorState, error) {
	d.Title = ""
	r := strings.NewReader(d.HTML)
	doc := html.NewTokenizer(r)
	inBody := false
	skip := false
	var text strings.Builder
	var currentTag string
out:
	for {
		tt := doc.Next()
		switch tt {
		case html.ErrorToken:
			err := doc.Err()
			if errors.Is(err, io.EOF) {
				break out
			}
			return types.ExtractorStop, errors.New("failed to parse html: " + err.Error())
		case html.SelfClosingTagToken, html.StartTagToken:
			tn, _ := doc.TagName()
			currentTag = string(tn)
			switch currentTag {
			case "body":
				inBody = true
			case "script", "style", "noscript":
				skip = true
			}
		case html.TextToken:
			if currentTag == "title" {
				d.Title += strings.TrimSpace(string(doc.Text()))
			}
			if inBody && !skip {
				text.Write(doc.Text())
			}
		case html.EndTagToken:
			tn, _ := doc.TagName()
			switch string(tn) {
			case "body":
				inBody = false
			case "script", "style", "noscript":
				skip = false
			}
		}
	}
	d.Text = strings.TrimSpace(text.String())
	if d.Text == "" && d.Title == "" {
		return types.ExtractorStop, errors.New("no content found")
	}
	return types.ExtractorStop, nil
}

func (e *basicExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	return types.PreviewResponse{Content: d.Text}, types.ExtractorStop, nil
}

func (e *readabilityExtractor) Name() string {
	return "Readability"
}

func (e *readabilityExtractor) Description() string {
	return "Extracts the main article content from any web page using the go-readability library, filtering out navigation, ads, and other boilerplate."
}

func (e *readabilityExtractor) Match(_ *document.Document) bool {
	return true
}

func (e *readabilityExtractor) Extract(d *document.Document) (types.ExtractorState, error) {
	r := strings.NewReader(d.HTML)

	u, err := url.Parse(d.URL)
	if err != nil {
		return types.ExtractorStop, err
	}
	a, err := readability.FromReader(r, u)
	if err != nil {
		return types.ExtractorContinue, err
	}
	buf := bytes.NewBuffer(nil)
	if err := a.RenderText(buf); err != nil {
		return types.ExtractorContinue, err
	}
	d.Text = buf.String()
	d.Title = a.Title()
	d.SetFaviconURL(a.Favicon())
	writeReadabilityMeta(d, a)
	return types.ExtractorStop, nil
}

// writeReadabilityMeta copies the rich fields readability already parsed
// (internally from JSON-LD, OpenGraph, and meta tags) onto d.Metadata so
// downstream consumers have byline/date/description without re-parsing.
// The JSON-LD extractor only writes type and headline, so these keys do
// not collide.
func writeReadabilityMeta(d *document.Document, a readability.Article) {
	if d.Metadata == nil {
		d.Metadata = make(map[string]any)
	}
	set := func(k, v string) {
		if v != "" {
			d.Metadata[k] = v
		}
	}
	set("author", a.Byline())
	set("description", a.Excerpt())
	set("site_name", a.SiteName())
	set("image", a.ImageURL())
	set("language", a.Language())
	if t, err := a.PublishedTime(); err == nil && !t.IsZero() {
		d.Metadata["published"] = t.Format(time.RFC3339)
	}
	if t, err := a.ModifiedTime(); err == nil && !t.IsZero() {
		d.Metadata["modified"] = t.Format(time.RFC3339)
	}
}

func (e *readabilityExtractor) Preview(d *document.Document) (types.PreviewResponse, types.ExtractorState, error) {
	r := strings.NewReader(d.HTML)
	u, err := url.Parse(d.URL)
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorStop, err
	}
	a, err := readability.FromReader(r, u)
	if err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}
	var htmlContent strings.Builder
	if err := a.RenderHTML(&htmlContent); err != nil {
		return types.PreviewResponse{}, types.ExtractorContinue, err
	}
	return types.PreviewResponse{Content: htmlContent.String()}, types.ExtractorStop, nil
}
