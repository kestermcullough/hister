<script lang="ts">
  import { Button } from '@hister/components/ui/button';
  import { Input } from '@hister/components/ui/input';
  import { Label } from '@hister/components/ui/label';
  import * as Card from '@hister/components/ui/card';
  import SettingsInput from './SettingsInput.svelte';
  import { Plus, Trash2, Sun, Moon } from '@lucide/svelte';
  import { ModeWatcher, toggleMode, mode } from 'mode-watcher';

  const defaultURL = 'http://127.0.0.1:4433/';

  let url = $state(defaultURL);
  let customHeaders: { name: string; value: string }[] = $state([]);
  let message = $state('');
  let messageType: 'success' | 'error' = $state('success');

  chrome.storage.local.get(['histerURL', 'histerCustomHeaders'], (data) => {
    if (!data['histerURL']) {
      chrome.storage.local.set({ histerURL: defaultURL });
    }
    url = data['histerURL'] || defaultURL;
    customHeaders = Array.isArray(data['histerCustomHeaders']) ? data['histerCustomHeaders'] : [];
  });

  function addHeader() {
    customHeaders.push({ name: '', value: '' });
  }

  function removeHeader(index: number) {
    customHeaders.splice(index, 1);
  }

  function save(e: Event) {
    e.preventDefault();
    const headersToSave = customHeaders.filter((h) => h.name.trim() !== '');
    chrome.storage.local
      .set({
        histerURL: url,
        histerCustomHeaders: $state.snapshot(headersToSave),
      })
      .then(() => {
        customHeaders = headersToSave;
        message = 'Settings saved';
        messageType = 'success';
      });
  }

  function authenticate() {
    let authURL = url;
    if (!authURL.endsWith('/')) {
      authURL += '/';
    }
    chrome.cookies.getAll({ url: authURL }, (cookies) => {
      if (!cookies.length) {
        message =
          'No cookies found for server URL. Make sure you are logged in to the Hister web app.';
        messageType = 'error';
        return;
      }
      const cookieStr = cookies.map((c) => `${c.name}=${c.value}`).join('; ');
      chrome.storage.local.set({ histerCookies: cookieStr }).then(() => {
        message = 'Authentication successful';
        messageType = 'success';
      });
    });
  }
</script>

<ModeWatcher />

<div class="bg-page-bg min-h-screen">
  <!-- Page header -->
  <div
    class="bg-brutal-bg border-brutal-border flex items-center justify-between border-b-[3px] px-8 py-5"
  >
    <span class="font-outfit text-text-brand-muted text-sm font-bold tracking-widest uppercase">
      Hister <span class="mx-1">/</span> Options
    </span>
    <button
      onclick={toggleMode}
      class="text-text-brand-muted hover:text-hister-coral cursor-pointer border-0 bg-transparent p-0 transition-colors"
      aria-label="Toggle theme"
    >
      {#if mode.current === 'light'}
        <Moon size={18} />
      {:else}
        <Sun size={18} />
      {/if}
    </button>
  </div>

  <div class="mx-auto max-w-2xl space-y-8 px-8 py-10">
    <!-- Connection settings card -->
    <Card.Root
      class="bg-card-surface border-hister-indigo gap-0 overflow-hidden rounded-none border-[3px] py-0 shadow-[6px_6px_0_var(--hister-indigo)]"
    >
      <Card.Header class="bg-hister-indigo px-7 py-5">
        <Card.Title class="font-outfit text-xl font-black tracking-wide text-white"
          >Connection Settings</Card.Title
        >
        <Card.Description class="font-inter text-sm text-white/70"
          >Configure how the extension connects to your Hister server.</Card.Description
        >
      </Card.Header>

      <Card.Content class="space-y-6 p-7">
        {#if message}
          <div
            class="font-inter border-l-[4px] px-4 py-3 text-sm {messageType === 'success'
              ? 'border-l-hister-teal bg-hister-teal/10 text-hister-teal'
              : 'border-l-hister-rose bg-hister-rose/10 text-hister-rose'}"
          >
            {message}
          </div>
        {/if}

        <form onsubmit={save} class="space-y-6">
          <SettingsInput
            label="Server URL"
            bind:value={url}
            placeholder="Server URL..."
            description="The full URL of your Hister server, including the port number."
          />

          <!-- Custom Headers -->
          <div class="space-y-3">
            <div>
              <Label class="font-outfit text-text-brand text-sm font-bold">Custom Headers</Label>
              <p class="text-text-brand-muted font-inter mt-1 text-xs">
                Add custom HTTP headers sent with every request. Useful for reverse proxy
                authentication.
              </p>
            </div>
            {#each customHeaders as header, i}
              <div class="flex items-center gap-2">
                <Input
                  bind:value={header.name}
                  placeholder="Header name"
                  class="bg-page-bg border-hister-indigo font-fira text-text-brand placeholder:text-text-brand-muted focus-visible:border-hister-coral h-10 border-[3px] px-3 text-sm shadow-none transition-colors focus-visible:ring-0"
                />
                <Input
                  bind:value={header.value}
                  placeholder="Header value"
                  class="bg-page-bg border-hister-indigo font-fira text-text-brand placeholder:text-text-brand-muted focus-visible:border-hister-coral h-10 border-[3px] px-3 text-sm shadow-none transition-colors focus-visible:ring-0"
                />
                <button
                  type="button"
                  onclick={() => removeHeader(i)}
                  class="text-text-brand-muted hover:text-hister-rose h-10 shrink-0 cursor-pointer border-0 bg-transparent p-2 transition-colors"
                  aria-label="Remove header"
                >
                  <Trash2 size={16} />
                </button>
              </div>
            {/each}
            <Button
              type="button"
              variant="outline"
              onclick={addHeader}
              class="border-brutal-border font-outfit hover:border-hister-indigo h-9 border-[3px] text-sm font-bold tracking-wide transition-all"
            >
              <Plus size={16} class="mr-1" />
              Add Header
            </Button>
          </div>

          <Button
            type="submit"
            size="lg"
            class="bg-hister-coral border-brutal-border font-outfit h-12 w-full border-[3px] text-base font-bold tracking-wide text-white shadow-[4px_4px_0_var(--brutal-shadow)] transition-all hover:translate-x-px hover:translate-y-px hover:shadow-[2px_2px_0_var(--brutal-shadow)]"
          >
            Save Settings
          </Button>

          <Button
            type="button"
            variant="outline"
            size="lg"
            onclick={authenticate}
            class="border-brutal-border font-outfit hover:border-hister-indigo h-12 w-full border-[3px] text-base font-bold tracking-wide transition-all"
          >
            Authenticate Extension
          </Button>
        </form>
      </Card.Content>
    </Card.Root>

    <!-- Indexing rules placeholder -->
    <Card.Root
      class="bg-card-surface border-brutal-border gap-0 overflow-hidden rounded-none border-[3px] py-0 opacity-50"
    >
      <Card.Header class="px-7 py-5">
        <Card.Title class="font-outfit text-text-brand text-lg font-bold">Indexing Rules</Card.Title
        >
        <Card.Description class="font-inter text-text-brand-muted text-sm"
          >Coming soon — configure which pages to index or skip.</Card.Description
        >
      </Card.Header>
    </Card.Root>
  </div>
</div>
