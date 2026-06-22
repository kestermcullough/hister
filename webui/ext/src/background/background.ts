import { fetchAPI, sendPageData, sendPDFData, sendResult } from '../modules/network';

const missingURLMsg = {
  error: 'Missing or invalid Hister server URL. Configure it in the addon popup.',
};

// --- Badge helpers ---

function setErrorBadge(tabId: number) {
  chrome.action.setBadgeText({ text: '!', tabId }, () => void chrome.runtime.lastError);
  chrome.action.setBadgeBackgroundColor(
    { color: '#ff4444', tabId },
    () => void chrome.runtime.lastError,
  );
}

function setPreviouslyIndexedBadge(tabId: number) {
  chrome.action.setBadgeText({ text: '✓', tabId }, () => void chrome.runtime.lastError);
  chrome.action.setBadgeBackgroundColor(
    { color: '#44aa44', tabId },
    () => void chrome.runtime.lastError,
  );
  chrome.action.setBadgeTextColor({ color: '#ffffff', tabId }, () => void chrome.runtime.lastError);
}

function clearBadge(tabId: number) {
  chrome.action.setBadgeText({ text: '', tabId }, () => void chrome.runtime.lastError);
}

// --- Grey icon helpers ---

let greyIconCache: Record<number, ImageData> | null = null;

async function buildGreyIcons(): Promise<Record<number, ImageData>> {
  if (greyIconCache) return greyIconCache;
  const response = await fetch(chrome.runtime.getURL('assets/icons/icon128.png'));
  const blob = await response.blob();
  const bitmap = await createImageBitmap(blob);
  const result: Record<number, ImageData> = {};
  for (const size of [16, 32]) {
    const canvas = new OffscreenCanvas(size, size);
    const ctx = canvas.getContext('2d')!;
    ctx.drawImage(bitmap, 0, 0, size, size);
    const imageData = ctx.getImageData(0, 0, size, size);
    for (let i = 0; i < imageData.data.length; i += 4) {
      const grey =
        imageData.data[i] * 0.299 + imageData.data[i + 1] * 0.587 + imageData.data[i + 2] * 0.114;
      imageData.data[i] = grey;
      imageData.data[i + 1] = grey;
      imageData.data[i + 2] = grey;
      imageData.data[i + 3] = Math.round(imageData.data[i + 3] * 0.5);
    }
    result[size] = imageData;
  }
  greyIconCache = result;
  return result;
}

async function setGreyIcon(tabId: number): Promise<void> {
  clearBadge(tabId);
  try {
    const imageData = await buildGreyIcons();
    chrome.action.setIcon({ imageData, tabId }, () => void chrome.runtime.lastError);
  } catch (_) {}
}

function setNormalIcon(tabId: number): void {
  chrome.action.setIcon(
    { path: { '128': 'assets/icons/icon128.png' }, tabId },
    () => void chrome.runtime.lastError,
  );
}

// --- Per-tab sensitive-content rejection state ---

// Maps tabId → URL that was last rejected due to sensitive content.
const tabSensitiveState = new Map<number, string>();

chrome.tabs.onRemoved.addListener((tabId) => {
  tabSensitiveState.delete(tabId);
});

// --- Skip rules cache ---

interface SkipRulesCache {
  patterns: RegExp[];
  timestamp: number;
}

// TODO find better way to keep skip rules updated
// Perhaps a websocket connection to the server which pushes skip rule changes?
const SKIP_RULES_TTL = 60_000;
let skipRulesCache: SkipRulesCache | null = null;

async function getSkipPatterns(
  serverURL: string,
  customHeaders: { name: string; value: string }[],
): Promise<RegExp[]> {
  const now = Date.now();
  if (skipRulesCache && now - skipRulesCache.timestamp < SKIP_RULES_TTL) {
    return skipRulesCache.patterns;
  }
  try {
    const u = serverURL.endsWith('/') ? serverURL : serverURL + '/';
    const r = await fetchAPI(u + 'api/rules', { customHeaders });
    if (!r.ok) return skipRulesCache?.patterns ?? [];
    const data = await r.json();
    const patterns: RegExp[] = ((data.skip as string[]) ?? [])
      .map((s) => {
        try {
          return new RegExp(s);
        } catch (_) {
          return null;
        }
      })
      .filter((p): p is RegExp => p !== null);
    skipRulesCache = { patterns, timestamp: now };
    return patterns;
  } catch (_) {
    return skipRulesCache?.patterns ?? [];
  }
}

// --- Tab icon state ---

