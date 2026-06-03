# Changelog

## v0.15.0

### New Features

#### Document Versioning

A new `versioning` rule type instructs Hister to track changes to a document each
time it is re-indexed. A diff-style changelog appears inside the preview panel so
you can see what changed between versions, and the preview endpoint returns the
full version history. Combine with a priority or skip rule to version only the
pages that matter to you.

#### Priority Rules

Priority rules have been reintroduced (closes #222). Documents whose URLs match a
priority rule are pinned to the top of search results regardless of their relevance
score. Rules can be created, edited, and sorted directly in the web UI.

#### PDF Indexing

Local PDF files can now be indexed with full text extraction. The browser extension
gains a new endpoint for sending PDFs directly to the server (closes #55). Indexed
PDFs are stored as a distinct document type and can be filtered with
`metadata.type:pdf` in search queries.

#### Faceted Filtering

Search results can now be filtered through facets that group documents by common
fields such as language, and domain. Facet counts update live as you refine your
query. Date-range filters have been moved into the same filters dropdown for a
unified experience, and a "load more" control expands facets with many values.

#### Embedded Video Extractor

A new extractor detects embedded videos (YouTube, Vimeo, and similar platforms)
on indexed pages and stores the embedding metadata as a dedicated document type.
Embedded videos are rendered directly inside the result preview panel, so you can
watch them without leaving Hister (closes #446).

#### Notion Extractor

A dedicated extractor for Notion pages extracts article content cleanly from
public Notion URLs, removing navigation chrome and other non-content elements.

#### Compressed HTML and Favicon Storage

HTML content and favicons are now stored gzip-compressed in separate files on
disk rather than inline inside the Bleve index. This significantly reduces index
size on disk and lowers memory pressure during search (closes #384).

#### Disable HTML Storage

A new config option lets you turn off full HTML storage and preview generation
entirely. Disabling storage trades preview functionality for a smaller footprint,
useful for bulk or headless indexing workflows (closes #440).

#### User-Specific Directory Indexing

The `indexer.directories` config now accepts a `user` field per directory entry.
Files under that path are indexed only for the specified user, making it easy to
share a single Hister instance while keeping personal file indexes private.

#### Label Editing from Results

Document labels can be edited inline directly from search result cards without
navigating away. The browser extension can also apply a one-off label to a
document at index time (closes #407). Updating a document no longer clears
previously assigned labels.

#### File Deletion Tracking

When a locally-watched file is deleted from the filesystem, Hister now
automatically removes it from the index (closes #230). Batch and single-document
deletions also clean up the associated HTML and favicon files from disk.

#### MCP Document Preview Endpoint

A new MCP endpoint exposes document previews to LLM agents and MCP-compatible
tools, complementing the existing MCP search endpoint.

#### Browser Import: Ladybird Support

The `import-browser` command now supports importing history from the
[Ladybird](https://ladybird.org/) browser.

#### Browser Import: Auto-Detect Database

The `import-browser` command now automatically detects the browser database file
path, so passing an explicit path is no longer required for supported browsers.

### Enhancements

- **Copy URL button**: a copy-to-clipboard icon appears next to each result URL
- **Preview extractor selector**: switch between available extractors when viewing
  a document preview without re-indexing
- **Rules table**: columns are sortable; values are filterable; filter toggle
  buttons are visually distinct from column headers; regexp validation runs before
  saving a rule
- **CLI index flags**: `--delay`, `--timeout`, and `--user-agent` are now
  available directly on `hister index` without needing a config file
- **Configurable client timeout**: the HTTP client timeout used during indexing
  is configurable (fixes #429)
- **Standardized config file lookup**: config file discovery follows a consistent
  search order across all platforms (closes #424)
- **History autoscroll**: the history view scrolls to keep the selected entry
  visible (#427)
- **Admin profile version**: the admin profile page now shows the running Hister
  version (closes #409)
- **Log level aliases**: common short aliases are accepted for log level values
  (fixes #411)
- **Random tips**: a rotating set of usage tips is shown on the front page when
  navigating back from search results
- **yt-dlp diagnostics**: clearer error messages and debug logging help diagnose
  yt-dlp configuration problems
- **Batch crawl insertion**: URLs discovered during a recursive crawl are
  inserted in batches for better throughput
- **Extension popup header**: the popup header is now a link to the configured
  Hister server
- **Per-sub-index paging**: each language sub-index is iterated independently to
  prevent paging gaps in multi-language setups
- **Improved embedding handling**: fallow and embedding request processing
  is more robust

### Bug Fixes

- Docker `BASE_URL` environment variable can now be correctly overridden by a
  config file (closes #442)
- Search results now contain all queried terms rather than any of them
- HTML is no longer re-written to the indexer during a reindex run
- yt-dlp subtitle download no longer skipped when the sub language differs from
  the original language (#429)
- History page stops loading more entries when the last page has been reached
- "Show all" in history now scrolls back to the top
- Rule type filter no longer incorrectly excludes rules
- Debug-level init messages are correctly suppressed at higher log levels
- Result paging uses a valid sort key, fixing out-of-order pages
- Times are displayed in the browser's configured timezone

## v0.14.0

### New Features

#### Mastodon Extractor

A dedicated extractor for Mastodon detects Mastodon pages and indexes each
toot as its own separate document rather than one big blob of text. Works
with any Mastodon instance without configuration. Every toot gets a
`metadata.type:toot` field so you can filter toots in search queries.
Combined with a search alias (`!toot → metadata.type:toot`) this makes
finding past toots fast and convenient.

#### Metadata Query Filtering

Documents can now be filtered by arbitrary metadata fields using the
`metadata.key:value` query syntax. Extractors (including the new Mastodon
extractor) populate these fields at index time.

#### Full Screen Preview

The split-screen preview can now be toggled into a full-screen mode that
occupies the entire content area (closes #401). The URL changes to
`/preview/[id]` so the view survives a page reload. Pressing the "view
result content" hotkey switches between split and full-screen. Full-screen
preview is also available on the history page.

#### Preview Panel on History Page

The history page now shows the same interactive preview panel as the search
page (closes #395). Hotkey navigation between history entries works the same
way as on the search page.

#### Infinite Scroll

Search results now load more pages automatically as you scroll to the bottom
of the list, removing the need to manually page through results (closes #1).

#### Image Lightbox

Images displayed inside the preview panel can be clicked to open a full-size
lightbox view.

#### Delete All Results

A new "delete all" action removes every document that matches the current
search query at once, without having to delete results one by one.

#### Keyboard Shortcut for Result Deletion

A dedicated hotkey deletes the currently focused search result directly from
the keyboard.

#### Quick Skip Rule

A skip-rule component lets you add a URL to your skip list directly from a
search result, available in both the web app and the browser extension
(closes #380).

#### robots.txt Support

The crawler and `hister index` now respect `robots.txt` by default (closes
#386). A new `ignore_robots_txt` config option disables this check when
needed.

#### Document Labeling

Documents can be tagged with custom labels when indexing from the CLI
(`--label`) or from the browser extension. Labels are stored as metadata
and can be used in search queries (closes #156, #373).

#### Editable Rules and Aliases

Existing skip/priority rules and search aliases can now be edited in the
web UI instead of having to delete and recreate them (closes #270).

#### Exact Phrase Matching

Multi-word queries now also attempt an exact phrase match across title and
text, so searching for `open source` surfaces pages that contain that exact
phrase more prominently (closes #394).

#### Extractor Templates and Extra Document Creation

Extractors can now supply a custom preview template, giving each content
type its own presentation in the preview panel. Extractors can also produce
additional sub-documents from a single page (used by the Mastodon extractor
to create one document per toot). A template scaffold is included to make
writing new extractors easier.

#### WebDriver BiDi Crawler Backend

A new `bidi` crawler backend uses the W3C [WebDriver BiDi](https://w3c.github.io/webdriver-bidi/)
protocol to drive an **already-running** browser over a WebSocket connection.
Unlike the `chromedp` backend, it does not launch a browser process: you start
the browser yourself (headless or not) and point Hister at it:

```bash
# Firefox
firefox --remote-debugging-port 9222

# Chrome / Chromium
chromium --remote-debugging-port=9222
```

```yaml
crawler:
  backend: bidi
  backend_options:
    host: '127.0.0.1'
    port: '9222'
    capture_delay: 1.5 # extra seconds to wait after load for JS rendering
```

Supported by Firefox (≥ 102), Chrome/Chromium (≥ 106), and Edge. Options:
`socket` (full WebSocket URL), `host`, `port`, `capture_delay`. The crawler
reuses a single BiDi session for all URLs in one `hister index` run, making
multi-URL indexing significantly more efficient than opening a new browser
session per URL (closes #284).

### Enhancements

- **Resizable preview panel**: drag the divider in split-screen view to
  adjust the panel width; the chosen width persists across sessions
- **History hotkey navigation**: keyboard navigation between history entries
  on the history page
- **Secondary date sorting**: when search scores are equal, results are
  sorted by indexed date
- **yt-dlp multi-language subtitles**: configure which subtitle languages
  the yt-dlp extractor indexes
- **MCP date filtering**: MCP search requests can now be filtered by date
- **Semantic search chunking**: punctuation-based boundaries used for
  chunk splitting, improving relevance for sentence-level queries
- **OIDC enhancements**: userinfo endpoint is now configurable for
  providers without auto-discovery (#279); password login can be disabled
  when OAuth is the only configured auth method
- **Clearer CLI error messages**: client-side HTTP errors now explain the
  problem in plain words and suggest which flag to use (#400)
- **Duplicate rule prevention**: the server and UI both reject duplicate
  skip/priority rules and aliases (#399)
- **Deletion error feedback**: errors during document deletion are now
  surfaced in the UI
- **Rules/aliases UX**: input fields moved above their respective lists
- **Version string in server log**: the server start message now includes
  the version number (#372)
- **Silent WebSocket disconnect**: closing the browser tab no longer shows
  a connection-error message

### Bug Fixes

- Search terms are now properly escaped before query execution
- Focused result index computed correctly when priority results are present
- Phrase queries are only applied when no field-specific terms exist in the
  query
- `/api/delete` requests from the browser extension are now accepted
- `text` and `html` fields always included in search results (#374)
- Language field included in document search results
- OIDC scopes correctly forwarded to the provider (#371)
- Auth page is scrollable on small screens (#370)
- TUI client now passes the access token to the server (#368)
- Optional peer dependencies no longer excluded on non-Linux platforms (#299)

## v0.13.0

### New Features

#### Semantic Search

Full vector search support via sentence-embedding models. Documents are chunked
and embedded at index time; search queries are embedded at query time and ranked
by cosine similarity. Two storage backends are supported:

- **SQLite** (default, via bundled `sqlite-vec`) zero extra infrastructure required
- **PostgreSQL** with `pgvector` auto-selected when the database is Postgres

Configure the embedding API endpoint, model, dimensions, and chunking parameters
in the new `semantic` config section. Semantic search is opt-in and off by default.
Relevance scores are shown alongside results when semantic search is active.

#### OAuth / SSO Authentication

OAuth 2.0 and OpenID Connect (OIDC) providers can now be configured as login
methods. Add one or more entries to the new `server.oauth` config section with
`client_id`, `client_secret`, `configuration_url` (for OIDC auto-discovery), or
manual `auth_url` / `token_url`, and optional `scopes`. Multiple providers can be
active at the same time alongside the built-in username/password login.

#### MCP Server

Hister now exposes a [Model Context Protocol](https://modelcontextprotocol.io/)
endpoint at `/api/mcp`, enabling LLM agents and MCP-compatible tools to search
the index directly.

#### Persistent Crawler State Management

Recursive crawl jobs (`hister index -r`) are now stored in the database and
survive interruptions. Each job gets a unique ID (auto-generated or set via
`--job-id`). Pass `--job-id <id>` without `--recursive` to resume an
interrupted crawl from exactly where it left off, including original validator
rules and visited-URL counts.

#### New Extractors

- **Wikipedia** extracts the article body and infobox, rewrites relative links, and sanitizes the output
- **GitHub project** extracts repository descriptions and README content from GitHub project pages
- **Lobste.rs** dedicated extractor for Lobste.rs story and comment pages
- **yt-dlp** extracts video metadata (title, description, channel) from video pages via yt-dlp
- **JSON-LD** surfaces structured metadata (`@type`, `headline`, description) from pages that embed JSON-LD

All extractors now expose a `Description()` method, and an extractor information
page is available at `/extractors` in the web UI.

#### OpenSearch Suggestions

The server now serves an OpenSearch suggestions endpoint (`/api/suggest`),
allowing browsers to display search-as-you-type completions when Hister is
configured as a search engine.

### Enhancements

#### Crawler Backend for All Index Operations

The `--backend` flag (and `--backend-option`) is now available on both
`hister index` (plain and `--recursive`) and `hister import-browser`, allowing
a headless Chrome/Chromium backend for JavaScript-heavy pages without running a
full recursive crawl:

```bash
hister index --backend chromedp https://example.com
hister import-browser --backend chromedp --backend-option exec_path=/usr/bin/chromium
```

Headers and cookies can also be injected per-invocation:

```bash
hister index --header "Accept-Language=en" --cookie "session=abc; Domain=example.com" https://example.com
```

Cookies use standard `Set-Cookie` format with a required `Domain` attribute.

#### CLI Search Improvements

- `--limit N` flag caps the number of results returned
- `--fields` flag selects which document fields to include in output
- `--html` flag includes raw HTML content in the output
- Paging support added to both CLI search and `list-urls`
- `list-urls` now fetches results from the server by default; `--offline` connects directly to the index without a running server

#### Quoted Field Queries

Field-qualified queries now support quoted values, enabling correct deletion and
lookup of URLs that contain spaces (common on Windows file paths):

```
url:"file:///C:/Users/My Documents/notes.txt"
```

#### Preview Panel Polish

- Preview title is now clickable (opens the result URL)
- Preview panel maximises available content width
- JSON-LD metadata surfaced inside the preview panel
- Dark theme font colors fixed in preview popup

#### NixOS / Nix Module

- `systemd` and `launchd` hardening applied to the Hister service units
- New `services.hister.environmentFile` option for secrets injection
- `openFirewall` now requires explicit opt-in
- `services.hister.config` renamed to `services.hister.settings`

#### Other

- Executable size reduced ~70 MB by switching to a trimmed `lingua-go` fork
- Sensitive content rejection errors surfaced in the browser extension
- `--verbose` flag on `hister delete` lists matched URLs before deleting
- Priority result deduplication now copies body text from the original result
- `/suggest` endpoint protected by auth middleware and `Sec-Fetch-Site` header check
- Version information included in the MCP endpoint response
- Timezone data bundled into the binary for environments without a system `tzdata`

### Bug Fixes

- File URLs (`file://`) now handled correctly in the UI for both opening and deletion (#362)
- Browser extension authentication documentation corrected (#366)
- URLs no longer lowercased during query building, preventing mismatches on case-sensitive paths
- History view correctly filtered per-user in multi-user mode (#314)
- Token authentication middleware now respects `NoAuth` flag (#348)
- Documents with no HTML content no longer attempt HTML extraction (#351)
- Extension no longer resubmits documents after a `406 Not Acceptable` response
- Priority results correctly deduplicated against standard results
- File indexing fixed on Windows
- Wide tables no longer overflow the preview panel
- Score field populated correctly in search responses
- `aws_access_key` sensitive content pattern tightened to reduce false positives
- Home-manager service units correctly gated on host platform in Nix module

## v0.12.0

### New Features

#### Web Crawler

New `hister index -r <url>` command crawls sites recursively using BFS traversal.
Supports an HTTP backend and a headless Chrome backend (chromedp).
Configurable depth, link count, allowed/excluded domains, and URL patterns.

#### PostgreSQL Backend

Full PostgreSQL support as an alternative to SQLite, including pgvector for semantic search.
Configure via a `postgres://` connection string in `server.database`.

#### Extractor Pipeline Overhaul

Extractors are now configurable, have explicit states (continue/done), and expose
a `Preview()` method used by the readability panel. New extractors included:

- Custom `pkg.go.dev` extractor for Go documentation pages
- Basic Stack Overflow extractor

#### Desktop Readability Panel

Focused search results load automatically in a split-pane reader on the right side
on screens wider than 1280 px. The panel is togglable and its open/closed state persists.

### Enhancements

- HTML sanitizer (bluemonday) applied to all extracted content
- `metadata` field added to documents for arbitrary key/value data
- `search` input type attribute on search fields for better mobile UX
- Build commit ID shown in the version string
- Admin users can create global indexes or indexes on behalf of other users
- `hister index` skips already-indexed URLs by default; pass `--force` to reindex them
- URL and domain wildcard matching automatically anchors to start and end
- Table of contents added to the API docs page
- Document indexed date shown in the preview panel
- Search query reflected in the browser tab title
- WebSocket communication optimised to reduce redundant round-trips
- Automatic redirect on zero results is now optional (configurable)
- `import` command renamed to `import-browser` to free `import` for index import/export

### Bug Fixes

- Browser history database opened read-only to avoid lock conflicts (#304)
- History entries now deleted when their associated document is deleted (#303)
- Crawler user-agent correctly applied after redirect handling (#302)
- Fixed field-specific alternation parts in query parser (#274)
- Negated query terms no longer trimmed twice
- HTML field no longer leaks into search results (#268)
- Expanded query hint only shown when the expansion is longer than the original query
- URL changes after HTTP redirects now resolved correctly
- Crawler no longer stops on HTTP errors
- Crawler timeout now applied during browser history import (#278)
- Pinned result titles no longer truncated on narrow screens
- Dark mode handled correctly in the preview panel
- Mobile layout no longer introduces unwanted line breaks
