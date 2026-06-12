---
date: '2026-02-25T00:00:00+00:00'
draft: false
title: 'Configuration Reference'
---

Hister is configured via a YAML file. The config file is searched in the following order:

### Default Config Locations

**Linux** (respects `$XDG_CONFIG_HOME`):

- `$XDG_CONFIG_HOME/hister/config.yml` (if `$XDG_CONFIG_HOME` is set)
- `~/.config/hister/config.yml` (default)
- `~/.histerrc` (legacy, deprecated)

**macOS** (respects `$XDG_CONFIG_HOME`):

- `$XDG_CONFIG_HOME/hister/config.yml` (if `$XDG_CONFIG_HOME` is set)
- `~/Library/Preferences/hister/config.yml` (recommended)
- `~/Library/Application Support/hister/config.yml` (backwards compatible)
- `~/.histerrc` (legacy, deprecated)
- `~/.config/hister/config.yml` (legacy)

**Windows** (respects environment variables):

- `%LOCALAPPDATA%\hister\config.yml` (recommended)
- `$XDG_CONFIG_HOME\hister\config.yml` (if `$XDG_CONFIG_HOME` is set)
- `%APPDATA%\hister\config.yml` (fallback)
- `~\.histerrc` (legacy, deprecated)
- `~\.config\hister\config.yml` (legacy)

Hister searches these paths in order and uses the first config file it finds. If you have a legacy `~/.histerrc` file and Hister finds it, you'll see a deprecation warning suggesting you move it to the recommended location.

### Creating a Config File

Generate a config file with default values:

```bash
hister create-config ~/.config/hister/config.yml
```

Or use your platform's recommended location:

```bash
# Linux
hister create-config ~/.config/hister/config.yml

# macOS
hister create-config ~/Library/Preferences/hister/config.yml

# Windows
hister create-config %LOCALAPPDATA%\hister\config.yml
```

You can also specify a custom config location using the `--config` flag:

```bash
hister listen --config /path/to/my/config.yml
```

**Important**: Restart the Hister server after modifying the configuration file.

## Full Example

```yaml
app:
  directory: '~/.config/hister'
  search_url: 'https://google.com/search?q={query}'
  log_level: 'info'
  log_format: 'text'
  log_file: ''
  open_results_on_new_tab: false

server:
  address: '127.0.0.1:4433'
  base_url: 'http://127.0.0.1:4433'
  database: 'db.sqlite3'
  oauth:
    github:
      client_id: 'your-github-client-id'
      client_secret: 'your-github-client-secret'

indexer:
  detect_languages: true
  directories:
    - path: '~/notes'
      filetypes: ['txt', 'md']

crawler:
  backend: 'http'
  timeout: 5
  delay: 1
  user_agent: 'Hister'

semantic_search:
  enable: false
  embedding_endpoint: 'http://localhost:11434/v1/embeddings'
  embedding_model: 'qwen3-embedding:8b'
  dimensions: 4096
  max_context_length: 4096
  chunk_overlap: 128
  similarity_threshold: 0.1
  result_limit: 50
  semantic_weight: 0.4

hotkeys:
  web:
    '/': 'focus_search_input'
    'enter': 'open_result'
    'alt+enter': 'open_result_in_new_tab'
    'alt+j': 'select_next_result'
    'alt+k': 'select_previous_result'
    'alt+o': 'open_query_in_search_engine'
    'alt+v': 'view_result_popup'
    'tab': 'autocomplete'
    '?': 'show_hotkeys'

sensitive_content_patterns:
  aws_access_key: '(^|[\s"''])AKIA[0-9A-Z]{16}([\s"'']|$)'
  github_token: '(ghp|gho|ghu|ghs|ghr)_[a-zA-Z0-9]{36}'
```

## `app` Section

