<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import {
    buildPreviewUrl,
    pushPreviewHistory,
    replacePreviewHistory,
    withSkipUrl,
  } from '$lib/preview';
  import { page } from '$app/stores';
  import {
    WebSocketManager,
    KeyHandler,
    getSearchUrl,
    exportJSON,
    exportCSV,
    exportRSS,
    formatTimestamp,
    formatRelativeTime,
    scrollTo,
    escapeHTML,
    buildSearchQuery,
    parseSearchResults,
    openURL,
  } from '$lib/search';
  import { fetchConfig, apiFetch, getUserId } from '$lib/api';
  import { SkipRuleActions, buildUrlSkipPattern, buildDomainSkipPattern } from '@hister/components';
  import { showHelp } from '$lib/stores';
  import type { SearchResults, SemanticHit, SearchResult, SearchQueryOptions } from '$lib/search';
  import { RESULTS_PER_PAGE } from '$lib/search';
  import { animate } from 'animejs';
  import { Input } from '@hister/components/ui/input';
  import { Button } from '@hister/components/ui/button';
  import { Badge } from '@hister/components/ui/badge';
  import { Separator } from '@hister/components/ui/separator';
  import * as Dialog from '@hister/components/ui/dialog';
  import * as Card from '@hister/components/ui/card';
  import * as DropdownMenu from '@hister/components/ui/dropdown-menu';
  import * as Tooltip from '@hister/components/ui/tooltip';
  import { ScrollArea } from '@hister/components/ui/scroll-area';
  import { PreviewPanel } from '$lib/components';
  import { Kbd } from '@hister/components/ui/kbd';
  import {
    Search,
    Star,
    Globe,
    MoreVertical,
    Eye,
    Trash2,
    Pin,
    PinOff,
    Download,
    ExternalLink,
    History,
    Shield,
    Link2,
    Keyboard,
    HelpCircle,
    X,
    ChevronDown,
    Calendar,
    Filter,
    Sparkles,
  } from 'lucide-svelte';
  import type { HistoryItem } from '$lib/types';

  interface Config {
    wsUrl: string;
    searchUrl: string;
    openResultsOnNewTab: boolean;
    hotkeys: Record<string, string>;
    semanticEnabled: boolean;
    similarityThreshold: number;
    semanticWeight: number;
  }

  let config: Config = $state({
    wsUrl: '',
    searchUrl: '',
    openResultsOnNewTab: false,
    hotkeys: {},
    semanticEnabled: false,
    similarityThreshold: 0.5,
    semanticWeight: 0.4,
  });

  let wsManager: WebSocketManager | undefined;
  let keyHandler: KeyHandler | undefined;
  let inputEl: HTMLInputElement | null = $state(null);

  let query = $state('');
  let autocomplete = $state('');
  let connected = $state(false);
  let lastResults = $state<SearchResults | null>(null);
  let accumulatedDocs = $state<SearchResult[]>([]);
  let pageKey = $state('');
  let hasMore = $state(false);
  let loadingMoreForQuery = $state('');
  let sentinelEl = $state<HTMLElement | undefined>();
  let highlightIdx = $state(0);
  let currentSort = $state('');
  let dateFrom = $state('');
  let dateTo = $state('');
  let showPopup = $state(false);
  let popupUrl = $state('');
  let popupHintTitle = $state('');
  let previewFullscreen = $state(false);
  let actionsQuery = $state('');
  let actionsMessage: string | null = $state(null);
  let actionsError = $state(false);

  // Desktop split-pane readability panel state
  let panelUrl = $state('');
  let panelHintTitle = $state('');
  let isDesktop = $state(false);
  let panelOpen = $state(true);

  let resultsShown = $state(false);

  // Semantic search per-session state — read from localStorage immediately so
  // the first $effect run doesn't overwrite the saved value with the default.
  let semanticOn = $state(localStorage.getItem('hister-semantic-on') === 'true');
  let similarityThreshold = $state(
    parseFloat(localStorage.getItem('hister-semantic-threshold') ?? 'NaN') || 0.5,
  );
  let semanticWeight = $state(
    parseFloat(localStorage.getItem('hister-semantic-weight') ?? 'NaN') || 0.4,
  );

  let contextMenuSearch: string | null = $state(null);
  let contextMenuPos = $state({ x: 0, y: 0 });

  let showDeleteConfirm = $state(false);
  let deleteConfirmUrl = $state('');
  let deleteConfirmSkip = $state(false);

  let showDeleteAllConfirm = $state(false);
  let deleteError: string | null = $state(null);
  let deleteErrorTimer: any = null;

  let recentSearches: string[] = $state([]);
  let rulesCount = $state(0);
  let aliasesCount = $state(0);
  let historyCount = $state(0);

  let displayHistoryCount = $state(0);
  let displayRulesCount = $state(0);
  let displayAliasesCount = $state(0);

  let heroTitleEl: HTMLElement | undefined = $state();
  let searchBoxEl: HTMLElement | undefined = $state();
  let hintEl: HTMLElement | undefined = $state();
  let chipsContainerEl: HTMLElement | undefined = $state();
  let statsRowEl: HTMLElement | undefined = $state();
  let kbdEl: HTMLElement | null = $state(null);
  let underlineEl: HTMLElement | undefined = $state();

  let animationHandles: any[] = [];

  const chipColors = [
    { border: 'border-hister-indigo', bg: 'bg-hister-indigo/10', text: 'text-hister-indigo' },
    { border: 'border-hister-teal', bg: 'bg-hister-teal/10', text: 'text-hister-teal' },
    { border: 'border-hister-coral', bg: 'bg-hister-coral/10', text: 'text-hister-coral' },
    { border: 'border-hister-amber', bg: 'bg-hister-amber/10', text: 'text-hister-amber' },
  ];

  const hotkeyActions: Record<
    string,
    (e?: KeyboardEvent, isInputFocus?: boolean) => void | boolean
  > = {
    open_result: openSelectedResult,
    open_result_in_new_tab: (e?: KeyboardEvent, i?: boolean) => openSelectedResult(e, i, true),
    select_next_result: selectNextResult,
    select_previous_result: selectPreviousResult,
    open_query_in_search_engine: openQueryInSearchEngine,
    focus_search_input: focusSearchInput,
    view_result_popup: viewResultPopup,
    autocomplete: autocompleteQuery,
    show_hotkeys: showHotkeys,
    delete_result: deleteSelectedResult,
  };

  const isSearching = $derived(query.length > 0 || resultsShown);

  interface MergedResult {
    url: string;
    title: string;
    domain: string;
    score?: number;
    text?: string;
    favicon?: string;
    added?: number;
    semanticScore?: number;
    finalScore: number;
    sourceType: 'keyword' | 'semantic' | 'both';
  }

  function mergeResults(
    docs: SearchResults['documents'],
    hits: SemanticHit[] | undefined,
    alpha: number,
  ): MergedResult[] {
    const kwDocs = docs ?? [];
    if (!semanticOn || !config.semanticEnabled || !hits?.length) {
      return kwDocs.map((d) => ({ ...d, finalScore: d.score ?? 0, sourceType: 'keyword' }));
    }

    const maxBleve = Math.max(...kwDocs.map((d) => d.score ?? 0), 1);
    const semByDocId = new Map<string, number>(hits.map((h) => [h.doc_id, h.similarity]));

    // Helper: the doc_id is either a bare URL or "{uid}:{url}".
    function urlFromDocId(docId: string): string {
      const userId = getUserId();
      if (userId) {
        const prefix = `${userId}:`;
        if (docId.startsWith(prefix)) return docId.slice(prefix.length);
      }
      return docId;
    }

    const merged = new Map<string, MergedResult>();

    for (const d of kwDocs) {
      // Find whether this keyword doc also appears in semantic hits.
      // The semantic doc_id for this user+URL:
      const userId = getUserId();
      const expectedDocId = userId ? `${userId}:${d.url}` : d.url;
      const semScore = semByDocId.get(expectedDocId) ?? semByDocId.get(d.url);
      const norm = (d.score ?? 0) / maxBleve;
      const finalScore =
        semScore !== undefined ? (1 - alpha) * norm + alpha * semScore : (1 - alpha) * norm;
      merged.set(d.url, {
        ...d,
        semanticScore: semScore,
        finalScore,
        sourceType: semScore !== undefined ? 'both' : 'keyword',
      });
    }

    // Add semantic-only hits (server sets `document` only for non-keyword hits).
    for (const hit of hits) {
      if (!hit.document) continue;
      const url = hit.document.url;
      if (!merged.has(url)) {
        merged.set(url, {
          url,
          title: hit.document.title ?? '',
          domain: hit.document.domain ?? '',
          favicon: hit.document.favicon,
          added: hit.document.added,
          text: hit.document.text,
          semanticScore: hit.similarity,
          finalScore: alpha * hit.similarity,
          sourceType: 'semantic',
        });
      }
    }

    return Array.from(merged.values()).sort((a, b) => b.finalScore - a.finalScore);
  }

  const mergedResults = $derived(
    mergeResults(accumulatedDocs, lastResults?.semantic_hits, semanticWeight),
  );

  const historyLen = $derived((lastResults?.history as any)?.length || 0);
  const docsLen = $derived(mergedResults.length);
  const totalResults = $derived(historyLen + docsLen);
  const hasResults = $derived(totalResults > 0);

  function connect() {
    wsManager = new WebSocketManager(config.wsUrl, {
      onOpen: () => {
        connected = true;
        if (query) sendQuery(query);
      },
      onMessage: renderResults,
      onClose: () => {
        connected = false;
      },
      onError: () => {
        connected = false;
      },
    });
    wsManager.connect();
  }

  function searchQueryOpts(pageKey = ''): SearchQueryOptions {
    return {
      sort: currentSort,
      dateFrom,
      dateTo,
      semantic: { enabled: semanticOn && config.semanticEnabled, threshold: similarityThreshold },
      pageKey,
      limit: RESULTS_PER_PAGE,
    };
  }

  function sendQuery(q: string) {
    loadingMoreForQuery = '';
    pageKey = '';
    hasMore = false;
    wsManager?.send(JSON.stringify(buildSearchQuery(q, searchQueryOpts())));
  }

  function loadMoreResults() {
    if (!pageKey || !hasMore || loadingMoreForQuery) return;
    loadingMoreForQuery = query;
    wsManager?.sendImmediate(JSON.stringify(buildSearchQuery(query, searchQueryOpts(pageKey))));
  }

  const skipUrl = { value: false };
  let lastPushedEmpty = true;

  // --- URL builders ---

  function buildSearchUrl(): string {
    return query
      ? `/?q=${encodeURIComponent(query)}${dateFrom ? '&date_from=' + encodeURIComponent(dateFrom) : ''}${dateTo ? '&date_to=' + encodeURIComponent(dateTo) : ''}`
      : '/';
  }

  // --- History state helpers ---

  function pushSearchHistory() {
    const url = buildSearchUrl();
    history.pushState({ type: 'search', query, dateFrom, dateTo }, '', url);
    lastPushedEmpty = !query;
  }

  function replaceSearchHistory() {
    const url = buildSearchUrl();
    history.replaceState({ type: 'search', query, dateFrom, dateTo }, '', url);
    lastPushedEmpty = !query;
  }

  function updateURL() {
    if (skipUrl.value) return;
    if (previewFullscreen) return;
    const isEmpty = !query;
    if (isEmpty !== lastPushedEmpty) {
      pushSearchHistory();
    } else {
      replaceSearchHistory();
    }
  }

  function handlePopState(event: PopStateEvent) {
    const state = event.state as { type?: string; id?: string; title?: string } | null;
    if (state?.type === 'preview') {
      panelUrl = state.id || '';
      panelHintTitle = state.title || '';
      panelOpen = true;
      previewFullscreen = true;
      return;
    }
    previewFullscreen = false;
    skipUrl.value = true;
    const params = new URLSearchParams(window.location.search);
    query = params.get('q') || '';
    dateFrom = params.get('date_from') || '';
    dateTo = params.get('date_to') || '';
    lastPushedEmpty = !query;
    if (query && connected) sendQuery(query);
    if (!query) {
      autocomplete = '';
      lastResults = null;
    }
    tick().then(() => {
      skipUrl.value = false;
    });
  }

  function renderResults(event: MessageEvent) {
    const res = parseSearchResults(event.data);
    const isLoadMore = loadingMoreForQuery !== '' && loadingMoreForQuery === query;
    loadingMoreForQuery = '';
    if (isLoadMore) {
      accumulatedDocs = [...accumulatedDocs, ...(res.documents ?? [])];
      lastResults = { ...lastResults!, ...res, documents: accumulatedDocs };
    } else {
      accumulatedDocs = res.documents ?? [];
      lastResults = res;
      autocomplete = (query && res.query_suggestion) || '';
      highlightIdx = 0;
      resultsShown = true;
    }
    hasMore = (res.documents?.length ?? 0) >= 20 && !!res.page_key;
    pageKey = res.page_key ?? '';
  }

  function stripHtml(s: string): string {
    return s.replace(/<[^>]*>/g, '');
  }

  function openResult(url: string, title: string, newWindow = false) {
    if (config.openResultsOnNewTab) newWindow = true;
    saveHistoryItem(url, stripHtml(title), query, false, () => openURL(url, newWindow));
  }

  function sendHistoryBeacon(url: string, title: string, queryStr: string) {
    const payload = JSON.stringify({
      url,
      title: stripHtml(title),
      query: queryStr,
      delete: false,
    });
    navigator.sendBeacon('api/history', new Blob([payload], { type: 'application/json' }));
  }

  async function saveHistoryItem(
    url: string,
    title: string,
    queryStr: string,
    remove: boolean,
    callback?: () => void,
  ) {
    try {
      const res = await apiFetch('/history', {
        method: 'POST',
        headers: { 'Content-type': 'application/json; charset=UTF-8' },
        body: JSON.stringify({ url, title, query: queryStr, delete: remove }),
      });
      callback?.();
    } catch {}
  }

  function setSort(sortId: string) {
    if (currentSort === sortId) return;
    currentSort = sortId;
    if (query) sendQuery(query);
  }

  function setDeleteError(msg: string) {
    deleteError = msg;
    if (deleteErrorTimer) clearTimeout(deleteErrorTimer);
    deleteErrorTimer = setTimeout(() => {
      deleteError = null;
    }, 6000);
  }

  async function deleteResult(url: string) {
    const res = await apiFetch('/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        query: 'url:"' + url.replaceAll('"', '\\"') + '"',
      }),
    });
    if (!res.ok) {
      const text = await res.text();
      setDeleteError(text || 'Delete failed.');
      return;
    }
    const data = await res.json();
    if (!data.deleted) {
      setDeleteError('Document not found in index. Run "hister reindex" to fix stale entries.');
      return;
    }
    accumulatedDocs = accumulatedDocs.filter((d) => d.url !== url);
    if (lastResults) {
      lastResults = { ...lastResults, documents: accumulatedDocs };
    }
  }

  function deleteSelectedResult(e?: KeyboardEvent) {
    if (e) e.preventDefault();
    const links = document.querySelectorAll<HTMLAnchorElement>('[data-result] [data-result-link]');
    const link = links[highlightIdx];
    if (!link) return;
    const url = link.getAttribute('data-result-link') ?? link.getAttribute('href');
    if (!url) return;
    if (localStorage.getItem('hister-skip-delete-confirm') === 'true') {
      deleteResult(url);
      return;
    }
    deleteConfirmUrl = url;
    deleteConfirmSkip = false;
    showDeleteConfirm = true;
  }

  function confirmDelete() {
    if (deleteConfirmSkip) {
      localStorage.setItem('hister-skip-delete-confirm', 'true');
    }
    showDeleteConfirm = false;
    deleteResult(deleteConfirmUrl);
    deleteConfirmUrl = '';
  }

  function cancelDelete() {
    showDeleteConfirm = false;
    deleteConfirmUrl = '';
  }

  async function deleteAllResults() {
    const q = query + (getUserId() !== undefined ? ' user_id:' + getUserId() : '');
    const res = await apiFetch('/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ query: q }),
    });
    if (!res.ok) {
      const text = await res.text();
      setDeleteError(text || 'Delete all failed.');
      return;
    }
    const data = await res.json();
    if (!data.deleted) {
      setDeleteError(
        'No matching documents found in index. Run "hister reindex" if results appear stale.',
      );
      return;
    }
    accumulatedDocs = [];
    if (lastResults) {
      lastResults = { ...lastResults, documents: [], total: 0 };
    }
    resultsShown = false;
  }

  function confirmDeleteAll() {
    showDeleteAllConfirm = false;
    deleteAllResults();
  }

  function cancelDeleteAll() {
    showDeleteAllConfirm = false;
  }

  // Convert a file:// URL to a server-side /api/file?path= URL for in-browser viewing.
  // On Windows, strips the extra leading slash before the drive letter (file:///C:/ → C:/).
  function fileResultUrl(url: string): string {
    if (!url.startsWith('file://')) return url;
    let path = url.slice('file://'.length);
    if (/^\/[A-Za-z]:/.test(path)) path = path.slice(1);
    return 'api/file?path=' + encodeURIComponent(path);
  }

  function updatePriorityResult(url: string, title: string, remove: boolean) {
    const q = actionsQuery || query;
    if (!q) return;
    saveHistoryItem(url, stripHtml(title), q, remove, () => {
      actionsMessage = `Priority result ${remove ? 'deleted' : 'added'}.`;
      actionsError = false;
    });
  }

  async function addSkipRuleForResult(
    url: string,
    domain: string,
    type: 'url' | 'domain',
    deleteMatches: boolean,
  ) {
    actionsMessage = null;
    actionsError = false;
    try {
      const pattern = type === 'url' ? buildUrlSkipPattern(url) : buildDomainSkipPattern(url);
      const rulesResp = await apiFetch('/rules');
      const rules = await rulesResp.json();
      const skipList: string[] = rules.skip || [];
      if (!skipList.includes(pattern)) {
        skipList.push(pattern);
      }
      const formData = new URLSearchParams();
      formData.set('skip', skipList.join('\n'));
      formData.set('priority', (rules.priority || []).join('\n'));
      await apiFetch('/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: formData.toString(),
      });
      if (deleteMatches) {
        const uid = getUserId();
        const userFilter = uid !== undefined ? ` user_id:${uid}` : '';
        const deleteQuery =
          type === 'url'
            ? `url:"${url.replaceAll('"', '\\"')}"${userFilter}`
            : `domain:${domain}${userFilter}`;
        await apiFetch('/delete', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: deleteQuery }),
        });
        if (type === 'url') {
          accumulatedDocs = accumulatedDocs.filter((d) => d.url !== url);
        } else {
          accumulatedDocs = accumulatedDocs.filter((d) => d.domain !== domain);
        }
        if (lastResults) {
          lastResults = { ...lastResults, documents: accumulatedDocs };
        }
      }
      actionsMessage = `Skip rule added${deleteMatches ? ' and documents deleted' : ''}.`;
      actionsError = false;
    } catch {
      actionsMessage = 'Failed to add skip rule.';
      actionsError = true;
    }
  }

  function openReadable(e: Event, url: string, title: string) {
    e.preventDefault();
    if (e.stopPropagation) e.stopPropagation();
    if (isDesktop) {
      if (!panelOpen) {
        panelOpen = true;
        localStorage.setItem('hister-panel-open', 'true');
      }
      panelHintTitle = title;
      panelUrl = url;
      return;
    }
    // Mobile: open fullscreen preview instead of popup dialog
    panelUrl = url;
    panelHintTitle = title;
    previewFullscreen = true;
    withSkipUrl(skipUrl, () => pushPreviewHistory(url, title));
    previewFullscreen = true;
    withSkipUrl(skipUrl, () => pushPreviewHistory(panelUrl, panelHintTitle));
  }

  function exitFullscreen() {
    previewFullscreen = false;
    withSkipUrl(skipUrl, () => pushSearchHistory());
  }

  function closePanelAndFullscreen() {
    previewFullscreen = false;
    panelOpen = false;
    localStorage.setItem('hister-panel-open', 'false');
    withSkipUrl(skipUrl, () => pushSearchHistory());
  }

  function selectNthResult(n: number) {
    if (!totalResults) return;
    highlightIdx = (highlightIdx + n + totalResults) % totalResults;
    const results = document.querySelectorAll('[data-result]');
    scrollTo(results[highlightIdx]);
  }

  function selectNextResult(e?: KeyboardEvent) {
    if (e) e.preventDefault();
    selectNthResult(1);
  }
  function selectPreviousResult(e?: KeyboardEvent) {
    if (e) e.preventDefault();
    selectNthResult(-1);
  }

  function openSelectedResult(e?: KeyboardEvent, isInputFocus?: boolean, newWindow = false) {
    if (e) e.preventDefault();
    if (query.startsWith('!!')) {
      openURL(getSearchUrl(config.searchUrl, query.substring(2)), newWindow);
      return;
    }
    const res = document.querySelectorAll<HTMLAnchorElement>('[data-result] [data-result-link]')[
      highlightIdx
    ];
    if (res) {
      openResult(res.getAttribute('href')!, res.innerText, newWindow);
    }
  }

  function viewResultPopup(e?: KeyboardEvent) {
    if (e) e.preventDefault();
    if (isDesktop) {
      if (previewFullscreen) {
        // Fullscreen → back to split-screen
        exitFullscreen();
      } else if (panelOpen) {
        // Split-screen → fullscreen
        enterFullscreen();
      } else {
        // No preview → open split-screen
        openHighlightedReadable();
      }
    } else {
      // Mobile: toggle fullscreen
      if (previewFullscreen) {
        closePanelAndFullscreen();
      } else {
        openHighlightedReadable();
      }
    }
  }

  function openHighlightedReadable() {
    const readables = document.querySelectorAll('[data-result] [data-readable]');
    if (highlightIdx >= 0 && highlightIdx < readables.length) {
      const el = readables[highlightIdx] as HTMLElement;
      const result = el.closest('[data-result]')!;
      const link = result.querySelector<HTMLAnchorElement>('[data-result-link]')!;
      openReadable(
        { preventDefault: () => {}, stopPropagation: () => {} } as unknown as Event,
        link.getAttribute('data-result-link')!,
        link.innerText,
      );
    }
  }

  function autocompleteQuery(e?: KeyboardEvent, isInputFocus?: boolean) {
    if (e) e.preventDefault();
    if (isInputFocus && autocomplete && query !== autocomplete) {
      query = autocomplete;
      sendQuery(query);
    } else {
      return true;
    }
  }

  function openQueryInSearchEngine(e?: KeyboardEvent) {
    if (e) e.preventDefault();
    openURL(getSearchUrl(config.searchUrl, query));
  }
  function focusSearchInput(e?: KeyboardEvent, isInputFocus?: boolean) {
    if (!isInputFocus) {
      if (e) e.preventDefault();
      inputEl?.focus();
    }
  }

  function closePopup(): boolean {
    if (previewFullscreen) {
      closePanelAndFullscreen();
      return true;
    }
    if (showPopup) {
      showPopup = false;
      return true;
    }
    return false;
  }

  const hotkeyDescriptions: Record<string, string> = {
    open_result: 'Open result',
    open_result_in_new_tab: 'Open result in new tab',
    select_next_result: 'Select next result',
    select_previous_result: 'Select previous result',
    open_query_in_search_engine: 'Open in search engine',
    focus_search_input: 'Focus search input',
    view_result_popup: 'View result content',
    autocomplete: 'Autocomplete query',
    show_hotkeys: 'Show help',
    delete_result: 'Delete focused result',
  };

  function showHotkeys(e?: KeyboardEvent, isInputFocus?: boolean) {
    if ($showHelp) {
      $showHelp = false;
      return;
    }
    if (!isInputFocus) {
      $showHelp = true;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (showDeleteConfirm) {
      if (e.key === 'Enter') {
        e.preventDefault();
        confirmDelete();
      } else if (e.key === 'Escape') {
        e.preventDefault();
        cancelDelete();
      }
      return;
    }
    const isInputFocus =
      document.activeElement instanceof HTMLInputElement ||
      document.activeElement instanceof HTMLTextAreaElement;
    keyHandler?.handle(e, isInputFocus);
    if (e.key === 'Escape') {
      if ($showHelp) {
        $showHelp = false;
        e.preventDefault();
        return;
      }
      if (contextMenuSearch) {
        contextMenuSearch = null;
        e.preventDefault();
        return;
      }
      if (closePopup()) {
        e.preventDefault();
        return;
      }
      if (isSearching) {
        query = '';
        resultsShown = false;
        return;
      }
    }
    contextMenuSearch = null;
  }

  function clickChip(q: string) {
    query = q;
    inputEl?.focus();
  }

  function deleteRecentSearch(q: string) {
    recentSearches = recentSearches.filter((s) => s !== q);
    localStorage.setItem(
      'deletedSearches',
      JSON.stringify([...JSON.parse(localStorage.getItem('deletedSearches') || '[]'), q]),
    );
    contextMenuSearch = null;
  }

  function deleteAllRecentSearches() {
    localStorage.setItem(
      'deletedSearches',
      JSON.stringify([
        ...JSON.parse(localStorage.getItem('deletedSearches') || '[]'),
        ...recentSearches,
      ]),
    );
    recentSearches = [];
  }

  function showChipContextMenu(e: MouseEvent, q: string) {
    e.preventDefault();
    contextMenuSearch = q;
    contextMenuPos = { x: e.clientX, y: e.clientY };
  }

  function getFaviconSrc(favicon: string | undefined, url: string): string | null {
    if (favicon) return favicon;
    return null;
  }

  async function loadHomeStats() {
    try {
      const statsRes = await apiFetch('/stats', { headers: { Accept: 'application/json' } });

      if (statsRes.ok) {
        const stats = await statsRes.json();
        rulesCount = stats.rule_count;
        aliasesCount = stats.alias_count;
        historyCount = stats.doc_count;
        if (stats.recent_searches) {
          const deletedSearches: string[] = JSON.parse(
            localStorage.getItem('deletedSearches') || '[]',
          );
          recentSearches = stats.recent_searches
            .map((s: { query: string }) => s.query)
            .filter((q: string) => !deletedSearches.includes(q));
        }
      }
    } catch (e) {
      console.log('Failed to retreive stats', e);
    }
    statsLoaded = true;
  }

  let statsLoaded = $state(false);

  function startHeroAnimations() {
    cleanupAnimations();

    if (heroTitleEl) {
      animationHandles.push(
        animate(heroTitleEl, {
          backgroundPosition: ['0% 50%', '100% 50%'],
          ease: 'inOutSine',
          duration: 6000,
          loop: true,
          alternate: true,
        }),
      );
    }

    if (kbdEl) {
      animationHandles.push(
        animate(kbdEl, {
          translateY: [0, 3, 0],
          duration: 400,
          ease: 'inOutSine',
          loop: true,
          loopDelay: 10000,
        }),
      );
    }

    if (underlineEl) {
      animationHandles.push(
        animate(underlineEl, {
          scaleX: [0, 1],
          duration: 800,
          ease: 'outCubic',
          delay: 300,
        }),
      );
    }
  }

  function animateCounters() {
    const counterObj = { h: displayHistoryCount, r: displayRulesCount, a: displayAliasesCount };
    animationHandles.push(
      animate(counterObj, {
        h: historyCount,
        r: rulesCount,
        a: aliasesCount,
        duration: 800,
        ease: 'outCubic',
        onRender: () => {
          displayHistoryCount = Math.round(counterObj.h);
          displayRulesCount = Math.round(counterObj.r);
          displayAliasesCount = Math.round(counterObj.a);
        },
      }),
    );
  }

  function cleanupAnimations() {
    for (const h of animationHandles) {
      try {
        h.revert();
      } catch {}
    }
    animationHandles = [];
  }

  $effect(() => {
    if (!isSearching) {
      tick().then(() => startHeroAnimations());
    }
    return () => cleanupAnimations();
  });

  $effect(() => {
    if (statsLoaded && !isSearching) {
      tick().then(() => animateCounters());
    }
  });

  $effect(() => {
    isSearching;
    (async () => {
      await tick();
      inputEl?.focus();
    })();
  });
  $effect(() => {
    if (query && connected) {
      sendQuery(query);
      localStorage.setItem('lastQuery', query);
    }
  });
  $effect(() => {
    if (!query) {
      autocomplete = '';
      lastResults = null;
      accumulatedDocs = [];
      pageKey = '';
      hasMore = false;
      loadingMoreForQuery = '';
    }
  });
  $effect(() => {
    if (dateFrom || dateTo) sendQuery(query);
  });

  // Persist and react to semantic setting changes.
  $effect(() => {
    localStorage.setItem('hister-semantic-on', String(semanticOn));
    if (query && connected) sendQuery(query);
  });
  $effect(() => {
    localStorage.setItem('hister-semantic-threshold', String(similarityThreshold));
    if (query && connected) sendQuery(query);
  });
  $effect(() => {
    localStorage.setItem('hister-semantic-weight', String(semanticWeight));
  });

  // Auto-load the readability panel for the focused result on desktop.
  // Tracks mergedResults (not just lastResults) so that reordering caused by
  // the semantic weight slider also refreshes the panel.
  // Uses data instead of DOM queries so it works when results are hidden (fullscreen mode).
  $effect(() => {
    const idx = highlightIdx;
    const results = mergedResults; // reactive dependency: reorders trigger this
    const isFullscreen = previewFullscreen;
    if (!isDesktop || !results.length || (!panelOpen && !isFullscreen)) return;
    const result = results[idx];
    if (!result) return;
    const url = result.url;
    if (url === untrack(() => panelUrl)) return;
    panelHintTitle = result.title || '';
    panelUrl = url;
    if (isFullscreen) {
      withSkipUrl(skipUrl, () => replacePreviewHistory(url, result.title || ''));
    }
  });
  $effect(() => {
    updateURL();
  });
  $effect.pre(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const q = urlParams.get('q');
    const df = urlParams.get('date_from');
    const dt = urlParams.get('date_to');
    if (q) query = q;
    if (df) dateFrom = df;
    if (dt) dateTo = dt;
    lastPushedEmpty = !q;
  });

  // IntersectionObserver: load more results when the sentinel comes into view.
  $effect(() => {
    const el = sentinelEl;
    if (!el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMoreForQuery) {
          loadMoreResults();
        }
      },
      { rootMargin: '200px' },
    );
    observer.observe(el);
    return () => observer.disconnect();
  });

  onMount(() => {
    (async () => {
      const appConfig = await fetchConfig();
      const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = new URL(appConfig.wsUrl);
      config = {
        wsUrl: `${wsProto}//${location.host}${wsUrl.pathname}`,
        searchUrl: appConfig.searchUrl,
        openResultsOnNewTab: appConfig.openResultsOnNewTab,
        hotkeys: appConfig.hotkeys,
        semanticEnabled: (appConfig as any).semanticEnabled ?? false,
        similarityThreshold: (appConfig as any).similarityThreshold ?? 0.1,
        semanticWeight: (appConfig as any).semanticWeight ?? 0.4,
      };
      if (config.semanticEnabled) {
        // Apply server defaults only when the user has not yet customised these.
        if (localStorage.getItem('hister-semantic-threshold') === null)
          similarityThreshold = config.similarityThreshold;
        if (localStorage.getItem('hister-semantic-weight') === null)
          semanticWeight = config.semanticWeight;
      }
      inputEl?.focus();
      connect();
      keyHandler = new KeyHandler(config.hotkeys, hotkeyActions);
      loadHomeStats();
    })();
    const mq = window.matchMedia('(min-width: 1280px)');
    isDesktop = mq.matches;
    const stored = localStorage.getItem('hister-panel-open');
    if (stored !== null) panelOpen = stored !== 'false';
    const mqHandler = (e: MediaQueryListEvent) => {
      isDesktop = e.matches;
    };
    mq.addEventListener('change', mqHandler);
    return () => {
      wsManager?.close();
      cleanupAnimations();
      mq.removeEventListener('change', mqHandler);
    };
  });