async function updateTabIcon(tabId: number, url: string): Promise<void> {
  if (
    !url ||
    url.startsWith('chrome://') ||
    url.startsWith('about:') ||
    url.startsWith('moz-extension://') ||
    url.startsWith('chrome-extension://')
  ) {
    return;
  }
  const data = await chrome.storage.local.get([
    'histerURL',
    'histerToken',
    'indexingEnabled',
    'histerCustomHeaders',
    'showIndexedBadge',
  ]);

  const serverURL: string = data['histerURL'] || '';
  const showIndexedBadge: boolean = data['showIndexedBadge'] === true;
  const customHeaders: { name: string; value: string }[] = Array.isArray(
    data['histerCustomHeaders'],
  )
    ? data['histerCustomHeaders']
    : [];
  if (data['histerToken']) {
    customHeaders.push({ name: 'X-Access-Token', value: data['histerToken'] });
  }

  if (data['indexingEnabled'] === false) {
    await setGreyIcon(tabId);
    if (showIndexedBadge && serverURL) {
      const indexed = await isUrlPreviouslyIndexed(url, serverURL, customHeaders);
      if (indexed) setPreviouslyIndexedBadge(tabId);
    }
    return;
  }

  if (!serverURL) {
    setNormalIcon(tabId);
    return;
  }

  const patterns = await getSkipPatterns(serverURL, customHeaders);
  if (patterns.some((re) => re.test(url))) {
    await setGreyIcon(tabId);
  } else {
    setNormalIcon(tabId);
    clearBadge(tabId);
  }
}

// --- PDF tab indexing ---

function isPDFUrl(url: string): boolean {
  try {
    const pathname = new URL(url).pathname.toLowerCase();
    return pathname.endsWith('.pdf');
  } catch (_) {
    return false;
  }
}

async function indexPDFTab(tabId: number, tab: chrome.tabs.Tab): Promise<void> {
  const data = await chrome.storage.local.get([
    'histerURL',
    'histerToken',
    'indexingEnabled',
    'histerCustomHeaders',
    'histerLabel',
  ]);

  if (data['indexingEnabled'] === false) return;

  const serverURL: string = data['histerURL'] || '';
  if (!serverURL) return;

  const customHeaders: { name: string; value: string }[] = Array.isArray(
    data['histerCustomHeaders'],
  )
    ? data['histerCustomHeaders']
    : [];
  if (data['histerToken']) {
    customHeaders.push({ name: 'X-Access-Token', value: data['histerToken'] });
  }

  const patterns = await getSkipPatterns(serverURL, customHeaders);
  if (patterns.some((re) => re.test(tab.url!))) {
    await setGreyIcon(tabId);
    return;
  }

  const u = serverURL.endsWith('/') ? serverURL : serverURL + '/';

  try {
    const response = await fetch(tab.url!, { credentials: 'include' });
    if (!response.ok) {
      setErrorBadge(tabId);
      return;
    }
    const buffer = await response.arrayBuffer();
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    const pdfBase64 = btoa(binary);

    const doc: Record<string, unknown> = {
      url: tab.url!,
      title: tab.title || tab.url!,
    };
    if (data['histerLabel']) {
      doc['label'] = data['histerLabel'];
    }

    const r = await sendPDFData(u + 'api/add_pdf', doc, pdfBase64, customHeaders);
    if (r.status === 201) {
      setNormalIcon(tabId);
      clearBadge(tabId);
    } else if (r.status === 406) {
      skipRulesCache = null;
      setGreyIcon(tabId);
    } else if (r.status === 422) {
      tabSensitiveState.set(tabId, tab.url!);
      setGreyIcon(tabId);
    } else {
      setErrorBadge(tabId);
    }
  } catch (err) {
    setErrorBadge(tabId);
  }
}

// --- Tab listeners ---

chrome.tabs.onActivated.addListener(async ({ tabId }) => {
  try {
    const tab = await chrome.tabs.get(tabId);
    if (tab.url) await updateTabIcon(tabId, tab.url);
  } catch (_) {}
});

chrome.tabs.onUpdated.addListener(async (tabId, changeInfo, tab) => {
  if (changeInfo.status === 'complete' && tab.url) {
    await updateTabIcon(tabId, tab.url);
    if (isPDFUrl(tab.url)) {
      await indexPDFTab(tabId, tab);
    }
  }
});

chrome.storage.onChanged.addListener(async (changes, area) => {
  if (area !== 'local') return;
  if (!('indexingEnabled' in changes || 'histerURL' in changes || 'showIndexedBadge' in changes))
    return;
  if ('histerURL' in changes) skipRulesCache = null;
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab?.id && tab.url) await updateTabIcon(tab.id, tab.url);
  } catch (_) {}
});

async function isUrlPreviouslyIndexed(
  url: string,
  serverURL: string,
  customHeaders: { name: string; value: string }[],
): Promise<boolean> {
  try {
    const base = serverURL.endsWith('/') ? serverURL : serverURL + '/';
    const r = await fetchAPI(`${base}api/document?url=${encodeURIComponent(url)}`, {
      method: 'HEAD',
      customHeaders,
    });
    return r.status === 200;
  } catch (_) {
    return false;
  }
}

// --- Message handler ---

