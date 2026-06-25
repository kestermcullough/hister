---
date: '2026-04-07T11:00:00+00:00'
draft: false
title: 'Extractors'
---

Extractors are the components responsible for turning raw HTML or file content
into rich, searchable data. Every time a page is added to the index or a
document preview is requested, Hister runs the content through a chain of
extractors until one succeeds.

The chain design means specialist extractors run first; generic ones act as a
safety net for any content that no specialist handles.

## Purpose

Generic HTML-to-text conversion loses a lot of signal. A Stack Overflow answer,
a Go package reference, a local Markdown note, and a news article all have
different structure and a one-size-fits-all parser cannot take advantage of
that structure.

Extractors exist so that each kind of source can be handled in the most
**domain-specific** way possible. A specialist extractor for a particular
website or file format can:

- pull out the parts of the page that actually matter and discard noise (ads,
  navigation, boilerplate)
- produce richer plain text that makes search results more relevant
- surface structured details answers, code snippets, documentation sections
  that a generic parser would flatten or miss entirely
- enable to use a custom front-end template for the document preview panel,
  giving each content type its own layout and presentation

The goal is always to capture **more specialised, higher-quality information**
about the content being processed, so that search results and the document
preview are as useful as possible for the source in question.

When a page is fetched by the browser extension, the CLI, or the crawler
Hister receives its raw HTML (or file bytes). That content needs to be
processed to provide a full `Document` object.

## Extractor chain

Extractors are tried in registration order. Each call to `Extract` or `Preview`
returns an `ExtractorState` value that signals how the chain should proceed:

| State               | Meaning                                                                                                                     |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `ExtractorStop`     | The extractor handled the document successfully; stop the chain and return a successful result.                             |
| `ExtractorContinue` | The extractor was inconclusive; try the next matching extractor in the chain.                                               |
| `ExtractorAbort`    | A fatal error occurred; stop the chain immediately and propagate the error to the caller without trying further extractors. |

If no extractor returns `ExtractorStop`, `ErrNoExtractor` is returned.

## The Extractor interface

