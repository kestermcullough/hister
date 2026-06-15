<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { Button } from '@hister/components/ui/button';
  import { ArrowLeft } from '@lucide/svelte';
  import PreviewPanel from '$lib/components/PreviewPanel.svelte';
  import { base } from '$app/paths';

  let docUrl = $state('');
  let docTitle = $state('');

  function readParams() {
    const params = new URLSearchParams(window.location.search);
    docUrl = params.get('id') || '';
    docTitle = params.get('title') || '';
  }

  onMount(() => {
    readParams();
  });
</script>

<svelte:window onpopstate={readParams} />

<svelte:head>
  <title>{docTitle ? `${docTitle} - Hister Preview` : 'Hister Preview'}</title>
</svelte:head>

<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
  {#if docUrl}
    <PreviewPanel
      url={docUrl}
      hintTitle={docTitle}
      fullscreen={true}
      onclose={() => {
        try {
          const ref = document.referrer;
          if (ref && new URL(ref).origin === window.location.origin) {
            window.history.back();
            return;
          }
        } catch {
          // ignore referrer parse errors
        }
        window.location.href = base + '/';
      }}
    />
  {:else}
    <div class="flex flex-1 flex-col items-center justify-center gap-4">
      <p class="font-inter text-text-brand-secondary">No document URL specified.</p>
      <Button
        variant="outline"
        href={base + '/'}
        class="font-inter gap-2 rounded-none border-[2px]"
      >
        <ArrowLeft class="size-4" />
        Back to search
      </Button>
    </div>
  {/if}
</div>