// TODO check source
function cjsMsgHandler(request, sender, sendResponse) {
  chrome.storage.local
    .get(['histerURL', 'histerToken', 'indexingEnabled', 'histerCustomHeaders'])
    .then((data) => {
      let u = data['histerURL'] || '';
      const indexingEnabled = data['indexingEnabled'] !== false;
      const customHeaders = Array.isArray(data['histerCustomHeaders'])
        ? data['histerCustomHeaders']
        : [];

      // token is not required, this is just for backward compatibility
      if (data['histerToken']) {
        customHeaders.push({ name: 'X-Access-Token', value: data['histerToken'] });
      }

      if (request.action === 'getTabState') {
        const stored = tabSensitiveState.get(request.tabId as number);
        sendResponse({ isSensitive: stored !== undefined && stored === request.url });
        return;
      }

      if (request.action === 'addSkipRule') {
        if (!u) {
          sendResponse({ error: 'No server URL configured' });
          return;
        }
        const baseURL = u.endsWith('/') ? u : u + '/';
        (async () => {
          try {
            const rulesResp = await fetchAPI(baseURL + 'api/rules', { customHeaders });
            if (!rulesResp.ok) {
              sendResponse({ error: `Failed to fetch rules: ${rulesResp.status}` });
              return;
            }
            const rulesData = await rulesResp.json();
            const existingSkip: string[] = rulesData.skip ?? [];
            const existingPriority: string[] = rulesData.priority ?? [];
            const newSkip = [...existingSkip, request.pattern];
            const saveResp = await fetchAPI(baseURL + 'api/rules', {
              formData: {
                skip: newSkip.join(' '),
                priority: existingPriority.join(' '),
              },
              customHeaders,
            });
            if (!saveResp.ok) {
              sendResponse({ error: `Failed to save rule: ${saveResp.status}` });
              return;
            }
            if (request.deleteQuery) {
              const deleteResp = await fetchAPI(baseURL + 'api/delete', {
                body: { query: request.deleteQuery },
                customHeaders,
              });
              if (!deleteResp.ok) {
                sendResponse({ error: `Failed to delete documents: ${deleteResp.status}` });
                return;
              }
            }
            skipRulesCache = null;
            sendResponse({ ok: true });
            // Grey out the icon on the active tab immediately
            const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
            if (tab?.id && tab.url) await updateTabIcon(tab.id, tab.url);
          } catch (e) {
            sendResponse({ error: e.message });
          }
        })();
        return;
      }

      if (request.action === 'checkSkipRule') {
        if (!u) {
          sendResponse({ isSkipped: false });
          return;
        }
        const baseURL = u.endsWith('/') ? u : u + '/';
        getSkipPatterns(baseURL, customHeaders).then((patterns) => {
          sendResponse({ isSkipped: patterns.some((re) => re.test(request.url)) });
        });
        return true;
      }

      if (!u) {
        chrome.tabs.sendMessage(sender.tab.id, missingURLMsg);
        setErrorBadge(sender.tab.id);
        return;
      }
      if (!u.endsWith('/')) {
        u += '/';
      }
      if (request.pageData) {
        if (!indexingEnabled && request.action != 'reindex') {
          sendResponse({ status: 'disabled' });
          return;
        }
        chrome.storage.local.get(['histerLabel']).then((labelData) => {
          const pageData = { ...request.pageData };
          if (labelData['histerLabel']) {
            pageData.label = labelData['histerLabel'];
          }
          sendPageData(u + 'api/add', pageData, customHeaders)
            .then((r) => {
              if (r.status === 201) {
                setNormalIcon(sender.tab.id);
                clearBadge(sender.tab.id);
              } else if (r.status === 406) {
                // URL matched a server-side skip rule; invalidate cache and grey out
                skipRulesCache = null;
                setGreyIcon(sender.tab.id);
              } else if (r.status === 422) {
                // Document rejected due to sensitive content; not an error
                tabSensitiveState.set(sender.tab.id, sender.tab.url ?? '');
                setGreyIcon(sender.tab.id);
              } else {
                setErrorBadge(sender.tab.id);
              }
              sendResponse({ status: 'ok', status_code: r.status });
            })
            .catch((err) => {
              setErrorBadge(sender.tab.id);
              sendResponse({ error: err.message });
            });
        });
        return true;
      }
      if (request.resultData) {
        sendResult(u + 'api/history', request.resultData, customHeaders)
          .then((r) => {
            if (r.status === 201) {
              clearBadge(sender.tab.id);
            } else if (r.status != 406) {
              setErrorBadge(sender.tab.id);
            }
            sendResponse({ status: 'ok', status_code: r.status });
          })
          .catch((err) => {
            setErrorBadge(sender.tab.id);
            sendResponse({ error: err.message });
          });
        return true;
      }
    })
    .catch((error) => {
      chrome.tabs.sendMessage(sender.tab.id, missingURLMsg);
      setErrorBadge(sender.tab.id);
    });
  return true;
}

chrome.runtime.onMessage.addListener(cjsMsgHandler);
