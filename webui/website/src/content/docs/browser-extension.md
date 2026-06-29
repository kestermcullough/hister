---
date: '2026-03-19T00:00:00+00:00'
draft: false
title: 'Browser Extension'
---

The Hister browser extension is the primary way to automatically index your browsing history. It runs silently in the background, sending page content to your Hister server as you browse.

## Installation

- **Chrome / Chromium / Edge**: [Install from Chrome Web Store](https://chromewebstore.google.com/detail/hister/cciilamhchpmbdnniabclekddabkifhb)
- **Firefox**: [Install from Firefox Add-ons](https://addons.mozilla.org/en-US/firefox/addon/hister/) (also works on Firefox for Android)

After installing, click the extension icon in your browser toolbar to open the popup and verify the server URL is correct.

> The extension only communicates with your Hister server, it never contacts any third-party services or the websites you visit (except for downloading the page's favicon while collecting page data).

## Features

### Automatic Page Indexing

The extension automatically captures page content every time you visit a URL. It extracts the page title, full text, HTML, and favicon, then sends them to your Hister server via its API.

After a page is successfully indexed, the extension continues monitoring it in the background and re-submits if the content changes (for example on single-page applications). The re-check interval starts at 10 seconds and doubles each time the page content is unchanged, reducing resource usage over time.

Automatic indexing can be paused at any time using the toggle in the popup.

### Manual Reindex

The **Reindex Page** button in the popup forces an immediate re-submission of the current page, regardless of whether it has changed. This is useful after clearing your server's index or when a page failed to index automatically (indicated by a `!` badge on the extension icon).

### Keyboard Shortcuts

The extension defines browser level shortcuts for common indexing actions.

| Action                              | Windows and Linux | macOS       | Notes                                                                 |
| ----------------------------------- | ----------------- | ----------- | --------------------------------------------------------------------- |
| Index current page                  | `Ctrl+I`          | `Command+I` | Runs even when automatic indexing is disabled.                        |
| Disable indexing for current page   | `Ctrl+B`          | `Command+B` | Adds a skip rule for the exact current URL.                           |
| Disable indexing for current domain | `Ctrl+Y`          | `Command+Y` | Adds a skip rule for the current origin, including scheme and domain. |

Successful shortcut actions show a `✓` badge on the extension icon for a few seconds. Failed actions show a `!` badge.

Browser shortcuts can be changed in the browser extension shortcut settings. In Chrome, Chromium, and Edge, open `chrome://extensions/shortcuts`. In Firefox, open `about:addons`, choose the extension shortcut settings, then edit the Hister shortcuts.

### Search Engine Result Tracking

The extension detects when you click on a search result in **Google** or **DuckDuckGo** and records the query alongside the result's title and URL to provide that result for the same query in the future.

## Popup

Clicking the extension icon opens the popup, which provides quick access to the most common controls.

| Control                                      | Description                                                                                                           |
| -------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| **Automatic indexing** toggle                | Enable or disable automatic page indexing. The setting is saved immediately.                                          |
| **Reindex Page** button                      | Force a re-submission of the current page to the server.                                                              |
| **Authenticate Extension** button            | Copy session cookies from the logged-in Hister web UI to authenticate the extension (user handling only).             |
| **Settings icon** (⚙)                        | Expand an inline form to view and update the Server URL without opening the full options page.                        |
| **Show indicator for indexed pages** setting | Show a `✓` badge on pages that are already indexed. This is available after opening the settings view from the popup. |

A status banner appears at the bottom of the popup after any action, showing success or error feedback. If the server rejected the last submission, a `!` badge is shown on the extension icon; saving valid settings clears it.

The indexed page indicator is off by default. To enable it, click the extension icon, open the settings view with the settings icon, then turn on **Show indicator for indexed pages**. When enabled, the extension checks the active page against your Hister server and keeps a `✓` badge visible on pages already present in the index.

## Options Page

The full options page is accessible via your browser's extension manager (right-click the icon → _Options_, or navigate to the extensions settings page). It provides all configuration in one place.

To open it directly, right click on the extension icon and select "Options", or with:

- **Chrome**: `chrome://extensions` → find Hister → click **Details** → **Extension options**
- **Firefox**: `about:addons` → find Hister → click the **…** menu → **Preferences**

### Connection Settings

| Setting            | Default                  | Description                                                                                                                                                      |
| ------------------ | ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Server URL**     | `http://127.0.0.1:4433/` | The full URL of your Hister server, including scheme and port.                                                                                                   |
| **Custom Headers** | _(none)_                 | Additional HTTP headers included with every request. Useful for reverse-proxy authentication (e.g., `Authorization: Basic …`). Each header is a name/value pair. |

Click **Save Settings** to apply. The extension validates the connection by calling `GET /api/config` before saving; an invalid URL will show an error instead.

### Authentication

When [user handling](/docs/user-handling) is enabled, the extension authenticates by sharing the session cookies from the Hister web interface:

1. Log in to the Hister web interface in the same browser.
2. Click the **Authenticate Extension** button in the popup.

The extension copies the active session cookie, so it submits pages under your user account. You don't need to repeat this step if you log out in the web interface.

## Troubleshooting

**The extension icon shows a `!` badge**

The last attempt to send page data to the server failed. Open the popup to see the error. Common causes:

- The Hister server is not running: start it with `hister listen`.
- The **Server URL** is wrong: confirm it matches the address printed when the server starts (default `http://127.0.0.1:4433/`).
- User handling is enabled but the extension is not authenticated: click **Authenticate Extension** in the popup after logging in to the web interface.

**Pages are not being indexed**

- Make sure **Automatic indexing** is enabled in the popup.
- Check that the server is reachable and the URL is correct (see above).
- If user handling is enabled, make sure you have authenticated the extension (see above).
- Some pages (browser-internal pages like `chrome://…`, `about:…`) cannot be accessed by extensions and are silently skipped.
