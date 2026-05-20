<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import VideoPreview from './VideoPreview.svelte';
  import { apiFetch } from '$lib/api';
  import { formatTimestamp, formatMetaDate } from '$lib/search';
  import { ScrollArea } from '@hister/components/ui/scroll-area';
  import { Button } from '@hister/components/ui/button';
  import { Eye, X, Maximize2, Minimize2, History } from '@lucide/svelte';

  interface Props {
    url: string;
    hintTitle?: string;
    onclose: () => void;
    fullscreen?: boolean;
    onfullscreentoggle?: () => void;
  }

  interface DocumentVersion {
    id: number;
    created_at: string;
    html_diff: string;
    text_diff: string;
  }

  let { url, hintTitle = '', onclose, fullscreen = false, onfullscreentoggle }: Props = $props();

  let title = $state('');
  let content = $state('');
  let template = $state('');
  let templateData = $state<any>(null);
  let meta = $state<Record<string, any> | null>(null);
  let added = $state<number | null>(null);
  let loading = $state(false);
  let versionCount = $state(0);
  let versions = $state<DocumentVersion[]>([]);
  let showVersions = $state(false);

  function parseTemplateData(c: string): any | null {
    try {
      return JSON.parse(c);
    } catch {
      return null;
    }
  }

  type DiffLine = { type: 'header' | 'add' | 'remove' | 'context'; text: string };

  function parseDiff(patch: string): DiffLine[] {
    return patch
      .split('\n')
      .filter((l) => l !== '')
      .map((l): DiffLine => {
        if (l.startsWith('@@')) return { type: 'header', text: l };
        if (l.startsWith('+')) return { type: 'add', text: l };
        if (l.startsWith('-')) return { type: 'remove', text: l };
        return { type: 'context', text: l };
      });
  }

  function versionTimestamp(iso: string): number {
    return new Date(iso).getTime() / 1000;
  }

  $effect(() => {
    if (url) {
      loadContent(url, hintTitle);
    }
  });

  async function loadContent(u: string, hint: string) {
    loading = true;
    content = '';
    template = '';
    templateData = null;
    meta = null;
    added = null;
    title = hint;
    showVersions = false;
    versions = [];
    versionCount = 0;
    try {
      const resp = await apiFetch(`/preview?url=${encodeURIComponent(u)}`);
      if (!resp.ok) {
        content = `<p class="text-hister-rose">Failed to load readable content. Status: ${resp.status}</p>`;
      } else {
        const data = await resp.json();
        title = data.title || hint;
        added = data.added ?? null;
        meta = data.meta ?? null;
        versionCount = data.version_count ?? 0;
        template = data.template || '';
        templateData = template === 'video' ? parseTemplateData(data.content) : null;
        content = template === 'video' ? '' : data.content || '<p>No content available</p>';
      }
    } catch (err) {
      content = `<p class="text-hister-rose">Failed to load: ${err}</p>`;
    } finally {
      loading = false;
    }
  }

  async function toggleVersions(u: string) {
    if (showVersions) {
      showVersions = false;
      return;
    }
    if (versions.length === 0) {
      try {
        const resp = await apiFetch(`/versions?url=${encodeURIComponent(u)}`);
        if (resp.ok) {
          versions = (await resp.json()) ?? [];
        }
      } catch {
        // silently ignore
      }
    }
    showVersions = true;
  }
</script>

<div
  class="border-border-brand bg-card-surface flex flex-1 flex-col overflow-hidden {fullscreen
    ? ''
    : 'shrink-0 border-l-[3px]'}"
