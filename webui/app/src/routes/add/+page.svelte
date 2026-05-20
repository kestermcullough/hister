<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchConfig, apiFetch } from '$lib/api';
  import { Input } from '@hister/components/ui/input';
  import { Textarea } from '@hister/components/ui/textarea';
  import { Label } from '@hister/components/ui/label';
  import { Button } from '@hister/components/ui/button';
  import * as Card from '@hister/components/ui/card';
  import * as Alert from '@hister/components/ui/alert';
  import AlertCircle from '@lucide/svelte/icons/circle-alert';
  import CheckCircle from '@lucide/svelte/icons/circle-check';
  import { Save } from '@lucide/svelte';

  let url = $state('');
  let title = $state('');
  let text = $state('');
  let message = $state('');
  let isError = $state(false);
  let submitting = $state(false);

  onMount(async () => {
    await fetchConfig();
  });

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (submitting) return;
    submitting = true;
    message = '';
    try {
      const res = await apiFetch('/add', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url, title, text }),
      });
      if (res.status === 201) {
        message = 'Document added successfully.';
        isError = false;
        url = '';
        title = '';
        text = '';
      } else if (res.status === 406) {
        message = 'URL skipped (matches skip rules or is a local URL).';
        isError = false;
      } else {
        message = 'Failed to add document.';
        isError = true;
      }
    } catch (err) {
      message = String(err);
      isError = true;
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>Hister - Add</title>
</svelte:head>

<div class="flex flex-1 items-start justify-center overflow-y-auto px-6 pt-12">
  <Card.Root color="hister-indigo" class="w-full max-w-160">
    <Card.Header color="hister-indigo" class="justify-between gap-2 px-7 py-5">
      <Card.Title class="font-outfit text-[22px] font-black text-white">Add Entry</Card.Title>
      <Card.Description class="font-inter text-[13px] font-medium text-white/70"
        >Manually add a document to the index</Card.Description
      >
    </Card.Header>

    <Card.Content class="space-y-6">
      {#if message}
        <Alert.Root variant={isError ? 'error' : 'success'}>
          {#if isError}
            <AlertCircle class="size-4 shrink-0" />
          {:else}
            <CheckCircle class="size-4 shrink-0" />
          {/if}
          <Alert.Description class="font-inter text-sm">{message}</Alert.Description>
        </Alert.Root>
      {/if}

      <form onsubmit={handleSubmit} class="space-y-6">
        <div class="space-y-2">
          <Label class="font-outfit text-text-brand text-sm font-bold">URL</Label>
          <Input
            type="text"
            variant="brutal"
            bind:value={url}
            placeholder="https://..."
            required
            class="border-hister-indigo focus-visible:border-hister-coral"
          />
        </div>

        <div class="space-y-2">
          <Label class="font-outfit text-text-brand text-sm font-bold">Title</Label>
          <Input
            type="text"
            variant="brutal"
            bind:value={title}
            placeholder="Page title..."
            class="border-hister-indigo font-inter focus-visible:border-hister-coral"
          />
        </div>

        <div class="space-y-2">
          <Label class="font-outfit text-text-brand text-sm font-bold">Content</Label>
          <Textarea
            bind:value={text}
            placeholder="Paste or type page content..."
            class="bg-page-bg border-hister-indigo font-inter text-text-brand placeholder:text-text-brand-muted focus-visible:border-hister-coral min-h-45 w-full resize-y rounded-none border-[3px] p-4 text-sm transition-colors outline-none focus-visible:ring-0"
          />
        </div>

        <Button
          type="submit"
          disabled={submitting}
          size="lg"
          class="bg-hister-coral font-outfit hover:bg-hister-coral/90 h-13 w-full text-base font-extrabold tracking-[1px] text-white shadow-[4px_4px_0px_var(--hister-coral)]"
        >
          <Save class="size-5 shrink-0" />
          <span>{submitting ? 'Saving...' : 'Save Entry'}</span>
        </Button>
      </form>
    </Card.Content>
  </Card.Root>
</div>