A custom extractor must implement the following Go interface (defined in
[`server/extractor/extractor.go`](https://github.com/asciimoo/hister/blob/main/server/extractor/extractor.go)):

```go
type Extractor interface {
    // Name returns a human-readable identifier used in logs and config.
    Name() string

    // Match reports whether this extractor applies to the given document.
    // Extract and Preview are only called when Match returns true.
    Match(*document.Document) bool

    // Extract rewrites the document before it is added to the index.
    // Return ExtractorStop on success, ExtractorContinue to fall through to
    // the next extractor, or ExtractorAbort to stop with a fatal error.
    Extract(*document.Document) (types.ExtractorState, error)

    // Preview returns a rendered representation suitable for display.
    // Return ExtractorStop on success, ExtractorContinue to fall through to
    // the next extractor, or ExtractorAbort to stop with a fatal error.
    Preview(*document.Document) (types.PreviewResponse, types.ExtractorState, error)

    // GetConfig returns the extractor's current configuration.
    // Must return sensible defaults before SetConfig is called.
    GetConfig() *config.Extractor

    // SetConfig applies user-supplied configuration on top of defaults.
    // Return an error for any unrecognised option key.
    SetConfig(*config.Extractor) error
}
```

### `ExtractorState`

[`types.ExtractorState`](https://github.com/asciimoo/hister/blob/main/server/types/types.go)
is defined in the `server/types` package:

```go
type ExtractorState int

const (
    ExtractorStop     ExtractorState = iota // success, stop the chain
    ExtractorContinue                       // inconclusive, try next extractor
    ExtractorAbort                          // fatal error, stop immediately
)
```

### `Document`

The whole [`document.Document`](https://github.com/asciimoo/hister/blob/main/server/document/document.go)
struct passed to `Match`, `Extract`, and `Preview`.

### `PreviewResponse`

[`types.PreviewResponse`](https://github.com/asciimoo/hister/blob/main/server/types/types.go)
carries the output of `Preview`:

```go
type PreviewResponse struct {
    Content  string // HTML or plain text to render
    Template string // optional custom front-end template name; leave blank for default
}
```

### Registering a new extractor

Add an instance of your extractor to the `extractors` slice in
[`server/extractor/extractor.go`](https://github.com/asciimoo/hister/blob/main/server/extractor/extractor.go).
Place it **before** the generic fallbacks so that it takes priority for the
pages it targets.

## Writing a new extractor

A ready-to-use starting point lives at
[`server/extractor/extractors/_extractor_template/extractor.go`](https://github.com/asciimoo/hister/blob/main/server/extractor/extractors/_extractor_template/extractor.go).
The directory begins with `_` so the Go toolchain ignores it during normal
builds, but the file itself is valid, fully-commented Go.

### Quick-start steps

1. Copy `server/extractor/extractors/_extractor_template/` to
   `server/extractor/extractors/<myname>/` (remove the leading `_`).
2. Change the `package` declaration to match the new directory name.
3. Rename `TemplateExtractor` to something descriptive (e.g. `HackerNewsExtractor`).
4. Update `matchURLPrefix` and the `Match` function for your target site.
5. Implement `Extract` to populate `d.Title`, `d.Text`, and optionally `d.Metadata`.
6. Implement `Preview` to return sanitized HTML (or return `ExtractorContinue`
   to reuse the generic readability preview).
7. Add an import and a `&MyExtractor{}` entry to the `extractors` slice in
   `server/extractor/extractor.go`, before the `&readabilityExtractor{}` line.

## Configuration

Each extractor can be enabled or disabled, and may expose custom options,
through the `extractors` section of the config file.

```yaml
extractors:
  <extractor-name>:
    enable: true | false
    options:
      key: value
```

The `<extractor-name>` key is the lowercased value returned by the extractor's
`Name()` method.

Only entries you want to change from the default need to be specified. If an
extractor is omitted from the config, its built-in defaults apply.

### `enable`

Controls whether the extractor participates in the chain.

| Value   | Effect                                              |
| ------- | --------------------------------------------------- |
| `true`  | Extractor is active (the default for all built-ins) |
| `false` | Extractor is skipped for both indexing and preview  |

### `options`

A free-form map of extractor-specific settings. The available keys depend on
the extractor implementation; each extractor validates its `options` in
`SetConfig` and returns an error for any unrecognised key.

### Implementing `GetConfig` and `SetConfig`

`GetConfig` must return the extractor's current configuration (or a default
when no config has been applied yet):

```go
func (e *MyExtractor) GetConfig() *config.Extractor {
    if e.cfg == nil {
        return &config.Extractor{
            Enable:  true,
            Options: map[string]any{},
        }
    }
    return e.cfg
}
```

`SetConfig` should validate that no unknown option keys are present, then store
the config:

```go
func (e *MyExtractor) SetConfig(c *config.Extractor) error {
    allowed := map[string]bool{"timeout": true}
    for k := range c.Options {
        if !allowed[k] {
            return fmt.Errorf("unknown option %q", k)
        }
    }
    e.cfg = c
    return nil
}
```

Config merging (default → user-supplied) is performed automatically by
`extractor.Init` before `SetConfig` is called, so `SetConfig` always receives
the fully resolved configuration.

## Built-in extractors

The extractors below are tried in the order listed. The first one that returns
`ExtractorStop` wins; the rest are skipped. Extractors that always return
`ExtractorContinue` act as metadata enrichers, they annotate the document
and then pass control to the next extractor in the chain.

### `jsonld`

Parses every `<script type="application/ld+json">` block in the page and writes
normalised [schema.org](https://schema.org) metadata to `d.Metadata`. Captures
the `@type` (content classification) and `headline` fields that the Readability
extractor does not expose.

Always returns `ExtractorContinue`, it enriches metadata but never produces
body text on its own. The `Readability` or `Default` extractor further down the
chain handles text extraction.

**Matches:** any page that contains the `application/ld+json` substring.

### `stackexchange`

Extracts Stack Exchange network question pages, including Stack Overflow,
Server Fault, Super User, Ask Ubuntu, MathOverflow, Stack Apps, and
`*.stackexchange.com` communities. Indexed text includes the question body and
all answers, with accepted answers marked.

The preview pane shows the full question body followed by each answer separated
by a horizontal rule, with accepted answers marked.

**Matches:**

- [stackoverflow.com](https://stackoverflow.com)
- [serverfault.com](https://serverfault.com)
- [superuser.com](https://superuser.com)
- [askubuntu.com](https://askubuntu.com)
- [mathoverflow.net](https://mathoverflow.net)
- [stackapps.com](https://stackapps.com)
- [stackexchange.com](https://stackexchange.com)

### `godoc`

Provides a rich preview for Go package documentation. The preview pane renders
the `Documentation-content` section of the page with relative links rewritten to
absolute `pkg.go.dev` URLs. Text extraction falls through to the `Readability`
extractor.

**Matches:** `https://pkg.go.dev/…`

### `github`

Extracts repository metadata and README content from GitHub project pages.
Produces searchable text from the repository description, star count, topics,
programming languages, and README plain text. The preview pane renders the
description summary card plus the sanitized README HTML.

**Matches:** `https://github.com/{owner}/{repo}` URLs (repository root pages
only; non-repository system paths such as `/settings`, `/topics`, and `/explore`
are excluded).

### `lobsters`

Extracts the full content of a lobste.rs submission, including the story
metadata (title, author, tags, submission date), the optional story body, and
the complete nested comment tree. Both indexed text and preview preserve the
parent–child comment hierarchy.

**Matches:** `https://lobste.rs/s/…`

### `wikipedia`

Extracts article content from Wikipedia. Indexed text includes the article
title, infobox key–value pairs, and the body text with navigation boxes,
references, and other noise removed. The preview renders the article HTML with
inline styles applied, videos replaced by their poster frames, and relative URLs
rewritten to absolute Wikipedia URLs.

**Matches:** `https://*.wikipedia.org/wiki/…` (article pages only; non-content
namespaces such as `Special:`, `Talk:`, `User:`, `File:`, and `Category:` are
excluded).

### `mastodon`

Handles Mastodon instance pages by splitting them into individual toot documents.
Each toot found on the page is indexed as a separate document with its own URL
and author, allowing individual posts to appear in search results. The original
aggregator page is not indexed.

Detection is heuristic: the extractor checks for a `"repository":"mastodon/mastodon"`
marker in the page HTML, or for a `type: toot` metadata flag set by a previous
pass.

**Matches:** any Mastodon instance page containing the Mastodon source marker.

### `ytdlp`

Extracts video metadata from video-hosting sites (YouTube, Vimeo, Twitch, etc.)
using the [`yt-dlp`](https://github.com/yt-dlp/yt-dlp) command-line tool.
Provides a dedicated video preview template that shows the thumbnail, duration,
uploader, description, chapter list, and optional transcript.

The extractor is **disabled by default** because it requires `yt-dlp` to be
installed separately.

**Matches:** a curated list of video-hosting domains (YouTube, Vimeo, Twitch,
Dailymotion, Bilibili, and others), as well as any hostname containing common
video-platform substrings.

#### Options

| Option                 | Type            | Default  | Description                                                                                                                                                                                          |
| ---------------------- | --------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `binary`               | string          | `yt-dlp` | Path to the `yt-dlp` executable. Useful when the binary is not on `PATH` or you want to pin a specific version.                                                                                      |
| `timeout`              | int             | `15`     | Seconds to wait for `yt-dlp` to finish before aborting the request.                                                                                                                                  |
| `fetch_subtitles`      | bool            | `false`  | Download and append subtitles/auto-generated transcript to the indexed text and preview.                                                                                                             |
| `sub_language`         | string          | `auto`   | Subtitle language code to request (e.g. `en`, `de`, `fr`). `auto` selects auto-generated captions. Only used when `fetch_subtitles` is `true`.                                                       |
| `cookies_file`         | string          | -        | Path to a [Netscape-format cookies file](https://curl.se/docs/http-cookies.html). Passed to `yt-dlp` as `--cookies`. Useful for age-restricted or members-only content.                              |
| `cookies_from_browser` | string          | -        | Browser name from which `yt-dlp` should extract cookies at runtime (e.g. `chrome`, `firefox`, `safari`). Passed as `--cookies-from-browser`. Takes precedence over `cookies_file` when both are set. |
| `extra_args`           | list of strings | -        | Additional `yt-dlp` CLI flags appended verbatim to every invocation.                                                                                                                                 |

#### Example configuration

```yaml
extractors:
  ytdlp:
    enable: true
    options:
      binary: /usr/local/bin/yt-dlp
      timeout: 30
      fetch_subtitles: true
      sub_language: en
      cookies_from_browser: firefox
      extra_args:
        - --proxy
        - socks5://127.0.0.1:1080
```

### `readability`

Generic article extractor using the
[go-readability](https://codeberg.org/readeck/go-readability) library. Strips
navigation, ads, sidebars, and other boilerplate and returns the main article
content as clean plain text and HTML. Also extracts author, publication date,
description, site name, and canonical image from JSON-LD, OpenGraph, and meta
tags.

**Matches:** every page. Acts as the primary fallback for all content that no
specialist extractor handles.

### `default`

Ultimate fallback. Walks the raw HTML token stream and collects all visible text
inside `<body>`, discarding `<script>`, `<style>`, and `<noscript>` elements.
Produces plain text with no further processing.

**Matches:** every page. Only reached when `Readability` fails or is disabled.

## Development guidelines

**Avoid additional HTTP requests.** Work with the HTML and metadata already
available in the `Document` struct wherever possible. Making extra requests
inside an extractor adds latency, increases network traffic, and can fail
silently in offline or restricted environments. More importantly, outbound
requests expose the user's IP address and browsing activity to external servers,
which is a privacy concern. Additional requests are not forbidden, but they
should only be made when there is no reasonable alternative.

**Avoid embedding third-party content.** Strip or discard remote images, videos,
iframes, and other externally hosted media before returning content from
`Extract` or `Preview` wherever possible. Embedding such content causes the
browser to contact third-party servers whenever a preview is opened, leaking
the user's IP address without their knowledge. Third-party content is not
forbidden, but it should be avoided unless it is essential to the extractor's
purpose. When multimedia must be surfaced, the preferred approach is to render
a placeholder button that the user can click to load the video, audio, or embed
on demand, so external contact only happens with explicit user intent.

**Use custom preview templates when they add value.** If the extracted content
has a well-defined structure (code documentation, Q&amp;A threads, recipes, and
so on), return a non-empty `Template` in `PreviewResponse` and build a
dedicated Svelte template for it. A tailored layout is almost always more
readable than the generic one.