>
  {#if loading}
    <div
      class="border-border-brand-muted flex shrink-0 items-center justify-end gap-1 border-b-[2px] px-2 py-1"
    >
      {#if onfullscreentoggle}
        <Button
          variant="ghost"
          size="icon-sm"
          class="text-text-brand-muted hover:text-text-brand"
          onclick={onfullscreentoggle}
          title={fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
        >
          {#if fullscreen}
            <Minimize2 class="size-4" />
          {:else}
            <Maximize2 class="size-4" />
          {/if}
        </Button>
      {/if}
      <Button
        variant="ghost"
        size="icon-sm"
        class="text-text-brand-muted hover:text-text-brand"
        onclick={onclose}
      >
        <X class="size-4" />
      </Button>
    </div>
    <div class="flex flex-1 items-center justify-center">
      <span class="font-inter text-text-brand-muted text-sm">Loading…</span>
    </div>
  {:else if content || templateData}
    <div
      class="border-border-brand-muted flex shrink-0 items-start gap-2 border-b-[2px] px-4 py-2.5"
    >
      <div class="flex flex-1 flex-col gap-0.5">
        <h2
          class="font-outfit text-text-brand line-clamp-2 text-lg leading-snug font-bold md:text-3xl"
        >
          <a href={url} target="_blank" rel="noopener noreferrer" class="hover:underline">{title}</a
          >
        </h2>
        {#if meta?.author || meta?.published || meta?.type}
          <span class="font-inter text-text-brand-muted text-xs">
            {#if meta?.author}<span>{meta.author}</span>{/if}
            {#if meta?.author && meta?.published}<span class="mx-1">·</span>{/if}
            {#if meta?.published}<span>{formatMetaDate(meta.published)}</span>{/if}
            {#if (meta?.author || meta?.published) && meta?.type}<span class="mx-1">·</span>{/if}
            {#if meta?.type}<span class="uppercase">{meta.type}</span>{/if}
          </span>
        {/if}
        {#if added}
          <span
            class="font-inter inline-flex flex-wrap items-center gap-1.5 text-xs"
            title={formatTimestamp(added)}
          >
            <span>indexed {formatTimestamp(added)}</span>
            {#if versionCount > 0}
              <span class="text-text-brand-muted">·</span>
              <button
                onclick={() => toggleVersions(url)}
                class="font-inter text-hister-teal inline-flex cursor-pointer items-center gap-1 text-xs hover:underline"
              >
                <History class="size-3" />
                {versionCount}
                {versionCount === 1 ? 'previous version' : 'previous versions'}
              </button>
            {/if}
          </span>
        {/if}
        {#if meta?.description}
          <p class="font-inter text-text-brand-secondary mt-1 line-clamp-3 text-sm">
            {meta.description}
          </p>
        {/if}
      </div>
      <div class="mt-1 flex shrink-0 items-center gap-1">
        {#if onfullscreentoggle}
          <Button
            variant="ghost"
            size="icon-sm"
            class="hover:text-text-brand"
            onclick={onfullscreentoggle}
            title={fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
          >
            {#if fullscreen}
              <Minimize2 class="size-4" />
            {:else}
              <Maximize2 class="size-4" />
            {/if}
          </Button>
        {/if}
        <Button variant="ghost" size="icon-sm" class="hover:text-text-brand" onclick={onclose}>
          <X class="size-4" />
        </Button>
      </div>
    </div>
    <ScrollArea class="min-h-0 flex-1">
      {#if showVersions}
        <div class="flex flex-col divide-y divide-[var(--border-brand-muted)] p-4">
          {#each versions as v}
            <div class="py-4 first:pt-0 last:pb-0">
              <p class="font-inter mb-2 text-xs font-semibold tracking-wide uppercase">
                {formatTimestamp(versionTimestamp(v.created_at))}
              </p>
              {#if v.text_diff || v.html_diff}
                <details class="group">
                  <summary
                    class="font-inter text-text-brand-muted hover:text-text-brand flex cursor-pointer list-none items-center gap-1 text-xs select-none"
                  >
                    <span class="inline-block transition-transform group-open:rotate-90">▶</span>
                    <span>show diff</span>
                  </summary>
                  <div class="mt-2 overflow-x-auto rounded font-mono text-xs leading-relaxed">
                    {#each parseDiff(v.text_diff || v.html_diff) as line}
                      <div
                        class="px-2 py-px break-all whitespace-pre-wrap {line.type === 'add'
                          ? 'bg-black text-green-300'
                          : line.type === 'remove'
                            ? 'bg-black text-red-300'
                            : line.type === 'header'
                              ? 'text-text-brand'
                              : 'text-text-brand-secondary'}"
                      >
                        {line.text}
                      </div>
                    {/each}
                  </div>
                </details>
              {:else}
                <p class="font-inter text-xs italic">No diff recorded.</p>
              {/if}
            </div>
          {/each}
        </div>
      {:else}
        <div
          class="font-inter text-text-brand-secondary prose dark:prose-invert prose-a:text-hister-teal w-full max-w-[60em] p-4 text-sm"
        >
          {#if template === 'video' && templateData}
            <VideoPreview data={templateData} />
          {:else}
            {@html content}
          {/if}
          {#if meta?.jsonld}
            <details class="not-prose border-border-brand-muted mt-6 border-t pt-3">
              <summary
                class="font-inter text-text-brand-muted cursor-pointer text-xs tracking-wide uppercase"
              >
                Extracted JSON-LD ({meta.jsonld.length})
              </summary>
              <pre
                class="bg-card-surface-muted text-text-brand-secondary mt-2 overflow-x-auto rounded p-2 text-[11px] leading-snug">{JSON.stringify(
                  meta.jsonld,
                  null,
                  2,
                )}</pre>
            </details>
          {/if}
        </div>
      {/if}
    </ScrollArea>
  {:else}
    <div
      class="border-border-brand-muted flex shrink-0 items-center justify-end gap-1 border-b-[2px] px-2 py-1"
    >
      {#if onfullscreentoggle}
        <Button
          variant="ghost"
          size="icon-sm"
          class="text-text-brand-muted hover:text-text-brand"
          onclick={onfullscreentoggle}
          title={fullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
        >
          {#if fullscreen}
            <Minimize2 class="size-4" />
          {:else}
            <Maximize2 class="size-4" />
          {/if}
        </Button>
      {/if}
      <Button
        variant="ghost"
        size="icon-sm"
        class="text-text-brand-muted hover:text-text-brand"
        onclick={onclose}
      >
        <X class="size-4" />
      </Button>
    </div>
    <div class="flex flex-1 flex-col items-center justify-center gap-2 opacity-40">
      <Eye class="size-6" />
      <p class="font-inter text-text-brand-muted text-sm">Focus a result to read it</p>
    </div>
  {/if}
</div>