| Key                       | Type   | Default                               | Description                                                                                                                                              |
| ------------------------- | ------ | ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `directory`               | string | platform default                      | Directory where Hister stores its data (index, rules, secret key).                                                                                       |
| `search_url`              | string | `https://google.com/search?q={query}` | Fallback web search URL. Use `{query}` as the placeholder for the search term.                                                                           |
| `access_token`            | string | (none)                                | Optional access token for securing the API. See [Access Token](#access-token).                                                                           |
| `user_handling`           | bool   | `false`                               | Enable multi-user mode. See [User Handling](/docs/user-handling) for details.                                                                            |
| `log_level`               | string | `info`                                | Log verbosity. One of: `debug`, `info`, `warn`, `error`.                                                                                                 |
| `log_format`              | string | `text`                                | Log output format. `text` emits colored, human-readable lines; `json` emits one JSON object per log entry, suitable for log aggregators.                 |
| `log_file`                | string | (none)                                | Path to a log file. When set, log output is written to this file instead of stderr. The file is created if it does not exist and appended to if it does. |
| `debug_sql`               | bool   | `false`                               | Enable verbose SQL query logging.                                                                                                                        |
| `open_results_on_new_tab` | bool   | `false`                               | Open search results in a new browser tab instead of the current tab.                                                                                     |
| `redirect_on_no_results`  | bool   | `true`                                | Redirect to the configured `search_url` when a query returns no results. Disable to always stay within Hister.                                           |
| `disable_previews`        | bool   | `false`                               | Disable the preview panel entirely. No HTML is stored on disk, and the preview UI is hidden. See [Disable Previews](#disable-previews).                  |

## `server` Section

| Key          | Type   | Default                | Description                                                                                                                                                                |
| ------------ | ------ | ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `address`    | string | `127.0.0.1:4433`       | Host and port to listen on. Use `[::]:4433` or `0.0.0.0:4433` to listen on all interfaces.                                                                                 |
| `base_url`   | string | derived from `address` | Public URL of the Hister instance. Required when `address` uses `0.0.0.0`. Must match how you access Hister.                                                               |
| `database`   | string | `db.sqlite3`           | Database connection. SQLite filename (relative to `app.directory`) or a PostgreSQL DSN. See [Database Backends](#database-backends).                                       |
| `oauth`      | map    | (none)                 | Optional map of OAuth/OIDC provider configurations. See [OAuth](#oauth).                                                                                                   |
| `oauth_only` | bool   | `false`                | When `true`, disables password login. OAuth and both the global `app.access_token` and per-user access tokens are still accepted. See [OAuth-Only Mode](#oauth-only-mode). |

## Database Backends

Hister supports **SQLite** (default) and **PostgreSQL**.

The `server.database` value determines which backend is used:

- If the value contains `=` it is treated as a **PostgreSQL DSN**.
- Otherwise it is treated as an **SQLite filename** relative to `app.directory`.

### SQLite (default)

```yaml
server:
  database: 'db.sqlite3'
```

### PostgreSQL

```yaml
server:
  database: 'host=localhost user=hister password=hister dbname=hister port=5432 sslmode=disable TimeZone=Europe/Budapest'
```

Hister uses the standard PostgreSQL DSN key=value format. Adjust `host`, `user`, `password`, `dbname`, `port`, `sslmode`, and `TimeZone` to match your setup.

## Semantic Search

Hister can augment keyword search with vector similarity search. When enabled, every indexed document is split into overlapping text chunks, each chunk is converted to a floating-point vector by an external embedding model, and the vectors are stored alongside the main index. At search time the query is also embedded and the closest chunks are retrieved, then merged with keyword results and re-ranked.

Semantic search is **opt-in** and disabled by default. It requires an OpenAI-compatible embeddings endpoint such as [Ollama](https://ollama.com), a local [llama.cpp](https://github.com/ggml-org/llama.cpp) server, or the OpenAI API itself.

| Key                    | Type              | Default                                | Description                                                                                                                                                                                                                                                    |
| ---------------------- | ----------------- | -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `enable`               | bool              | `false`                                | Enable or disable semantic search. All other keys are ignored when `false`.                                                                                                                                                                                    |
| `embedding_endpoint`   | string            | `http://localhost:11434/v1/embeddings` | URL of the OpenAI-compatible `/v1/embeddings` endpoint.                                                                                                                                                                                                        |
| `embedding_model`      | string            | `qwen3-embedding:8b`                   | Model name passed in the embedding request. Must match a model served by your endpoint.                                                                                                                                                                        |
| `api_key`              | string            | `""`                                   | Optional API key sent as `Authorization: Bearer <key>`. Required for hosted providers such as OpenAI, Together, Mistral, or Voyage.                                                                                                                            |
| `headers`              | map[string]string | `{}`                                   | Optional extra HTTP headers added to every embedding request. Useful for proxies or providers that use a non-standard auth scheme.                                                                                                                             |
| `dimensions`           | int               | `4096`                                 | Vector dimensionality. Must match the output of the chosen model.                                                                                                                                                                                              |
| `max_context_length`   | int               | `4096`                                 | Maximum number of tokens per text chunk sent to the embedding model.                                                                                                                                                                                           |
| `chunk_overlap`        | int               | `128`                                  | Number of tokens shared between consecutive chunks. Helps preserve context across chunk boundaries.                                                                                                                                                            |
| `query_prefix`         | string            | `"query: "`                            | String prepended to every search query before embedding. Many models require a task prefix for optimal recall (e.g. `"search_query: "` for Nomic, `"query: "` for E5/BGE). Set to `""` for models that do not use prefixes (e.g. OpenAI `text-embedding-3-*`). |
| `document_prefix`      | string            | `""`                                   | String prepended to every document chunk before embedding (e.g. `"search_document: "` for Nomic, `"passage: "` for E5/BGE). Must match the model's expected convention.                                                                                        |
| `similarity_threshold` | float             | `0.1`                                  | Minimum cosine similarity score for a chunk to be included in results. Raise this to surface only highly relevant matches.                                                                                                                                     |
| `result_limit`         | int               | `50`                                   | Maximum number of semantic hits retrieved per query.                                                                                                                                                                                                           |
| `semantic_weight`      | float             | `0.4`                                  | Weight applied to the semantic score when merging with keyword scores (0.0 = keyword only, 1.0 = semantic only). Adjustable in the web UI.                                                                                                                     |

### Vector Storage Backends

The vector store backend is chosen automatically based on `server.database`:

- **SQLite** (default) stores vectors in a separate `vectors.sqlite3` file in the same directory as the main database, using the [sqlite-vec](https://github.com/asg017/sqlite-vec) extension. No extra setup required.
- **PostgreSQL** stores vectors in the same database as the main data using the [pgvector](https://github.com/pgvector/pgvector) extension. Make sure `pgvector` is installed and enabled (`CREATE EXTENSION vector;`) before starting Hister.

### Example

```yaml
semantic_search:
  enable: true
  embedding_endpoint: 'http://localhost:11434/v1/embeddings'
  embedding_model: 'nomic-embed-text'
  dimensions: 768
  max_context_length: 512
  chunk_overlap: 50
  query_prefix: 'search_query: '
  document_prefix: 'search_document: '
  similarity_threshold: 0.5
  result_limit: 10
  semantic_weight: 0.4
  # api_key: 'sk-...'   # required for hosted providers
  # headers: {}         # extra HTTP headers for proxies or custom auth
```

The example above uses [nomic-embed-text](https://ollama.com/library/nomic-embed-text) via Ollama, which produces 768-dimensional vectors and fits well in a 512-token context window. The `query_prefix` and `document_prefix` values shown are the ones recommended by the Nomic model. Other models use different conventions: `"query: "` / `"passage: "` for E5 and BGE families (this is also the built-in default for `query_prefix`), `"Represent this sentence for searching relevant passages: "` for GTE. Check your model's documentation for the expected prefix strings. Set both to `""` for models that do not use prefixes (such as OpenAI `text-embedding-3-*`).

## TUI Settings

TUI settings are configured in a separate `tui.yaml` file located in the same directory as your main config file. This file is automatically created with default values when you first run `hister search`.

### Theme Settings

| Key            | Type   | Default            | Description                                                                                             |
| -------------- | ------ | ------------------ | ------------------------------------------------------------------------------------------------------- |
| `dark_theme`   | string | `tokyonight`       | Theme to use in dark mode. Available themes: catppuccin, dracula, gruvbox, nord, rose-pine, tokyonight. |
| `light_theme`  | string | `catppuccin-latte` | Theme to use in light mode.                                                                             |
| `color_scheme` | string | `auto`             | Color scheme mode: `auto` (follow system), `dark`, or `light`.                                          |
| `themes_dir`   | string | (built-in themes)  | Custom directory for theme YAML files (optional).                                                       |

**Built-in themes**: catppuccin-mocha, catppuccin-frappe, catppuccin-macchiato, catppuccin-latte, dracula, gruvbox, nord, rose-pine, tokyonight.

## `indexer` Section

| Key                | Type        | Default | Description                                                                                                                                                        |
| ------------------ | ----------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `detect_languages` | bool        | `true`  | Enable automatic language detection for indexed pages. See [Language Detection](#language-detection) for details on memory/CPU impact and reindexing requirements. |
| `directories`      | Directory[] | (none)  | List of local directories to index. See [Local Directory Indexing](#local-directory-indexing) for details.                                                         |
| `max_file_size_mb` | int         | `1`     | Maximum file size (in MB) to index. Files larger than this value are skipped.                                                                                      |

### Directory Entry

Each entry in `directories` is an object with the following keys:

| Key                | Type     | Default | Description                                                                                                                                                                                                                                                                                                  |
| ------------------ | -------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `path`             | string   | ""      | **(required)** Directory path to index. Paths starting with `~/` are expanded to your home directory.                                                                                                                                                                                                        |
| `filetypes`        | string[] | (none)  | Only index files with these extensions (without the dot). e.g. `['txt', 'md']`.                                                                                                                                                                                                                              |
| `patterns`         | string[] | (none)  | Only index files whose names match at least one glob pattern. e.g. `['doc_*', 'README*']`.                                                                                                                                                                                                                   |
| `excludes`         | string[] | (none)  | Skip files whose names match any of these glob patterns. e.g. `['*secret*', '*.tmp']`.                                                                                                                                                                                                                       |
| `include_hidden`   | bool     | `false` | When `true`, index hidden files/directories (starting with `.`) and well-known dependency/cache directories (`node_modules`, `__pycache__`, etc.) that are skipped by default. User-specified `excludes` still apply.                                                                                        |
| `delete_on_remove` | bool     | `false` | When `true`, automatically remove a file from the index when it is deleted or renamed on the filesystem.                                                                                                                                                                                                     |
| `user`             | string   | `""`    | Username that owns files in this directory. Files are only visible to this user in search results (plus global files). Omit or leave empty for global access. A non-existing username, or a configured username when `user_handling` is not enabled, causes an error log entry and the directory is skipped. |

When multiple filters are specified, they are applied in order: excludes first, then filetypes, then patterns. A file must pass all specified filters to be indexed. When a filter is omitted, it is not applied (all files pass).

## Local Directory Indexing

The `indexer.directories` option lets you index local files so they appear alongside your browser history in search results. Files are indexed automatically when the server starts, running in the background so the server is available immediately. A file watcher monitors configured directories for changes, so new and modified files are indexed automatically without needing to restart the server.

```yaml
indexer:
  directories:
    - path: '~/notes'
      filetypes: ['txt', 'md']
      patterns: ['doc_*']
      excludes: ['*secret*']
    - path: '~/Documents/wiki'
    - path: '/path/to/project'
      filetypes: ['go', 'py', 'js']
```

### User-scoped directories

When `user_handling` is enabled, you can scope indexed files to specific users using the `user` field. Files in a user-scoped directory are only visible to that user in search results. Global directories (no `user` set) are visible to all users.

```yaml
indexer:
  directories:
    - path: '/nextcloud/notes/alice'
      user: 'alice'
      filetypes: ['txt', 'md']
    - path: '/nextcloud/notes/bob'
      user: 'bob'
      filetypes: ['txt', 'md']
    - path: '/shared/docs'
      # no user = global, visible to all
      filetypes: ['pdf', 'doc']
```

Visibility rules:

- A user sees files from directories where `user == username` plus global directories (`user` empty or unset) plus their own web documents
- Admins have the same visibility as regular users (no special access to other users' files)
- Unauthenticated users see global directories only

Files are indexed recursively, with the following rules:

- Hidden files and directories (starting with `.`) are skipped unless `include_hidden: true`
- Well-known dependency/cache directories (`node_modules`, `bower_components`, `jspm_packages`, `__pycache__`, `__pypackages__`) are skipped unless `include_hidden: true`
- Binary files are skipped
- Files larger than `indexer.max_file_size_mb` (default: 1 MB) are skipped
- Files matching `sensitive_content_patterns` are skipped

Changes to indexed directories are picked up automatically by the file watcher, no server restart is needed. On server start, only files that have been modified since they were last indexed are re-processed. File results appear with the domain `local` and are served through the Hister web interface directly.

When `delete_on_remove: true` is set on a directory, deleting or renaming a file on the filesystem also removes it from the index automatically. This is opt-in and disabled by default.

No reindex is required when adding or removing files. Files are detected and indexed automatically.

## Disable Previews

By default, Hister stores the full HTML content of every indexed page on disk and makes it available in a split-pane preview panel in the search and history UI. Setting `disable_previews: true` turns this off completely:

- HTML content is **never written to disk** during indexing or re-indexing. Only the extracted plain text, title, URL, domain, language, and favicon are kept.
- Running `hister reindex` with this option enabled will delete all previously stored HTML files, reclaiming disk space.
- The preview panel, the per-result **view** button, and the **Preview** toggle are hidden in the web UI.

This is useful when disk space is limited or when you prefer not to retain full page snapshots.

```yaml
app:
  disable_previews: true
```

> **Note**: Favicons are unaffected by this setting and are always stored.

## Access Token

The `app.access_token` setting provides a simple authentication mechanism to secure your Hister instance. When configured, clients must include the token in API requests using the `X-Access-Token` HTTP header. This is particularly useful when exposing Hister to the network or internet, preventing unauthorized access to your browsing history and search index.

To use the access token, set it in your configuration file:

```yaml
app:
  access_token: 'your-secret-token-here'
```

The web UI automatically prompts for and stores the access token when configured. The access token has to be added to the browser extension as well.

For command-line usage with `curl` or similar tools, include the header in your requests:

```bash
curl -H "X-Access-Token: your-secret-token-here" http://localhost:4433/api/config
```

**Security note**: The access token is transmitted in plain text with each request. When exposing Hister over the network, always use HTTPS (via reverse proxy) to encrypt the token in transit. The token provides basic access control but does not replace proper authentication systems for multi-user scenarios.

## OAuth

When [user handling](/docs/user-handling) is enabled, Hister supports delegating authentication to external OAuth 2.0 / OpenID Connect providers. Users can then sign in with their existing accounts instead of a Hister-local password.

The `server.oauth` key is a map where each key is a provider name and the value is its configuration. Three providers are built in:

| Provider | Description                                                  |
| -------- | ------------------------------------------------------------ |
| `github` | GitHub accounts via the GitHub OAuth app                     |
| `google` | Google accounts via Google Identity                          |
| `oidc`   | Any OpenID Connect provider (Keycloak, Authentik, Dex, etc.) |

Each entry supports the following fields:

| Field               | Type     | Required  | Description                                                                                                                                                                                                 |
| ------------------- | -------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `client_id`         | string   | yes       | OAuth application client ID issued by the provider.                                                                                                                                                         |
| `client_secret`     | string   | yes       | OAuth application client secret issued by the provider.                                                                                                                                                     |
| `configuration_url` | string   | OIDC only | OIDC discovery URL (e.g. `https://accounts.example.com/.well-known/openid-configuration`). When set, `auth_url`, `token_url`, and `userinfo_url` are discovered automatically.                              |
| `auth_url`          | string   | no        | Override the provider's authorization endpoint. Not needed for `github` or `google`. Required for `oidc` when `configuration_url` is not set.                                                               |
| `token_url`         | string   | no        | Override the provider's token endpoint. Not needed for `github` or `google`. Required for `oidc` when `configuration_url` is not set.                                                                       |
| `userinfo_url`      | string   | no        | Override the OIDC userinfo endpoint. Normally discovered from `configuration_url`. Set this when auto-discovery does not return a `userinfo_endpoint` or when configuring OIDC without `configuration_url`. |
| `scopes`            | []string | no        | Additional OAuth scopes to request. The provider defaults are used when omitted.                                                                                                                            |

### GitHub Example

Register an OAuth app at [github.com/settings/developers](https://github.com/settings/developers). Set the **Authorization callback URL** to `https://your-hister-instance/api/oauth/callback?provider=github`.

```yaml
server:
  oauth:
    github:
      client_id: 'your-github-client-id'
      client_secret: 'your-github-client-secret'
```

### Google Example

Create OAuth credentials in the [Google Cloud Console](https://console.cloud.google.com/). Add `https://your-hister-instance/api/oauth/callback?provider=google` as an authorised redirect URI.

```yaml
server:
  oauth:
    google:
      client_id: 'your-google-client-id'
      client_secret: 'your-google-client-secret'
```

### Generic OIDC Example

```yaml
server:
  oauth:
    oidc:
      client_id: 'hister'
      client_secret: 'your-client-secret'
      configuration_url: 'https://accounts.example.com/.well-known/openid-configuration'
```

If your provider does not publish a discovery document, set `auth_url` and `token_url` directly and omit `configuration_url`:

```yaml
server:
  oauth:
    oidc:
      client_id: 'hister'
      client_secret: 'your-client-secret'
      auth_url: 'https://accounts.example.com/oauth/authorize'
      token_url: 'https://accounts.example.com/oauth/token'
```

### How It Works

1. The login page shows a **Sign in with &lt;Provider&gt;** button for each configured provider.
2. Clicking the button redirects the user to the provider's authorization page.
3. After the user grants access the provider redirects back to `/api/oauth/callback?provider=<name>`.
4. Hister verifies the state token, exchanges the authorization code for a token, and fetches the user's identity from the provider.
5. If no local account is linked to that identity, one is created automatically using the provider's username (or email prefix for OIDC).
6. The user is logged in and redirected to the home page.

> **Note**: OAuth login requires `app.user_handling: true`. The buttons only appear on the login page when user handling is active and at least one provider is configured.

## OAuth-Only Mode

Setting `server.oauth_only: true` prevents users from authenticating with a username/password. Only OAuth logins are accepted through the web interface.

```yaml
server:
  oauth_only: true
  oauth:
    github:
      client_id: 'your-github-client-id'
      client_secret: 'your-github-client-secret'
```

The global `app.access_token` and per-user personal access tokens continue to work regardless of this setting, so API clients, the CLI, and the browser extension are unaffected.

The login page hides the username/password form when `oauth_only` is active, showing only the OAuth provider buttons.

> **Note**: `oauth_only` only takes effect when `app.user_handling: true` and at least one OAuth provider is configured. Enabling it without any OAuth providers configured would leave users with no way to log in.

## Language Detection

The `indexer.detect_languages` option (default: `true`) controls automatic language detection for indexed pages. When enabled, Hister uses language detection libraries to identify the language of each page's content, creating separate language-specific indexes that improve search accuracy through language-aware tokenization and stemming.

**Performance considerations**: Language detection increases both CPU usage and memory consumption. Each document requires additional processing to analyze text and determine its language, and separate indexes are maintained for each detected language. If you're experiencing memory pressure or slow indexing performance, especially with large numbers of documents, consider disabling this feature.

**Important**: Changing this setting requires a full reindex to take effect. After enabling or disabling language detection in your config file, run:

```bash
hister reindex
```

The reindex operation will rebuild all indexes according to the new setting. With language detection disabled, all documents are indexed using a single default analyzer, reducing memory overhead and simplifying the indexing process at the cost of potentially less accurate search results.

## `hotkeys.web` Section

Defines keyboard shortcuts for the web interface. Each entry maps a key combination to an action.

**Key format**: `[modifier+]key` where modifier is `ctrl`, `alt`, or `meta`. Key can be a letter, digit, or special key (`enter`, `tab`, `arrowup`, `arrowdown`, etc.).

| Action                        | Description                                                     |
| ----------------------------- | --------------------------------------------------------------- |
| `focus_search_input`          | Move focus to the search input box                              |
| `open_result`                 | Open the selected (or first) result                             |
| `open_result_in_new_tab`      | Open the selected result in a new tab                           |
| `select_next_result`          | Move selection to the next result                               |
| `select_previous_result`      | Move selection to the previous result                           |
| `open_query_in_search_engine` | Open the current query in the configured fallback search engine |
| `view_result_popup`           | Open the offline preview popup for the selected result          |
| `autocomplete`                | Accept the autocomplete suggestion                              |
| `show_hotkeys`                | Show the keyboard shortcuts help overlay                        |

## TUI Configuration

TUI-specific settings are stored in a separate `tui.yaml` file in the same directory as your main config. This file is automatically created with defaults the first time you run `hister search`.

**Default location**: `~/.config/hister/tui.yaml` (or alongside your config file)

### tui.yaml Example

```yaml
dark_theme: 'tokyonight'
light_theme: 'catppuccin-latte'
color_scheme: 'auto'

hotkeys:
  ctrl+c: 'quit'
  f1: 'toggle_help'
  tab: 'toggle_focus'
  esc: 'toggle_focus'
  up: 'scroll_up'
  k: 'scroll_up'
  down: 'scroll_down'
  j: 'scroll_down'
  enter: 'open_result'
  ctrl+d: 'delete_result'
  ctrl+t: 'toggle_theme'
  ctrl+s: 'toggle_settings'
  ctrl+o: 'toggle_sort'
  alt+1: 'tab_search'
  alt+2: 'tab_history'
  alt+3: 'tab_rules'
  alt+4: 'tab_add'
```

### TUI Hotkeys

TUI keyboard shortcuts are configured in `tui.yaml` under the `hotkeys` section. See the [tui.yaml example](#tui-configuration) above.

| Action            | Description                                                                 |
| ----------------- | --------------------------------------------------------------------------- |
| `quit`            | Exit the TUI                                                                |
| `toggle_help`     | Show/hide the keybindings help overlay                                      |
| `toggle_focus`    | Switch between search input and results list                                |
| `scroll_up`       | Move selection up                                                           |
| `scroll_down`     | Move selection down                                                         |
| `open_result`     | Open the selected result in your browser                                    |
| `delete_result`   | Delete the selected entry from the index                                    |
| `toggle_theme`    | Open the interactive theme picker overlay                                   |
| `toggle_settings` | Open the keybinding editor overlay                                          |
| `toggle_sort`     | Toggle domain-based sorting for search results                              |
| `tab_search`      | Switch to the Search tab                                                    |
| `tab_history`     | Switch to the History tab (view recent searches)                            |
| `tab_rules`       | Switch to the Rules tab (manage skip/priority/versioning rules and aliases) |
| `tab_add`         | Switch to the Add tab (manually add URLs to index)                          |

## `crawler` Section

The `crawler` section configures the web crawler used by `hister index`, `hister index --recursive`,
and `hister import-browser`. All three commands share the same backend and request settings.
Every recursive crawl runs as a persistent job so it can be interrupted and resumed
without losing progress. See [Terminal Client](terminal-client) for usage details.

| Key               | Type              | Default | Description                                                                |
| ----------------- | ----------------- | ------- | -------------------------------------------------------------------------- |
| `backend`         | string            | `http`  | Scraping backend to use. One of: `http`, `chromedp`, `bidi`.               |
| `backend_options` | map               | (none)  | Backend-specific options. See [Backend Options](#crawler-backend-options). |
| `timeout`         | int               | `5`     | Request timeout in seconds.                                                |
| `delay`           | int               | `0`     | Seconds to wait between requests. Use to avoid overloading target servers. |
| `user_agent`      | string            | (none)  | Custom `User-Agent` header sent with every request (both backends).        |
| `headers`         | map[string]string | (none)  | Extra HTTP headers sent with every request (both backends).                |
| `cookies`         | Cookie[]          | (none)  | Cookies sent with every request. See [Crawler Cookies](#crawler-cookies).  |
| `no_robots`       | bool              | `false` | Disable robots.txt compliance during crawling.                             |

### Crawler Backend Options

The `backend_options` map passes configuration to the selected backend. Each backend validates its own options and rejects unknown keys.

**`http` backend** — no backend-specific options supported.

**`chromedp` backend**:

| Option      | Type   | Description                                   |
| ----------- | ------ | --------------------------------------------- |
| `exec_path` | string | Path to the Chrome or Chromium binary to use. |

```yaml
crawler:
  backend: 'chromedp'
  backend_options:
    exec_path: '/usr/bin/chromium'
  timeout: 15
```

**`bidi` backend** (WebDriver BiDi):

Connects to an **already-running** browser that exposes a [WebDriver BiDi](https://w3c.github.io/webdriver-bidi/) WebSocket endpoint. This is the W3C-standard automation protocol supported by Firefox (≥ 102), Chrome (≥ 106), Edge, and other modern browsers. Unlike `chromedp`, the `bidi` backend does **not** launch a browser process — it reuses one you have started yourself (headless or not).

| Option          | Type   | Default     | Description                                                                                        |
| --------------- | ------ | ----------- | -------------------------------------------------------------------------------------------------- |
| `socket`        | string | (none)      | Full WebSocket URL (e.g. `ws://127.0.0.1:9222/session`). When set, `host` and `port` are ignored.  |
| `host`          | string | `127.0.0.1` | Hostname or IP of the browser's BiDi endpoint.                                                     |
| `port`          | string | `9222`      | Port of the browser's BiDi endpoint.                                                               |
| `capture_delay` | float  | `0`         | Seconds to wait after page load before capturing HTML. Useful for pages that rely on JS rendering. |

Start your browser with BiDi enabled, for example:

```bash
# Firefox
firefox --remote-debugging-port 9222

# Chrome / Chromium
chromium --remote-debugging-port=9222
```

Then configure Hister to use it:

```yaml
crawler:
  backend: 'bidi'
  backend_options:
    host: '127.0.0.1'
    port: '9222'
    capture_delay: 1.5 # wait 1.5s after load for JS to render
  timeout: 15
```

Or using a full socket URL:

```yaml
crawler:
  backend: 'bidi'
  backend_options:
    socket: 'ws://127.0.0.1:9222/session'
```

### Crawler Cookies

Each entry in `cookies` is an object with the following keys:

| Key      | Type   | Required | Description                                        |
| -------- | ------ | -------- | -------------------------------------------------- |
| `name`   | string | ✓        | Cookie name.                                       |
| `value`  | string | ✓        | Cookie value.                                      |
| `domain` | string | ✓        | Domain the cookie applies to (e.g. `example.com`). |
| `path`   | string |          | Cookie path. Defaults to `/`.                      |

### Full Crawler Example

```yaml
crawler:
  backend: 'http'
  timeout: 10
  delay: 2
  user_agent: 'Hister'
  headers:
    Accept-Language: 'en-US,en;q=0.9'
  cookies:
    - name: 'session'
      value: 'abc123'
      domain: 'example.com'
      path: '/'
```

## `sensitive_content_patterns` Section

A map of named [Go regular expression](https://pkg.go.dev/regexp/syntax) patterns. Any indexed page containing a match will have that field redacted before storage. Useful for preventing accidental indexing of secrets, tokens, or credentials.

```yaml
sensitive_content_patterns:
  my_pattern: 'regex here'
```

Default patterns cover common secrets: AWS keys, GitHub tokens, SSH/PGP private keys.

## Environment Variables

You can override configuration values using environment variables. The naming convention is:

```textplain
HISTER__<SECTION>__<KEY>=value
```

For example:

- `HISTER__APP__LOG_LEVEL=debug` overrides `app.log_level`
- `HISTER__APP__LOG_FORMAT=json` overrides `app.log_format`
- `HISTER__APP__LOG_FILE=/var/log/hister.log` overrides `app.log_file`
- `HISTER__SERVER__ADDRESS=0.0.0.0:8080` overrides `server.address`

Two special-purpose variables are also supported:

| Variable          | Description                                                            |
| ----------------- | ---------------------------------------------------------------------- |
| `HISTER_PORT`     | Override the port only (keeps the existing host from `server.address`) |
| `HISTER_DATA_DIR` | Override `app.directory`                                               |
