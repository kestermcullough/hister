<script lang="ts">
  import { apiFetch } from '$lib/api';
  import { Button } from '@hister/components/ui/button';
  import * as Card from '@hister/components/ui/card';
  import { PageHeader } from '@hister/components';
  import { StatusMessage } from '$lib/components';
  import { Eye, EyeOff, RefreshCw, User, Info } from '@lucide/svelte';

  let username = $state('');
  let token = $state('');
  let tokenVisible = $state(false);
  let message = $state('');
  let messageType = $state<'success' | 'error'>('success');
  let generating = $state(false);
  let isAdmin = $state(false);
  let version = $state('');

  $effect(() => {
    apiFetch('/profile')
      .then((r) => r.json())
      .then((data) => {
        username = data.username;
        isAdmin = data.is_admin ?? false;
        version = data.version ?? '';
      })
      .catch(() => {
        message = 'Failed to load profile';
        messageType = 'error';
      });
  });

  async function generateToken() {
    generating = true;
    message = '';
    try {
      const r = await apiFetch('/profile/token', { method: 'POST' });
      if (!r.ok) {
        message = 'Failed to generate token';
        messageType = 'error';
        return;
      }
      const data = await r.json();
      token = data.token;
      tokenVisible = true;
      message = 'New token generated. Store it securely — it will not be shown again.';
      messageType = 'success';
    } catch {
      message = 'Failed to generate token';
      messageType = 'error';
    } finally {
      generating = false;
    }
  }
</script>

<svelte:head>
  <title>Profile - Hister</title>
</svelte:head>

<div class="flex-1 overflow-y-auto px-4 py-6 md:px-12 md:py-10">
  <PageHeader color="hister-indigo" class="mx-auto mb-8 max-w-2xl">Profile</PageHeader>

  <div class="mx-auto max-w-2xl space-y-6">
    <!-- User info card -->
    <Card.Root
      class="bg-card-surface border-hister-indigo gap-0 overflow-hidden rounded-none border-[3px] py-0 shadow-[6px_6px_0_var(--hister-indigo)]"
    >
      <Card.Header class="bg-hister-indigo px-7 py-5">
        <Card.Title
          class="font-outfit flex items-center gap-2 text-xl font-black tracking-wide text-white"
        >
          <User size={20} />
          Account
        </Card.Title>
      </Card.Header>
      <Card.Content class="px-7 py-6">
        <div class="flex items-center gap-3">
          <span
            class="font-outfit text-text-brand-muted text-sm font-bold tracking-widest uppercase"
            >Username</span
          >
          <span class="font-fira text-text-brand text-sm">{username}</span>
        </div>
      </Card.Content>
    </Card.Root>

    <!-- Access token card -->
    <Card.Root
      class="bg-card-surface border-brutal-border gap-0 overflow-hidden rounded-none border-[3px] py-0 shadow-[6px_6px_0_var(--brutal-shadow)]"
    >
      <Card.Header class="border-brutal-border border-b-[3px] px-7 py-5">
        <Card.Title class="font-outfit text-text-brand text-xl font-black tracking-wide"
          >Access Token</Card.Title
        >
        <Card.Description class="font-inter text-text-brand-muted text-sm">
          Use this token to authenticate CLI and API access. Generating a new token will invalidate
          the previous one.
        </Card.Description>
      </Card.Header>
      <Card.Content class="space-y-4 px-7 py-6">
        {#if message}
          <StatusMessage {message} type={messageType} />
        {/if}

        {#if token}
          <div class="border-brutal-border flex items-center gap-2 border-[3px] px-4 py-3">
            {#if tokenVisible}
              <code class="font-fira text-text-brand flex-1 text-sm break-all">{token}</code>
            {:else}
              <code class="font-fira text-text-brand-muted flex-1 text-sm">{'•'.repeat(40)}</code>
            {/if}
            <button
              onclick={() => (tokenVisible = !tokenVisible)}
              class="text-text-brand-muted hover:text-hister-indigo shrink-0 cursor-pointer border-0 bg-transparent p-1 transition-colors"
              aria-label={tokenVisible ? 'Hide token' : 'Show token'}
            >
              {#if tokenVisible}
                <EyeOff size={16} />
              {:else}
                <Eye size={16} />
              {/if}
            </button>
          </div>
        {/if}

        <Button
          onclick={generateToken}
          disabled={generating}
          variant="outline"
          class="border-brutal-border font-outfit hover:border-hister-indigo h-11 w-full border-[3px] text-sm font-bold tracking-wide transition-all hover:shadow-[4px_4px_0_var(--brutal-shadow)] disabled:opacity-50"
        >
          <RefreshCw size={16} class="mr-2 {generating ? 'animate-spin' : ''}" />
          {token ? 'Regenerate Token' : 'Generate Token'}
        </Button>
      </Card.Content>
    </Card.Root>

    <!-- Instance info card — admin only -->
    {#if isAdmin && version}
      <Card.Root
        class="bg-card-surface border-brutal-border gap-0 overflow-hidden rounded-none border-[3px] py-0 shadow-[6px_6px_0_var(--brutal-shadow)]"
      >
        <Card.Header class="border-brutal-border border-b-[3px] px-7 py-5">
          <Card.Title
            class="font-outfit text-text-brand flex items-center gap-2 text-xl font-black tracking-wide"
          >
            <Info size={20} />
            Instance Info
          </Card.Title>
        </Card.Header>
        <Card.Content class="px-7 py-6">
          <div class="flex items-center gap-3">
            <span
              class="font-outfit text-text-brand-muted w-32 shrink-0 text-sm font-bold tracking-widest uppercase"
              >Version</span
            >
            <span class="font-fira text-text-brand text-sm">{version}</span>
          </div>
        </Card.Content>
      </Card.Root>
    {/if}
  </div>
</div>