</script>

<svelte:head>
  <title>{query ? `${query} - Hister search` : 'Hister'}</title>
</svelte:head>

<svelte:window onkeydown={handleKeydown} onpopstate={handlePopState} />

<Dialog.Root bind:open={$showHelp}>
  <Dialog.Content
    showCloseButton={false}
    class="border-border-brand bg-card-surface max-w-md gap-0 overflow-hidden rounded-none border-[3px] p-0 shadow-[6px_6px_0px_var(--hister-indigo)]"
  >
    <Dialog.Header class="bg-hister-indigo flex-row items-center justify-between gap-2 px-5 py-4">
      <Dialog.Title class="flex items-center gap-2">
        <Keyboard class="size-5 text-white" />
        <span class="font-outfit text-lg font-extrabold text-white">Keyboard Shortcuts</span>
      </Dialog.Title>
      <Dialog.Close class="p-0.5 text-white/70 hover:text-white">
        <X class="size-5" />
      </Dialog.Close>
    </Dialog.Header>
    <Card.Content class="space-y-0 p-4">
      {#each Object.entries(config.hotkeys) as [key, action]}
        <div
          class="border-border-brand-muted flex items-center justify-between border-b-[1px] py-2.5"
        >
          <span class="font-inter text-text-brand-secondary"
            >{hotkeyDescriptions[action] || action}</span
          >
          <Kbd
            class="bg-muted-surface border-border-brand-muted font-fira text-text-brand h-auto rounded-none border-[2px] px-2.5 py-0.5 text-xs font-semibold"
            >{key}</Kbd
          >
        </div>
      {/each}
    </Card.Content>
    <Card.Footer class="bg-muted-surface border-border-brand-muted border-t-[2px] px-5 py-3">
      <p class="font-inter text-text-brand-muted text-xs">
        Press <Kbd
          class="bg-card-surface border-border-brand-muted font-fira h-auto rounded-none border px-1.5 py-0.5 text-[10px]"
          >?</Kbd
        > to toggle this dialog
      </p>
    </Card.Footer>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showDeleteConfirm}>
  <Dialog.Content
    escapeKeydownBehavior="ignore"
    showCloseButton={false}
    class="border-border-brand bg-card-surface flex max-h-[80vh] max-w-md flex-col gap-0 overflow-hidden rounded-none border-[3px] p-0 shadow-[6px_6px_0px_black]"
  >
    <Dialog.Header class="bg-hister-rose flex-row items-center justify-between gap-2 px-5 py-4">
      <Dialog.Title class="flex items-center gap-2">
        <Trash2 class="size-5 text-white" />
        <span class="font-outfit text-lg font-extrabold text-white">Delete result</span>
      </Dialog.Title>
    </Dialog.Header>
    <div class="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-4">
      <p class="font-inter text-text-brand-secondary text-sm">
        Are you sure you want to delete this result?
      </p>
      <code
        class="font-fira bg-muted-surface text-text-brand-muted block px-2 py-1 text-xs break-all"
        title={deleteConfirmUrl}>{deleteConfirmUrl}</code
      >
      <label class="flex cursor-pointer items-center gap-2">
        <input type="checkbox" bind:checked={deleteConfirmSkip} class="accent-hister-rose" />
        <span class="font-inter text-text-brand-secondary text-sm">Don't ask again</span>
      </label>
    </div>
    <div class="border-border-brand-muted flex shrink-0 justify-end gap-2 border-t-[3px] px-5 py-3">
      <Button
        variant="outline"
        size="sm"
        class="border-border-brand-muted text-text-brand-secondary rounded-none"
        onclick={cancelDelete}
      >
        No
      </Button>
      <Button
        size="sm"
        class="bg-hister-rose hover:bg-hister-rose/90 rounded-none border-0 text-white"
        onclick={confirmDelete}
      >
        Yes, delete
      </Button>
    </div>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showDeleteAllConfirm}>
  <Dialog.Content
    escapeKeydownBehavior="ignore"
    showCloseButton={false}
    class="border-border-brand bg-card-surface flex max-h-[80vh] max-w-md flex-col gap-0 overflow-hidden rounded-none border-[3px] p-0 shadow-[6px_6px_0px_black]"
  >
    <Dialog.Header class="bg-hister-rose flex-row items-center justify-between gap-2 px-5 py-4">
      <Dialog.Title class="flex items-center gap-2">
        <Trash2 class="size-5 text-white" />
        <span class="font-outfit text-lg font-extrabold text-white"
          >Delete all matching results</span
        >
      </Dialog.Title>
    </Dialog.Header>
    <div class="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-4">
      <p class="font-inter text-text-brand-secondary text-sm">
        Are you sure you want to delete <strong
          >all {lastResults?.total || totalResults} result(s)</strong
        > matching:
      </p>
      <code
        class="font-fira bg-muted-surface text-text-brand-muted block px-2 py-1 text-xs break-all"
        >{query}</code
      >
      <p class="font-inter text-hister-rose text-xs font-semibold">This action cannot be undone.</p>
      {#if dateFrom || dateTo}
        <p class="font-inter text-text-brand-muted text-xs">
          Note: date filters are not applied to deletion — all results matching the text query above
          will be deleted.
        </p>
      {/if}
    </div>
    <div class="border-border-brand-muted flex shrink-0 justify-end gap-2 border-t-[3px] px-5 py-3">
      <Button
        variant="outline"
        size="sm"
        class="border-border-brand-muted text-text-brand-secondary rounded-none"
        onclick={cancelDeleteAll}
      >
        No
      </Button>
      <Button
        size="sm"
        class="bg-hister-rose hover:bg-hister-rose/90 rounded-none border-0 text-white"
        onclick={confirmDeleteAll}
      >
        Yes, delete all
      </Button>
    </div>
  </Dialog.Content>
</Dialog.Root>

{#if isSearching}
  <div class="flex min-h-0 flex-1 flex-col">
    <div
      class="search bg-card-surface border-brutal-border flex h-10 shrink-0 items-center gap-3 border-b-[3px] px-4 md:h-14"
    >
      <Search class="text-text-brand-muted size-4 md:size-6" />
      <Input
        bind:ref={inputEl}
        bind:value={query}
        type="search"
        placeholder="Search..."
        class="font-inter text-text-brand placeholder:text-text-brand-muted h-full flex-1 border-0 bg-transparent p-0 text-lg font-medium shadow-none focus-visible:ring-0 md:text-2xl"
      />
      {#if config.semanticEnabled}
        <Tooltip.Provider delayDuration={0}>
          <Tooltip.Root>
            <Tooltip.Trigger>
              <button
                type="button"
                onclick={() => (semanticOn = !semanticOn)}
                class="flex shrink-0 items-center gap-1 px-1.5 py-0.5 text-xs font-semibold transition-colors {semanticOn
                  ? 'text-hister-indigo'
                  : 'text-text-brand-muted hover:text-hister-indigo'}"
                aria-pressed={semanticOn}
                aria-label="Toggle semantic search"
              >
                <Sparkles class="size-3.5" />
                <span class="hidden md:inline">Semantic</span>
              </button>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content>
                {semanticOn ? 'Semantic search on' : 'Semantic search off'} — click to toggle
              </Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>
      {/if}
      <Tooltip.Provider delayDuration={0}>
        <Tooltip.Root>
          <Tooltip.Trigger>
            <div class="h-3 w-3 shrink-0 {connected ? 'bg-hister-lime' : 'bg-hister-rose'}"></div>
          </Tooltip.Trigger>
          <Tooltip.Portal>
            <Tooltip.Content>
              Server: {connected ? 'Connected' : 'Disconnected'}
            </Tooltip.Content>
          </Tooltip.Portal>
        </Tooltip.Root>
      </Tooltip.Provider>
    </div>
    {#if autocomplete && autocomplete !== query}
      <span class="font-fira text-text-brand-muted mx-8 text-sm">
        Tab: <span class="text-hister-indigo">{autocomplete}</span>
      </span>
    {/if}

    <div class="flex min-h-0 flex-1 overflow-hidden">
      {#if !previewFullscreen}
        <ScrollArea class="min-h-0 flex-1">
          <div class="w-full max-w-[70em] space-y-3 overflow-x-hidden px-3 py-2 md:px-12">
            {#if deleteError}
              <div
                class="border-hister-rose bg-hister-rose/10 text-hister-rose flex items-center justify-between gap-2 border-[2px] px-3 py-2 text-sm"
              >
                <span class="font-inter">{deleteError}</span>
                <button
                  class="shrink-0 cursor-pointer opacity-60 hover:opacity-100"
                  onclick={() => (deleteError = null)}
                  aria-label="Dismiss">✕</button
                >
              </div>
            {/if}
            {#if hasResults}
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-outfit text-hister-indigo text-sm font-bold md:text-base">
                  {semanticOn && config.semanticEnabled
                    ? totalResults
                    : lastResults?.total || totalResults} results{query ? ` for "${query}"` : ''}
                </span>
                <div class="flex items-center gap-2">
                  {#if isDesktop && !panelOpen}
                    <Button
                      variant="ghost"
                      size="sm"
                      class="font-inter text-text-brand-muted hover:text-hister-indigo gap-1 text-xs"
                      onclick={() => {
                        panelOpen = true;
                        localStorage.setItem('hister-panel-open', 'true');
                      }}
                    >
                      <Eye class="size-3" />
                      Preview
                    </Button>
                  {/if}
                  <DropdownMenu.Root>
                    <DropdownMenu.Trigger>
                      {#snippet child({ props })}
                        <Button
                          {...props}
                          variant="ghost"
                          size="sm"
                          class="font-inter text-text-brand-muted hover:text-hister-indigo gap-1 text-xs"
                        >
                          <Filter class="size-3" />
                          Search Actions
                          <ChevronDown class="size-3" />
                        </Button>
                      {/snippet}
                    </DropdownMenu.Trigger>
                    <DropdownMenu.Content
                      class="border-brutal-border bg-card-surface w-80 rounded-none border-[3px] p-3 shadow-[4px_4px_0_var(--brutal-shadow)]"
                    >
                      <div class="space-y-3">
                        <div class="space-y-2">
                          <p
                            class="font-inter text-text-brand-muted flex items-center gap-1.5 text-xs font-semibold"
                          >
                            <Calendar class="size-3" />
                            Date Filter
                          </p>
                          <div class="flex flex-col gap-2">
                            <label
                              class="font-inter text-text-brand-secondary flex items-center gap-1.5 text-xs"
                            >
                              From:
                              <Input
                                type="date"
                                bind:value={dateFrom}
                                class="border-border-brand-muted bg-card-surface text-text-brand font-fira focus-visible:border-hister-indigo h-7 flex-1 border-[2px] px-2 text-xs shadow-none focus-visible:ring-0"
                              />
                            </label>
                            <label
                              class="font-inter text-text-brand-secondary flex items-center gap-1.5 text-xs"
                            >
                              To:
                              <Input
                                type="date"
                                bind:value={dateTo}
                                class="border-border-brand-muted bg-card-surface text-text-brand font-fira focus-visible:border-hister-indigo h-7 flex-1 border-[2px] px-2 text-xs shadow-none focus-visible:ring-0"
                              />
                            </label>
                          </div>
                        </div>
                        <Separator class="bg-border-brand-muted" />
                        {#if config.semanticEnabled && semanticOn}
                          <div class="space-y-2">
                            <p
                              class="font-inter text-text-brand-muted flex items-center gap-1.5 text-xs font-semibold"
                            >
                              <Sparkles class="size-3" />
                              Semantic Search
                            </p>
                            <label
                              class="font-inter text-text-brand-secondary flex flex-col gap-1 text-xs"
                            >
                              <span
                                >Similarity threshold: <span class="font-fira text-hister-indigo"
                                  >{similarityThreshold.toFixed(2)}</span
                                ></span
                              >
                              <input
                                type="range"
                                min="0"
                                max="1"
                                step="0.002"
                                bind:value={similarityThreshold}
                                class="accent-hister-indigo w-full cursor-pointer"
                              />
                            </label>
                            <label
                              class="font-inter text-text-brand-secondary flex flex-col gap-1 text-xs"
                            >
                              <span
                                >Semantic weight: <span class="font-fira text-hister-indigo"
                                  >{semanticWeight.toFixed(2)}</span
                                ></span
                              >
                              <input
                                type="range"
                                min="0"
                                max="1"
                                step="0.05"
                                bind:value={semanticWeight}
                                class="accent-hister-indigo w-full cursor-pointer"
                              />
                            </label>
                          </div>
                          <Separator class="bg-border-brand-muted" />
                        {/if}
                        <div class="space-y-2">
                          <p
                            class="font-inter text-text-brand-muted flex items-center gap-1.5 text-xs font-semibold"
                          >
                            <Download class="size-3" />
                            Export Results
                          </p>
                          <div class="flex flex-wrap gap-2">
                            <Button
                              variant="outline"
                              size="sm"
                              class="border-hister-indigo text-hister-indigo hover:bg-hister-indigo/10 h-7 border-[2px] text-xs"
                              onclick={() =>
                                exportJSON({ ...lastResults!, documents: accumulatedDocs })}
                            >
                              JSON
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              class="border-hister-indigo text-hister-indigo hover:bg-hister-indigo/10 h-7 border-[2px] text-xs"
                              onclick={() =>
                                exportCSV({ ...lastResults!, documents: accumulatedDocs }, query)}
                            >
                              CSV
                            </Button>
                            <Button
                              variant="outline"
                              size="sm"
                              class="border-hister-indigo text-hister-indigo hover:bg-hister-indigo/10 h-7 border-[2px] text-xs"
                              onclick={() =>
                                exportRSS({ ...lastResults!, documents: accumulatedDocs }, query)}
                            >
                              RSS
                            </Button>
                          </div>
                        </div>
                        <Separator class="bg-border-brand-muted" />
                        <div class="space-y-2">
                          <p
                            class="font-inter text-hister-rose flex items-center gap-1.5 text-xs font-semibold"
                          >
                            <Trash2 class="size-3" />
                            Danger Zone
                          </p>
                          <Button
                            variant="outline"
                            size="sm"
                            class="border-hister-rose text-hister-rose hover:bg-hister-rose/10 h-7 w-full border-[2px] text-xs"
                            onclick={() => {
                              showDeleteAllConfirm = true;
                            }}
                          >
                            <Trash2 class="size-3" />
                            Delete all matching results
                          </Button>
                        </div>
                      </div>
                    </DropdownMenu.Content>
                  </DropdownMenu.Root>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="font-inter text-text-brand-muted hover:text-hister-coral gap-1 text-xs no-underline"
                    href={getSearchUrl(config.searchUrl, query)}
                  >
                    <ExternalLink class="size-3" />
                    Web
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="font-inter text-hister-indigo hover:text-hister-coral text-xs"
                    onclick={() => setSort(currentSort === '' ? 'domain' : '')}
                  >
                    Sort: {currentSort === 'domain' ? 'Domain' : 'Relevance'}
                  </Button>
                </div>
              </div>

              {#if lastResults?.query && lastResults.query.text.length > query.length}
                <p class="font-inter text-text-brand-muted text-sm">
                  Expanded query: <code
                    class="font-fira bg-muted-surface text-primary px-1.5 py-0.5 text-xs"
                    >{lastResults.query.text}</code
                  >
                </p>
              {/if}

              {#if lastResults?.history?.length}
                {#each lastResults.history as r, i}
                  {@const favSrc = getFaviconSrc(r.favicon, r.url)}
                  <article
                    data-result
                    class="flex w-full scroll-my-[6em] gap-3 overflow-hidden py-3.5 transition-all duration-150"
                    style={i === highlightIdx
                      ? 'background: linear-gradient(90deg, transparent, rgba(90, 138, 138, 0.12), transparent); border-left: 3px solid var(--hister-teal); padding-left: 0.75rem;'
                      : ''}
                  >
                    <div class="w-0 min-w-0 flex-1 space-y-0.5">
                      <div class="flex items-center gap-1.5">
                        <div
                          class="bg-hister-teal flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden"
                        >
                          {#if favSrc}
                            <img
                              src={favSrc}
                              alt=""
                              class="h-full w-full object-cover"
                              onload={(e) => {
                                (
                                  e.target as HTMLImageElement
                                ).parentElement!.style.backgroundColor = 'transparent';
                              }}
                              onerror={(e) => {
                                (e.target as HTMLImageElement).style.display = 'none';
                                (e.target as HTMLImageElement).nextElementSibling?.classList.remove(
                                  'hidden',
                                );
                              }}
                            />
                            <Star class="hidden size-3 text-white" />
                          {:else}
                            <Star class="size-3 text-white" />
                          {/if}
                        </div>
                        <a
                          data-result-link={r.url}
                          href={fileResultUrl(r.url)}
                          class="font-outfit text-md text-hister-teal min-w-0 flex-1 font-semibold hover:underline md:overflow-hidden md:text-xl"
                          target={config.openResultsOnNewTab ? '_blank' : undefined}
                          onclick={() => {
                            sendHistoryBeacon(r.url, r.title || '*title*', query);
                          }}
                          onauxclick={(e) => {
                            if (e.button === 1)
                              sendHistoryBeacon(r.url, r.title || '*title*', query);
                          }}
                        >
                          {r.title || '*title*'}
                        </a>
                        <DropdownMenu.Root>
                          <DropdownMenu.Trigger>
                            {#snippet child({ props })}
                              <Button
                                {...props}
                                variant="ghost"
                                size="icon-sm"
                                class="text-text-brand-muted hover:text-text-brand shrink-0 cursor-pointer"
                                onclick={() => {
                                  actionsMessage = null;
                                  actionsError = false;
                                }}
                              >
                                <MoreVertical class="size-4" />
                              </Button>
                            {/snippet}
                          </DropdownMenu.Trigger>
                          <DropdownMenu.Content
                            class="border-brutal-border bg-card-surface w-72 rounded-none border-[3px] p-3 shadow-[4px_4px_0_var(--brutal-shadow)]"
                          >
                            <div class="space-y-3">
                              <div class="space-y-2">
                                <p
                                  class="font-outfit text-text-brand-muted mb-1 text-xs font-bold tracking-widest uppercase"
                                >
                                  Priority
                                </p>
                                <Button
                                  variant="outline"
                                  size="sm"
                                  class="border-hister-rose text-hister-rose hover:bg-hister-rose/10 w-full border-[2px] text-xs"
                                  onclick={() =>
                                    updatePriorityResult(r.url, r.title || '*title*', true)}
                                >
                                  <PinOff class="size-3.5" />
                                  Unpin
                                </Button>
                              </div>
                              <SkipRuleActions
                                onAddSkipRule={(type, deleteMatches) =>
                                  addSkipRuleForResult(r.url, r.domain, type, deleteMatches)}
                              />
                              {#if actionsMessage}
                                <p
                                  class="font-inter text-xs {actionsError
                                    ? 'text-hister-rose'
                                    : 'text-hister-teal'}"
                                >
                                  {actionsMessage}
                                </p>
                              {/if}
                            </div>
                          </DropdownMenu.Content>
                        </DropdownMenu.Root>
                      </div>
                      <div class="flex items-center gap-2">
                        <span
                          class="font-fira text-hister-teal truncate overflow-hidden text-ellipsis whitespace-nowrap"
                          >{r.url}</span
                        >
                        <Badge
                          variant="secondary"
                          class="bg-hister-teal/10 text-hister-teal h-4 border-0 px-1.5 py-0"
                          >pinned</Badge
                        >
                        <Button
                          data-readable
                          variant="link"
                          size="sm"
                          class="text-hister-indigo h-auto shrink-0 cursor-pointer gap-0.5 p-0 text-xs font-medium md:text-sm"
                          onclick={(e) => {
                            highlightIdx = i;
                            openReadable(e, r.url, r.title || '*title*');
                          }}
                        >
                          <Eye class="size-3" /><span>view</span>
                        </Button>
                      </div>
                      {#if r.text}
                        <p
                          class="font-inter text-text-brand-secondary text-sm leading-[1.4] md:text-base"
                        >
                          {@html r.text}
                        </p>
                      {/if}
                    </div>
                  </article>
                {/each}
              {/if}

              {#if mergedResults.length > 0}
                {#each mergedResults as r, i}
                  {@const idx = historyLen + i}
                  {@const color = 'hister-cyan'}
                  {@const favSrc = getFaviconSrc(r.favicon, r.url)}
                  {@const isSemOnly = r.sourceType === 'semantic'}
                  <article
                    data-result
                    class="flex w-full scroll-my-[6em] gap-3 overflow-hidden py-3.5 transition-all duration-150"
                    style={idx === highlightIdx
                      ? `background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--${color}) 12%, transparent), transparent); border-left: 3px solid var(--${color}); padding-left: 0.75rem;`
                      : ''}
                  >
                    <div class="w-0 min-w-0 flex-1 space-y-0.5">
                      <div class="flex items-center gap-1.5">
                        <div
                          class="flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden"
                          style="background-color: var(--{color});"
                        >
                          {#if favSrc}
                            <img
                              src={favSrc}
                              alt=""
                              class="h-full w-full object-cover"
                              onload={(e) => {
                                (
                                  e.target as HTMLImageElement
                                ).parentElement!.style.backgroundColor = 'transparent';
                              }}
                              onerror={(e) => {
                                (e.target as HTMLImageElement).style.display = 'none';
                                (e.target as HTMLImageElement).nextElementSibling?.classList.remove(
                                  'hidden',
                                );
                              }}
                            />
                            <Globe class="hidden size-3 text-white" />
                          {:else}
                            <Globe class="size-3 text-white" />
                          {/if}
                        </div>
                        <a
                          data-result-link={r.url}
                          href={fileResultUrl(r.url)}
                          class="font-outfit text-md min-w-0 flex-1 font-semibold hover:underline md:text-xl"
                          style="color: var(--{color});"
                          target={config.openResultsOnNewTab ? '_blank' : undefined}
                          onclick={() => {
                            sendHistoryBeacon(r.url, r.title || '*title*', query);
                          }}
                          onauxclick={(e) => {
                            if (e.button === 1)
                              sendHistoryBeacon(r.url, r.title || '*title*', query);
                          }}
                        >
                          {r.title || '*title*'}
                        </a>
                        <DropdownMenu.Root>
                          <DropdownMenu.Trigger>
                            {#snippet child({ props })}
                              <Button
                                {...props}
                                variant="ghost"
                                size="icon-sm"
                                class="text-text-brand-muted hover:text-text-brand shrink-0 cursor-pointer"
                                onclick={() => {
                                  actionsMessage = null;
                                  actionsError = false;
                                }}
                              >
                                <MoreVertical class="size-4" />
                              </Button>
                            {/snippet}
                          </DropdownMenu.Trigger>
                          <DropdownMenu.Content
                            class="border-brutal-border bg-card-surface w-100 rounded-none border-[3px] p-3 shadow-[4px_4px_0_var(--brutal-shadow)]"
                          >
                            <div class="space-y-3">
                              <div class="space-y-2">
                                <p
                                  class="font-outfit mb-1 text-xs font-bold tracking-widest uppercase"
                                >
                                  Prioritize this result in query:
                                </p>
                                <div class="flex items-center gap-2">
                                  <Input
                                    bind:value={actionsQuery}
                                    placeholder="Query.."
                                    size="sm"
                                    class="font-inter border-border-brand-muted focus-visible:border-hister-indigo flex-1 border-[2px] text-sm shadow-none focus-visible:ring-0"
                                  />
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    class="border-hister-indigo text-hister-indigo border-[2px] text-xs"
                                    onclick={() =>
                                      updatePriorityResult(r.url, r.title || '*title*', false)}
                                  >
                                    <Pin class="size-3.5" />
                                    Pin
                                  </Button>
                                </div>
                                <hr />
                              </div>
                              <SkipRuleActions
                                onAddSkipRule={(type, deleteMatches) =>
                                  addSkipRuleForResult(r.url, r.domain, type, deleteMatches)}
                              />
                              <hr />
                              <Button
                                variant="outline"
                                size="sm"
                                class="border-hister-rose text-hister-rose hover:bg-hister-rose/10 w-full border-[2px] text-xs"
                                onclick={() => deleteResult(r.url)}
                              >
                                <Trash2 class="size-3.5" />
                                Delete result
                              </Button>
                              {#if actionsMessage}
                                <p
                                  class="font-inter text-xs {actionsError
                                    ? 'text-hister-rose'
                                    : 'text-hister-teal'}"
                                >
                                  {actionsMessage}
                                </p>
                              {/if}
                            </div>
                          </DropdownMenu.Content>
                        </DropdownMenu.Root>
                      </div>
                      <div class="flex items-center gap-2">
                        <span
                          class="font-fira text-hister-teal truncate overflow-hidden text-xs text-ellipsis whitespace-nowrap md:text-sm"
                          >{r.url}</span
                        >
                        {#if r.added}
                          <span
                            class="font-inter text-text-brand-muted text-xs whitespace-nowrap md:text-sm"
                            title={formatTimestamp(r.added)}>· {formatRelativeTime(r.added)}</span
                          >
                        {/if}
                        <Button
                          data-readable
                          variant="link"
                          size="sm"
                          class="text-hister-indigo h-auto shrink-0 cursor-pointer gap-0.5 p-0 text-xs font-medium md:text-sm"
                          onclick={(e) => {
                            highlightIdx = idx;
                            openReadable(e, r.url, r.title || '*title*');
                          }}
                        >
                          <Eye class="size-3" /><span>view</span>
                        </Button>
                        {#if r.finalScore && config.semanticEnabled && semanticOn}
                          <Tooltip.Provider delayDuration={0}>
                            <Tooltip.Root>
                              <Tooltip.Trigger>
                                <Badge
                                  variant="secondary"
                                  class="bg-hister-indigo/10 text-hister-indigo shrink-0 border-0 px-1.5 py-0 align-middle font-mono text-[10px]"
                                  >{r.finalScore?.toFixed(2)}</Badge
                                >
                              </Tooltip.Trigger>
                              <Tooltip.Portal>
                                <Tooltip.Content>
                                  Result score: {r.finalScore?.toFixed(2)}
                                </Tooltip.Content>
                              </Tooltip.Portal>
                            </Tooltip.Root>
                          </Tooltip.Provider>
                        {/if}
                      </div>
                      {#if r.text}
                        <p
                          class="font-inter text-text-brand-secondary text-sm leading-[1.4] md:text-base"
                        >
                          {@html r.text}
                        </p>
                      {/if}
                    </div>
                  </article>
                {/each}
              {/if}
            {:else if query && lastResults}
              <section class="pmd:px-12 y-12 text-center">
                <p class="font-inter text-text-brand-secondary mb-4">
                  No results found for "<span class="font-semibold">{query}</span>"
                </p>
                <Button
                  variant="outline"
                  class="border-hister-coral text-hister-coral hover:bg-hister-coral/10 font-inter border-[3px] font-semibold shadow-[3px_3px_0px_var(--hister-coral)]"
                  href={getSearchUrl(config.searchUrl, query)}
                >
                  <ExternalLink class="size-4" />
                  Search
                </Button>
              </section>
            {:else if query}
              <div class="flex items-center justify-center py-16">
                <span class="font-inter text-text-brand-muted">Searching...</span>
              </div>
            {/if}
            {#if hasMore || loadingMoreForQuery}
              <div bind:this={sentinelEl} class="flex items-center justify-center py-4">
                <span class="font-inter text-text-brand-muted text-sm">Loading more…</span>
              </div>
            {:else if hasResults}
              <div bind:this={sentinelEl}></div>
            {/if}
          </div>
        </ScrollArea>
      {/if}

      <!-- Preview panel: fullscreen (both mobile and desktop) or split-pane (desktop only) -->
      {#if previewFullscreen}
        <PreviewPanel
          url={panelUrl}
          hintTitle={panelHintTitle}
          fullscreen={true}
          onclose={closePanelAndFullscreen}
          onfullscreentoggle={isDesktop ? exitFullscreen : undefined}
        />
      {:else if lastResults && panelOpen && isDesktop}
        <PreviewPanel
          url={panelUrl}
          hintTitle={panelHintTitle}
          fullscreen={false}
          onclose={() => {
            panelOpen = false;
            localStorage.setItem('hister-panel-open', 'false');
          }}
          onfullscreentoggle={enterFullscreen}
        />
      {/if}
    </div>
  </div>
{:else}
  <div
    class="relative flex flex-1 flex-col items-center gap-5 overflow-y-auto px-4 py-4 md:gap-10 md:px-12 md:py-12"
  >
    <h1
      bind:this={heroTitleEl}
      class="font-outfit bg-clip-text text-5xl leading-none font-black tracking-[8px] text-transparent uppercase select-none md:text-9xl"
      style="background-image: linear-gradient(90deg, var(--hister-indigo), var(--hister-coral), var(--hister-teal), var(--hister-indigo)); background-size: 300% 100%; background-position: 0% 50%;"
    >
      Hister
    </h1>

    <p class="font-inter text-md text-text-brand-secondary md:text-lg">Your own search engine</p>
    <div
      bind:this={underlineEl}
      class="h-[3px] w-48"
      style="background: linear-gradient(90deg, var(--hister-indigo), var(--hister-coral), var(--hister-teal)); transform: scaleX(0); transform-origin: left;"
    ></div>

    <div
      bind:this={searchBoxEl}
      class="search-box-gradient w-full max-w-[1200px] p-[3px] shadow-[4px_4px_0px_var(--hister-coral)]"
    >
      <div class="bg-card-surface flex h-10 items-center gap-3 pl-4 md:h-14">
        <Search class="text-text-brand-muted size-6" />
        <Input
          bind:ref={inputEl}
          bind:value={query}
          type="search"
          placeholder="Search ..."
          class="font-inter text-text-brand placeholder:text-text-brand-muted h-full min-w-0 flex-1 border-0 bg-transparent p-0 shadow-none focus-visible:ring-0 md:text-lg"
        />
        <Tooltip.Provider delayDuration={0}>
          <Tooltip.Root>
            <Tooltip.Trigger class="mr-4">
              <div class="h-3 w-3 shrink-0 {connected ? 'bg-hister-lime' : 'bg-hister-rose'}"></div>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content>
                Server: {connected ? 'Connected' : 'Disconnected'}
              </Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>
      </div>
    </div>

    <div
      bind:this={hintEl}
      class="font-inter text-text-brand-muted hidden items-center gap-1 text-xs md:flex md:gap-2"
    >
      <span>Pro tip: Press</span>
      <Kbd
        bind:ref={kbdEl}
        class="bg-muted-surface border-border-brand-muted font-fira text-text-brand-secondary rounded-none border-[2px] px-2 py-0.5 text-xs font-semibold"
        >/</Kbd
      >
      <span>to focus search anywhere</span>
    </div>

    {#if recentSearches.length > 0}
      <div
        bind:this={chipsContainerEl}
        class="relative flex flex-wrap items-center justify-center gap-3"
      >
        {#each recentSearches as search, i}
          {@const chip = chipColors[i % chipColors.length]}
          <Button
            variant="outline"
            class="border-[3px] {chip.border} {chip.bg} font-inter px-3.5 py-1.5 text-sm font-semibold {chip.text} brutal-press h-auto rounded-none"
            onclick={() => clickChip(search)}
            oncontextmenu={(e) => showChipContextMenu(e, search)}
          >
            {search}
          </Button>
        {/each}
        <Button
          variant="ghost"
          size="sm"
          class="border-hister-rose/40 font-inter text-hister-rose/60 hover:text-hister-rose hover:border-hister-rose hover:bg-hister-rose/10 h-auto rounded-none border-[2px] px-2.5 py-1.5 text-xs font-semibold transition-all duration-200"
          onclick={deleteAllRecentSearches}
          title="Clear all recent searches"
        >
          &times; clear
        </Button>
      </div>
    {/if}

    {#if contextMenuSearch}
      <div
        class="fixed inset-0 z-40"
        role="presentation"
        onclick={() => {
          contextMenuSearch = null;
        }}
        oncontextmenu={(e) => {
          e.preventDefault();
          contextMenuSearch = null;
        }}
      ></div>
      <div
        class="border-brutal-border bg-card-surface fixed z-50 min-w-[160px] border-[3px] py-1 shadow-[4px_4px_0_var(--brutal-shadow)]"
        style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
      >
        <Button
          variant="ghost"
          class="font-inter text-text-brand hover:bg-muted-surface h-auto w-full justify-start gap-2 rounded-none px-3 py-2 text-sm"
          onclick={() => {
            clickChip(contextMenuSearch!);
            contextMenuSearch = null;
          }}
        >
          <Search class="size-3.5" /> Search "{contextMenuSearch}"
        </Button>
        <Separator class="bg-border-brand-muted mx-2" />
        <Button
          variant="ghost"
          class="font-inter text-hister-rose hover:bg-hister-rose/10 h-auto w-full justify-start gap-2 rounded-none px-3 py-2 text-sm"
          onclick={() => deleteRecentSearch(contextMenuSearch!)}
        >
          <Trash2 class="size-3.5" /> Remove
        </Button>
      </div>
    {/if}

    <div bind:this={statsRowEl} class="flex flex-col items-center gap-3 md:flex-row md:gap-8">
      <div
        class="border-brutal-border shadow-brutal-sm flex items-center gap-2 border-[3px] px-4 py-2"
        style="color: var(--hister-indigo);"
      >
        <History class="size-3 md:size-4.5" />
        <span class="font-outfit text-xl font-extrabold">{displayHistoryCount}</span>
        <span class="font-inter text-sm">indexed pages</span>
      </div>
      <div
        class="border-brutal-border shadow-brutal-sm flex items-center gap-2 border-[3px] px-4 py-2"
        style="color: var(--hister-teal);"
      >
        <Shield class="size-3 md:size-4.5" />
        <span class="font-outfit text-xl font-extrabold">{displayRulesCount}</span>
        <span class="font-inter text-sm">active rules</span>
      </div>
      <div
        class="border-brutal-border shadow-brutal-sm flex items-center gap-2 border-[3px] px-4 py-2"
        style="color: var(--hister-coral);"
      >
        <Link2 class="size-3 md:size-4.5" />
        <span class="font-outfit text-xl font-extrabold">{displayAliasesCount}</span>
        <span class="font-inter text-sm">aliases</span>
      </div>
    </div>
  </div>
{/if}

<style>
  .search-box-gradient {
    background: linear-gradient(
      90deg,
      var(--hister-indigo),
      var(--hister-coral),
      var(--hister-teal),
      var(--hister-indigo)
    );
    background-size: 300% 100%;
    animation: gradient-slide 6s ease-in-out infinite alternate;
  }
  @keyframes gradient-slide {
    0% {
      background-position: 0% 50%;
    }
    100% {
      background-position: 100% 50%;
    }
  }
</style>
