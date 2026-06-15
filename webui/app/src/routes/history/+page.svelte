<script lang="ts">
  import {
    buildPreviewUrl,
    pushPreviewHistory,
    replacePreviewHistory,
    withSkipUrl,
    createResizeHandler,
  } from '$lib/preview';
  import { onMount, untrack } from 'svelte';
  import { fetchConfig, apiFetch, getUserId } from '$lib/api';
  import { base } from '$app/paths';
  import { formatTimestamp, formatRelativeTime, KeyHandler, scrollTo } from '$lib/search';
  import type { HistoryItem } from '$lib/types';
  import { Button } from '@hister/components/ui/button';
  import { Input } from '@hister/components/ui/input';
  import { Badge } from '@hister/components/ui/badge';
  import { Separator } from '@hister/components/ui/separator';
  import { ScrollArea } from '@hister/components/ui/scroll-area';
  import { PageHeader } from '@hister/components';
  import { StatusMessage, PreviewPanel } from '$lib/components';
  import { CalendarDays, Clock, Eye, ListFilter, Rss, Search, Trash2, X } from '@lucide/svelte';

  let items: HistoryItem[] = $state([]);
  let loading = $state(true);
  let error = $state('');
  let filter = $state('');
  let pageKey = $state('');
  let openedLastID = $state(0);
  let activeGroup = $state('');
  let filterByDate = $state('');
  let openedOnly = $state(
    typeof localStorage !== 'undefined'
      ? localStorage.getItem('historyOpenedOnly') === 'true'
      : false,
  );

  // Keyboard navigation
  let keyHandler: KeyHandler | undefined;
  let openResultsOnNewTab = $state(false);
  let highlightIdx = $state(0);

  // Preview state
  let isDesktop = $state(false);
  let panelUrl = $state('');
  let panelHintTitle = $state('');
  let panelOpen = $state(
    typeof localStorage !== 'undefined'
      ? localStorage.getItem('hister-history-panel-open') !== 'false'
      : true,
  );
  let previewFullscreen = $state(false);
  let disablePreviews = $state(false);
  const skipUrl = { value: false };
  let panelWidthPct = $state(parseFloat(localStorage.getItem('hister-panel-width') ?? '') || 50);
  let splitContainerEl: HTMLDivElement | undefined = $state();
  const startPanelResize = createResizeHandler({
    getContainer: () => splitContainerEl,
    onWidth: (pct) => {
      panelWidthPct = pct;
    },
    onDone: (pct) => {
      localStorage.setItem('hister-panel-width', String(pct));
    },
  });

  // --- History state helpers ---

  function pushHistoryPageHistory() {
    history.pushState({ type: 'history' }, '', base + '/history');
  }

  $effect(() => {
    const mq = window.matchMedia('(min-width: 1280px)');
    isDesktop = mq.matches;
    const handler = (e: MediaQueryListEvent) => {
      isDesktop = e.matches;
    };
    mq.addEventListener('change', handler);
    return () => mq.removeEventListener('change', handler);
  });

  function openPreview(url: string, title: string) {
    if (isDesktop) {
      if (!panelOpen) {
        panelOpen = true;
        localStorage.setItem('hister-history-panel-open', 'true');
      }
      panelHintTitle = title;
      panelUrl = url;
      return;
    }
    panelUrl = url;
    panelHintTitle = title;
    previewFullscreen = true;
    withSkipUrl(skipUrl, () => pushPreviewHistory(url, title));
  }

  function enterFullscreen() {
    previewFullscreen = true;
    withSkipUrl(skipUrl, () => pushPreviewHistory(panelUrl, panelHintTitle));
  }

  function exitFullscreen() {
    previewFullscreen = false;
    withSkipUrl(skipUrl, () => pushHistoryPageHistory());
  }

  function closePanelAndFullscreen() {
    previewFullscreen = false;
    panelOpen = false;
    localStorage.setItem('hister-history-panel-open', 'false');
    withSkipUrl(skipUrl, () => pushHistoryPageHistory());
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
  }

  $effect(() => {
    localStorage.setItem('historyOpenedOnly', String(openedOnly));
  });

  const groupColors = [
    'hister-indigo',
    'hister-coral',
    'hister-teal',
    'hister-amber',
    'hister-rose',
    'hister-cyan',
    'hister-lime',
  ];

  function getColorVar(color: string): string {
    return `var(--${color})`;
  }

  function formatDateLabel(timestamp: int): string {
    if (!timestamp) {
      return 'Unknown';
    }
    const date = new Date(timestamp * 1000);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(today);
    yesterday.setDate(yesterday.getDate() - 1);
    const itemDate = new Date(date.getFullYear(), date.getMonth(), date.getDate());

    if (itemDate.getTime() === today.getTime()) return 'Today';
    if (itemDate.getTime() === yesterday.getTime()) return 'Yesterday';
    return itemDate.toLocaleDateString(undefined, {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  }

  function getDateKey(timestamp: int): string {
    if (!timestamp) {
      return 'unknown';
    }
    const date = new Date(timestamp * 1000);
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
  }

  const filteredItems = $derived.by(() => {
    let result = items;
    if (filter) {
      const f = filter.toLowerCase();
      result = result.filter(
        (item) => item.title.toLowerCase().includes(f) || item.url.toLowerCase().includes(f),
      );
    }
    if (filterByDate) {
      result = result.filter((item) => item.added && getDateKey(item.added) === filterByDate);
    }
    return result;
  });

  function groupByDate(
    sourceItems: HistoryItem[],
  ): { key: string; label: string; items: HistoryItem[] }[] {
    const g: { key: string; label: string; items: HistoryItem[] }[] = [];
    const seen = new Map<string, number>();
    for (const item of sourceItems) {
      const key = getDateKey(item.added);
      const label = formatDateLabel(item.added);
      if (seen.has(key)) {
        g[seen.get(key)!].items.push(item);
      } else {
        seen.set(key, g.length);
        g.push({ key, label, items: [item] });
      }
    }
    return g;
  }

  const allGroups = $derived.by(() => {
    let baseItems = items;
    if (filter) {
      const f = filter.toLowerCase();
      baseItems = baseItems.filter(
        (item) => item.title.toLowerCase().includes(f) || item.url.toLowerCase().includes(f),
      );
    }
    return groupByDate(baseItems);
  });

  const groups = $derived.by(() => groupByDate(filteredItems));
  const allCount = $derived(filteredItems.length);

  function getGroupColor(idx: number): string {
    return groupColors[idx % groupColors.length];
  }

  function getGlobalGroupColor(key: string): string {
    let idx = 0;
    for (const i in allGroups) {
      if (allGroups[i].key == key) {
        idx = i;
        break;
      }
    }
    return groupColors[idx % groupColors.length];
  }

  function scrollToGroup(key: string) {
    activeGroup = key;
    filterByDate = key;
    if (sentinel) {
      getScrollParent(sentinel.parentElement)?.scrollTo({ top: 0, behavior: 'instant' });
    }
  }

  function showAll() {
    filterByDate = '';
    activeGroup = groups.length > 0 ? groups[0].key : '';
    if (sentinel) {
      getScrollParent(sentinel.parentElement)?.scrollTo({ top: 0, behavior: 'instant' });
    }
  }

  function clearFilter() {
    filter = '';
  }

  function groupCount(group: { key: string; items: HistoryItem[] }): number {
    return group.items.length;
  }

  async function loadItems(latest: string = '') {
    loading = true;
    try {
      await fetchConfig();
      let url = '/history';
      if (openedOnly) {
        url += '?opened=true';
        if (latest) {
          url += '&last_id=' + encodeURIComponent(latest);
        }
      } else if (latest) {
        url += '?last=' + encodeURIComponent(latest);
      }
      const res = await apiFetch(url, {
        headers: { Accept: 'application/json' },
      });
      if (!res.ok) throw new Error('Failed to load history');
      const resJSON = await res.json();
      if (resJSON && resJSON.documents) {
        if (!latest) {
          items = resJSON.documents;
        } else {
          items.push(...resJSON.documents);
        }
        if (openedOnly) {
          openedLastID = resJSON.last_id ?? 0;
        } else {
          pageKey = resJSON.page_key ?? '';
        }
      } else {
        pageKey = '';
        openedLastID = 0;
      }
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    openedOnly;
    openedLastID = 0;
    pageKey = '';
    loadItems();
  });

  async function loadMore() {
    if (openedOnly) {
      loadItems(String(openedLastID));
    } else {
      loadItems(pageKey);
    }
  }

  const hasMore = $derived((!openedOnly && pageKey !== '') || (openedOnly && openedLastID > 0));
  const rssUrl = $derived(`api/history?${openedOnly ? 'opened=true&' : ''}format=rss`);

  // Allow autoscroll only when no date filter is active, or when there are no
  // items from dates earlier than the selected date loaded yet.
  const canAutoLoad = $derived(
    !filterByDate || !items.some((item) => item.added && getDateKey(item.added) < filterByDate),
  );

  let sentinel: HTMLDivElement | undefined = $state();
  let sentinelVisible = $state(false);

  function getScrollParent(el: HTMLElement | null): HTMLElement | null {
    while (el && el !== document.documentElement) {
      const style = getComputedStyle(el);
      if (/auto|scroll/.test(style.overflow + style.overflowY + style.overflowX)) return el;
      el = el.parentElement;
    }
    return null;
  }

  $effect(() => {
    if (!sentinel) return;
    const root = getScrollParent(sentinel.parentElement);
    const observer = new IntersectionObserver(
      (entries) => {
        sentinelVisible = entries[0].isIntersecting;
      },
      { root, threshold: 0 },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  });

  $effect(() => {
    if (sentinelVisible && !loading && hasMore && untrack(() => canAutoLoad)) {
      loadMore();
    }
  });

  async function deleteItem(item: HistoryItem) {
    try {
      if (openedOnly) {
        await apiFetch('/history', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query: item.query, url: item.url, delete: true }),
        });
      } else {
        await apiFetch('/delete', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            query: 'url:' + item.url + (getUserId() !== undefined ? ' user_id:' + getUserId() : ''),
          }),
        });
      }
      items = items.filter((i) => i.url !== item.url);
    } catch (e) {
      error = String(e);
    }
  }

  function selectNthResult(n: number) {
    if (!filteredItems.length) return;
    highlightIdx = (highlightIdx + n + filteredItems.length) % filteredItems.length;
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

  function openSelectedResult(e?: KeyboardEvent, _isInputFocus?: boolean, newWindow = false) {
    if (e) e.preventDefault();
    const item = filteredItems[highlightIdx];
    if (!item) return;
    if (openResultsOnNewTab) newWindow = true;
    window.open(item.url, newWindow ? '_blank' : '_self');
  }

  function viewResultPopup(e?: KeyboardEvent) {
    if (e) e.preventDefault();
    const item = filteredItems[highlightIdx];
    if (!item) return;
    if (isDesktop) {
      if (previewFullscreen) {
        exitFullscreen();
      } else if (panelOpen) {
        enterFullscreen();
      } else {
        openPreview(item.url, item.title || item.url);
      }
    } else {
      if (previewFullscreen) {
        closePanelAndFullscreen();
      } else {
        openPreview(item.url, item.title || item.url);
      }
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement;
    const isInputFocus =
      ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName) || target.isContentEditable;
    keyHandler?.handle(e, isInputFocus);
  }

  const hotkeyActions: Record<
    string,
    (e?: KeyboardEvent, isInputFocus?: boolean) => void | boolean
  > = {
    open_result: openSelectedResult,
    open_result_in_new_tab: (e?: KeyboardEvent, i?: boolean) => openSelectedResult(e, i, true),
    select_next_result: selectNextResult,
    select_previous_result: selectPreviousResult,
    view_result_popup: viewResultPopup,
  };

  onMount(async () => {
    const cfg = await fetchConfig();
    openResultsOnNewTab = (cfg as any).openResultsOnNewTab ?? false;
    keyHandler = new KeyHandler((cfg as any).hotkeys ?? {}, hotkeyActions);
    disablePreviews = (cfg as any).disablePreviews ?? false;
  });

  // Reset highlight when filtered list changes
  $effect(() => {
    filteredItems;
    highlightIdx = 0;
  });

  // Auto-update panel on desktop when focused item changes.
  // Uses data so it works even when results are hidden in fullscreen mode.
  $effect(() => {
    const idx = highlightIdx;
    filteredItems;
    const isFullscreen = previewFullscreen;
    if (!isDesktop || !filteredItems.length || (!panelOpen && !isFullscreen)) return;
    const item = filteredItems[idx];
    if (!item) return;
    if (untrack(() => panelUrl) === item.url) return;
    panelHintTitle = item.title || item.url;
    panelUrl = item.url;
    if (isFullscreen) {
      withSkipUrl(skipUrl, () => replacePreviewHistory(item.url, item.title || item.url));
    }
  });
</script>

<svelte:window onkeydown={handleKeydown} onpopstate={handlePopState} />

<svelte:head>
  <title>Hister - History</title>
  <link rel="alternate" type="application/rss+xml" title="Hister History" href={rssUrl} />
</svelte:head>

<header class="border-brutal-border bg-card-surface shrink-0 border-b-[3px] px-3 py-3 md:px-6">
  <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
    <div class="flex min-w-0 items-center gap-4">
      <PageHeader color="hister-indigo" size="sm" class="min-w-0 shrink-0" truncate>
        History
      </PageHeader>
      <a
        href={rssUrl}
        title="RSS feed"
        aria-label="RSS feed"
        class="text-(--text-secondary) transition-colors hover:text-[#f26522]"
      >
        <Rss size={20} />
      </a>
    </div>

    <nav class="grid min-w-0 gap-2 md:grid-cols-[auto_minmax(16rem,28rem)] md:items-center">
      <label
        class="font-inter text-text-brand-secondary flex shrink-0 cursor-pointer items-center gap-1.5 text-xs font-semibold select-none"
      >
        <input
          type="checkbox"
          bind:checked={openedOnly}
          class="accent-hister-indigo size-3.5 cursor-pointer"
        />
        <span>Show only opened results</span>
      </label>

      <div
        class="border-brutal-border bg-page-bg flex h-11 min-w-0 items-center gap-2 border-[3px] px-3 shadow-[2px_2px_0_var(--brutal-shadow)]"
      >
        <Search class="text-text-brand-muted size-4 shrink-0" />
        <Input
          bind:value={filter}
          placeholder="Filter title or URL"
          aria-label="Filter history"
          class="font-inter text-text-brand placeholder:text-text-brand-muted h-full min-w-0 flex-1 border-0 bg-transparent p-0 text-sm font-medium shadow-none focus-visible:ring-0"
        />
        {#if filter}
          <Button
            variant="ghost"
            size="icon-sm"
            class="text-text-brand-muted hover:bg-muted-surface hover:text-text-brand size-7 shrink-0"
            aria-label="Clear history filter"
            onclick={clearFilter}
          >
            <X class="size-4" />
          </Button>
        {/if}
      </div>
    </nav>
  </div>
</header>

{#if loading && items.length === 0}
  <StatusMessage message="Loading history..." type="loading" />
{:else if error}
  <StatusMessage message={error} type="error" class="mx-3 mt-4 md:mx-6" />
{:else if filteredItems.length === 0}
  <StatusMessage message={filter ? 'No matching entries' : 'No history yet'} type="empty" />
{:else}
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden md:flex-row">
    <!-- Timeline sidebar: hidden on mobile, shown on md+ -->
    {#if !previewFullscreen}
      <ScrollArea
        class="border-brutal-border bg-page-bg hidden w-72 shrink-0 border-r-[3px] md:block"
      >
        <div class="space-y-2 p-4">
          <span
            class="font-space text-text-brand-muted flex items-center gap-2 px-1 text-xs font-bold uppercase"
          >
            <CalendarDays class="size-3.5" />
            Timeline
          </span>
          <Separator class="bg-border-brand-muted h-[2px]" />

          <Button
            variant="ghost"
            class="flex h-auto w-full cursor-pointer items-center justify-start gap-2 rounded-none border-[2px] px-3 py-2 shadow-[2px_2px_0_transparent] hover:shadow-[2px_2px_0_var(--brutal-shadow)] {!filterByDate
              ? 'border-brutal-border bg-hister-indigo text-primary-foreground hover:bg-hister-indigo/90 hover:text-primary-foreground'
              : 'border-transparent hover:border-border-brand hover:bg-muted-surface'}"
            onclick={showAll}
          >
            <span
              class="font-inter text-sm font-semibold"
              class:text-text-brand-secondary={!!filterByDate}
            >
              Show All
            </span>
            <Badge
              variant="secondary"
              class="ml-auto h-4 shrink-0 border-0 px-1.5 py-0 text-xs {filterByDate
                ? 'bg-muted-surface text-text-brand-muted'
                : 'bg-white/20 text-primary-foreground'}"
            >
              {allCount}
            </Badge>
          </Button>

          <Separator class="bg-border-brand-muted h-[2px]" />

          {#each allGroups as group, i}
            {@const color = getGroupColor(i)}
            {@const isActive = filterByDate === group.key}
            <Button
              variant="ghost"
              class="flex h-auto w-full cursor-pointer items-center justify-start gap-2 rounded-none border-[2px] px-3 py-2 shadow-[2px_2px_0_transparent] hover:shadow-[2px_2px_0_var(--brutal-shadow)] {isActive
                ? 'border-brutal-border text-primary-foreground hover:text-primary-foreground'
                : 'border-transparent hover:border-border-brand hover:bg-muted-surface'}"
              style={isActive ? `background-color: ${getColorVar(color)};` : ''}
              onclick={() => scrollToGroup(group.key)}
            >
              <span
                class="h-2 w-2 shrink-0 rounded-full"
                style={isActive
                  ? 'background-color: white;'
                  : `background-color: ${getColorVar(color)};`}
              ></span>
              <span
                class="font-inter truncate text-sm"
                class:font-semibold={isActive}
                class:font-medium={!isActive}
                class:text-text-brand-secondary={!isActive}
              >
                {group.label}
              </span>
              <Badge
                variant="secondary"
                class="ml-auto h-4 shrink-0 border-0 px-1.5 py-0 text-xs {isActive
                  ? 'bg-white/20 text-primary-foreground'
                  : 'bg-muted-surface text-text-brand-muted'}"
              >
                {groupCount(group)}
              </Badge>
            </Button>
          {/each}
        </div>
      </ScrollArea>
    {/if}

    <!-- Mobile timeline: horizontal scrollable filter chips -->
    {#if !previewFullscreen}
      <div
        class="border-brutal-border bg-page-bg flex shrink-0 items-center gap-2 overflow-x-auto border-b-[3px] px-3 py-2 md:hidden"
      >
        <span class="text-text-brand-muted flex shrink-0 items-center">
          <ListFilter class="size-4" />
        </span>
        <Button
          variant="ghost"
          size="sm"
          class="font-inter border-brutal-border h-8 shrink-0 rounded-none border-[2px] px-3 text-xs font-bold {!filterByDate
            ? 'bg-hister-indigo hover:bg-hister-indigo/90 text-primary-foreground hover:text-primary-foreground'
            : 'text-text-brand-secondary hover:bg-muted-surface'}"
          onclick={showAll}
        >
          All ({allCount})
        </Button>
        {#each allGroups as group, i}
          {@const color = getGroupColor(i)}
          {@const isActive = filterByDate === group.key}
          <Button
            variant="ghost"
            size="sm"
            class="font-inter border-brutal-border h-8 shrink-0 rounded-none border-[2px] px-3 text-xs font-semibold {isActive
              ? 'text-primary-foreground hover:text-primary-foreground'
              : 'text-text-brand-secondary hover:bg-muted-surface'}"
            style={isActive ? `background-color: ${getColorVar(color)};` : ''}
            onclick={() => scrollToGroup(group.key)}
          >
            {group.label} ({groupCount(group)})
          </Button>
        {/each}
      </div>
    {/if}

    <div class="flex min-h-0 flex-1 overflow-hidden" bind:this={splitContainerEl}>
      {#if !previewFullscreen}
        <ScrollArea
          orientation="vertical"
          class="min-h-0 max-w-full min-w-0 flex-1 overflow-x-hidden"
        >
          <div
            class="mx-auto w-full max-w-5xl space-y-5 overflow-hidden px-3 py-3 md:space-y-7 md:px-6 md:py-5"
          >
            {#each groups as group, gi}
              {@const color = getGlobalGroupColor(group.key)}
              {@const groupOffset = groups
                .slice(0, gi)
                .reduce((acc: number, g) => acc + g.items.length, 0)}
              <section id="group-{encodeURIComponent(group.key)}" class="history-group">
                <div class="history-group-header" style="--history-color: {getColorVar(color)};">
                  <div class="min-w-0">
                    <h2
                      class="font-outfit text-text-brand truncate text-base font-black md:text-lg"
                    >
                      {group.label}
                    </h2>
                    <p class="font-fira text-text-brand-muted text-xs">
                      {groupCount(group).toLocaleString()} entries
                    </p>
                  </div>
                  <span class="history-group-count">{groupCount(group).toLocaleString()}</span>
                </div>

                <div class="history-stack">
                  {#each group.items as item, ii}
                    {@const itemColor = color}
                    {@const flatIdx = groupOffset + ii}
                    <article
                      data-result
                      class="history-row flex items-start gap-3 px-3 py-3 transition-all duration-150 md:items-center md:px-4"
                      class:history-row-active={flatIdx === highlightIdx}
                      style="--history-color: {getColorVar(itemColor)};"
                    >
                      <div class="w-0 min-w-0 flex-1 space-y-1">
                        <a
                          data-result-link={item.url}
                          href={item.url}
                          class="history-title font-outfit text-hister-cyan block text-base font-bold no-underline hover:underline md:text-lg"
                          target="_blank"
                          rel="noopener"
                          onclick={() => (highlightIdx = flatIdx)}
                        >
                          {(item.title || item.url).replace(/<[^>]*>/g, '')}
                        </a>
                        <div
                          class="flex min-w-0 flex-col gap-1 md:flex-row md:items-center md:gap-2"
                        >
                          {#if item.added}
                            <span
                              class="font-inter bg-muted-surface text-text-brand-secondary w-fit px-1.5 py-0.5 text-xs font-semibold whitespace-nowrap"
                              title={formatTimestamp(item.added)}
                              >{formatRelativeTime(item.added)}</span
                            >
                          {/if}
                          <span
                            class="history-url font-fira text-text-brand-muted block text-xs md:text-sm"
                            title={item.url}>{item.url}</span
                          >
                        </div>
                      </div>
                      <nav class="flex shrink-0 items-center gap-1">
                        {#if !disablePreviews}
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            class="text-text-brand-muted hover:bg-muted-surface hover:text-hister-teal size-8 shrink-0"
                            aria-label="Preview entry"
                            title="Preview"
                            onclick={() => {
                              highlightIdx = flatIdx;
                              openPreview(item.url, item.title || item.url);
                            }}
                          >
                            <Eye class="size-3.5" />
                          </Button>
                        {/if}
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted hover:bg-muted-surface hover:text-hister-rose size-8 shrink-0"
                          aria-label="Delete entry"
                          title="Delete"
                          onclick={() => deleteItem(item)}
                        >
                          <Trash2 class="size-3.5" />
                        </Button>
                      </nav>
                    </article>
                  {/each}
                </div>
              </section>
            {/each}
          </div>
          {#if loading && items.length > 0}
            <div class="flex justify-center py-4">
              <span class="font-inter text-text-brand-muted animate-pulse text-xs">Loading...</span>
            </div>
          {/if}
          <div bind:this={sentinel} class="h-px w-full"></div>
        </ScrollArea>
      {/if}

      <!-- Preview panel: fullscreen (both mobile and desktop) or split-pane (desktop only) -->
      {#if !disablePreviews}
        {#if previewFullscreen}
          <PreviewPanel
            url={panelUrl}
            hintTitle={panelHintTitle}
            fullscreen={true}
            onclose={closePanelAndFullscreen}
            onfullscreentoggle={isDesktop ? exitFullscreen : undefined}
          />
        {:else if panelOpen && isDesktop}
          <!-- Drag handle to resize the split-screen panel -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <div
            class="hover:bg-hister-indigo/40 w-1.5 shrink-0 cursor-col-resize bg-transparent transition-colors"
            onmousedown={startPanelResize}
            role="separator"
            aria-label="Resize preview panel"
          ></div>
          <div
            style="width: min({panelWidthPct}%, max(0px, calc(100% - 28rem))); flex: none;"
            class="flex min-h-0 overflow-hidden"
          >
            <PreviewPanel
              url={panelUrl}
              hintTitle={panelHintTitle}
              fullscreen={false}
              onclose={() => {
                panelOpen = false;
                localStorage.setItem('hister-history-panel-open', 'false');
              }}
              onfullscreentoggle={enterFullscreen}
            />
          </div>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  .history-group {
    min-width: 0;
  }

  .history-group-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.5rem 0 0.65rem;
    border-bottom: 3px solid var(--history-color);
  }

  .history-group-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 2rem;
    height: 1.55rem;
    padding: 0 0.45rem;
    flex-shrink: 0;
    color: var(--text-primary-brand);
    font-family: var(--font-fira);
    font-size: 0.72rem;
    font-weight: 700;
    background: color-mix(in srgb, var(--history-color) 18%, var(--muted-surface));
    border: 2px solid color-mix(in srgb, var(--history-color) 55%, var(--border-brand));
  }

  .history-stack {
    padding-top: 0.75rem;
  }

  .history-row {
    border: 2px solid var(--border-muted-brand);
    background-color: var(--card-surface);
    box-shadow: 0 1px 0 color-mix(in srgb, white 6%, transparent) inset;
  }

  .history-title,
  .history-url {
    overflow-wrap: anywhere;
    word-break: break-word;
  }

  .history-row + .history-row {
    border-top: 0;
  }

  .history-row-active {
    border-color: color-mix(in srgb, var(--history-color) 60%, var(--border-brand));
    background:
      linear-gradient(
        90deg,
        color-mix(in srgb, var(--history-color) 12%, transparent),
        transparent 42%
      ),
      var(--card-surface);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--history-color) 32%, transparent) inset;
  }

  :global(.dark) .history-row {
    box-shadow: 0 1px 0 color-mix(in srgb, white 8%, transparent) inset;
  }
</style>
