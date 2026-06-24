---
date: '2026-03-26T00:00:00+00:00'
draft: false
title: 'User Handling'
---

User handling enables multiple independent users to share a single Hister instance. Each user has their own credentials, their own isolated search index, and their own access token for API clients. User handling is disabled by default, making Hister fully backward compatible. Existing single-user setups require no changes.

## Activation

Set `user_handling: true` in the `app` section of your configuration file:

```yaml
app:
  user_handling: true
```

> **Note**: When `user_handling` is active, `access_token` is used only to authenticate users by comparing it to the user's access token. This can be useful when the Hister admin sets `app.access_token` in the configuration file to their personal access token in order to execute command-line Hister commands as the admin user.

After enabling user handling, restart the server and create at least one user account before attempting to log in.

## Authentication

### Web Interface

When user handling is enabled, the web interface presents a login page to unauthenticated visitors. Enter your username and password to log in. Your session is maintained via a secure HTTP-only cookie valid for one year.

If OAuth providers are configured, the login page also shows **Sign in with &lt;Provider&gt;** buttons. See [OAuth Login](#oauth-login) below.

### OAuth Login

Hister supports signing in via GitHub, Google, or any OpenID Connect provider when `server.oauth` is configured. No password is required for OAuth accounts.

When a user signs in via OAuth for the first time, Hister automatically creates a local account linked to their provider identity (GitHub login name, Google email, or OIDC preferred username). Subsequent logins with the same provider identity reuse the same account.

OAuth accounts work identically to password accounts: they have their own isolated search index, personal access token, rules, and aliases. An OAuth user can generate a personal access token from their profile page to use with the CLI or browser extension.

See the [OAuth section of the configuration docs](/docs/configuration#oauth) for setup instructions.

### OAuth-Only Mode

Setting `server.oauth_only: true` disables password logins entirely. Only OAuth sign-in is accepted through the web interface. The login page hides the credential form and shows only the OAuth provider buttons.

This is useful when you want to enforce a single sign-on policy and prevent users from bypassing it with locally-set passwords.

The global `app.access_token` and per-user personal access tokens continue to work even when `oauth_only` is enabled, so API clients and CLI tools are unaffected.

See the [OAuth-Only Mode section of the configuration docs](/docs/configuration#oauth-only-mode) for the full configuration reference.

### Browser Extension

The extension authenticates by copying the session cookies from the already-logged-in web interface:

1. Log in to the Hister web interface in the same browser.
2. Click the **Authenticate Extension** button in the extension popup (or options page).

The extension will copy the active session cookies from the web UI. All pages indexed through the extension are stored under your user account.

### API / Command-Line Client

The `hister` CLI and any API client can authenticate using the personal access token via the `X-Access-Token` header:

```bash
curl -H "X-Access-Token: <your-token>" http://localhost:4433/api/stats
```

When using the `hister` CLI with user handling, pass your token with the `-t` flag:

```bash
hister -t <your-token> search "query"
```

## User Management Commands

All user management commands require `user_handling: true` in the configuration and direct access to the Hister server host (they operate on the database directly, not over the API).

### `create-user`

Create a new user account. Prompts interactively for a password (minimum 8 characters).

```bash
hister create-user USERNAME [--admin]
```

| Flag      | Description                      |
| --------- | -------------------------------- |
| `--admin` | Grant the user admin privileges. |

### `delete-user`

Permanently delete a user account (soft delete).

```bash
hister delete-user USERNAME
```

### `show-user`

Display information about a user account.

```bash
hister show-user USERNAME [--token]
```

| Flag      | Description                           |
| --------- | ------------------------------------- |
| `--token` | Also display the user's access token. |

Example output:

```
Username:   alice
ID:         1
Admin:      yes
Created at: 2026-03-26 09:00:00
Updated at: 2026-03-26 09:00:00
```

### `update-user`

Modify an existing user account. At least one flag must be provided.

```bash
hister update-user USERNAME [--username NEW] [--regen-token] [--toggle-admin]
```

| Flag             | Description                                                        |
| ---------------- | ------------------------------------------------------------------ |
| `--username NEW` | Rename the user to `NEW`.                                          |
| `--regen-token`  | Generate a new access token and print it. Invalidates the old one. |
| `--toggle-admin` | Toggle admin status on or off.                                     |

Flags may be combined. When `--username` is used together with other flags, the rename is applied first.

## Per-User Rules and Aliases

When user handling is enabled, each user has their own set of rules and aliases stored in the database. Changes made through the web UI or API affect only the authenticated user's rules and do not modify the configuration file.

- **Skip rules**: URLs matching a user's skip rules are silently ignored when indexing, just as in single-user mode.
- **Priority rules**: A user's priority rules boost matching results to the top of their search results.
- **Versioning rules**: URLs matching a user's versioning rules have their content diffed and stored on each re-index.
- **Search aliases**: Aliases defined by a user apply only to that user's searches.

Users can view and edit their rules and aliases through the **Rules** tab in the web interface, or via the API endpoints.

In single-user mode (user handling disabled), rules and aliases continue to be read from and written to the configuration file on disk.

## Regexp

Skip rules apply upon the **full** URL (from protocol to the query-string parameters) and limited

- Anchoring must include the protocol: Eg `^https://foo.com` or `^https?://(login|mail)\.` but no `^foo.com`
- `/login$` would **not** match `https://foo.com/login?auth=1`
- URL hash is removed (`https://foo.com/#active-tab` -> `https://foo.com/`)
- Query-string parameters are **not reordered** and barely stripped (only `utm_*` [at the moment](https://github.com/asciimoo/hister/blob/master/server/document/document.go#L137))
- [Go regular expression](https://pkg.go.dev/regexp/syntax) does not support look-ahead/look-behind regexp.

## Admin Users

Admin users have access to privileged operations. Currently, the following endpoints require admin privileges:

- **`POST /api/reindex`** rebuilds the entire full-text search index.

Non-admin users receive `403 Forbidden` when attempting to call admin-only endpoints.

Grant or revoke admin status using `create-user --admin` (at creation time) or `update-user --toggle-admin` (at any time).

## Single User Compatibility

Hister reserves user ID `0` for unauthenticated use. Documents indexed without user handling enabled are stored under user ID `0` and remain visible to all authenticated users after the feature is turned on. This means you can enable user handling on an existing instance without losing access to previously indexed content.

## Document Isolation

Each user's indexed documents are stored with their user ID. Searches are automatically scoped to:

- Documents indexed by the authenticated user.
- Documents indexed without user handling enabled (user ID `0`). These act as a shared, read-only baseline visible to everyone.

Users cannot see each other's documents.

The document count shown on the home page reflects the authenticated user's own document count rather than the total across all users.

## Public Mode

When `app.public: true` is enabled together with user handling, anonymous visitors can search only global documents stored under user ID `0`. Documents owned by named users remain private to those authenticated users.

Authenticated users can still add, delete, label, and manage their own content according to the normal user handling rules. Web history is disabled for everyone while public mode is enabled.

## Personal Access Tokens

Every user account has a personal access token used for API authentication. Tokens are random and stored in the database.

- Generate a new token from the web UI (Profile → Generate Token) or via `hister update-user --regen-token`.
- Generating a new token immediately invalidates the previous one. Update any clients (browser extension, scripts) accordingly.
- Tokens are not displayed in `show-user` output by default; use `--token` to reveal them.

## Profile API

The `/api/profile` endpoint returns information about the currently authenticated user:

```json
{
  "user_id": 1,
  "username": "alice",
  "is_admin": true
}
```

## Security Considerations

- Passwords are hashed with bcrypt before storage and are never returned by any API.
- Sessions are stored in signed HTTP-only cookies. The signing key is derived from Hister's secret key file.
- Personal access tokens bypass session cookies and can be used in scripts. Keep them secret and regenerate them if compromised.
- OAuth state tokens are single-use random values stored in the session cookie. They prevent cross-site request forgery during the OAuth redirect flow.
- OAuth accounts have no password set. If you need to disable an OAuth user's access, use `hister delete-user` or remove the provider from the configuration.
- Enable `server.oauth_only: true` to enforce OAuth-only login and prevent password authentication. Per-user access tokens and the global `app.access_token` remain valid for API and CLI access.
- User handling is intended for a trusted group of users on a shared instance (family, team). For public-facing deployments, place Hister behind a reverse proxy with HTTPS and only index content that may be shown publicly.
