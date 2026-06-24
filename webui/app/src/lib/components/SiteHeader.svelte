<script lang="ts">
  import { page } from '$app/stores';
  import { base } from '$app/paths';
  import { Button } from '@hister/components/ui/button';
  import { LogIn, LogOut, UserRound } from '@lucide/svelte';
  import type { AppConfig } from '$lib/api';

  let { config, onLogout }: { config: AppConfig | null; onLogout: () => void } = $props();

  const navItems = [
    { label: 'History', href: 'history', color: 'var(--hister-indigo)' },
    { label: 'Rules', href: 'rules', color: 'var(--hister-teal)' },
    { label: 'Add', href: 'add', color: 'var(--hister-coral)' },
  ];

  const iconBtn =
    'text-text-brand-muted hover:text-hister-indigo hover:bg-muted-surface size-8 shrink-0 transition-all md:size-9';
  const navLink =
    'primary-link font-space relative p-3 text-[11px] font-semibold tracking-[1px] uppercase no-underline transition-colors hover:no-underline md:p-6 md:text-[13px] md:tracking-[1.5px]';

  const showWriteNav = $derived(!config?.public || !!config?.canWrite);
  const showLogin = $derived(
    !!config &&
      !config.authenticated &&
      (config.authMode === 'user' || config.authMode === 'token'),
  );
  const showLogout = $derived(
    !!config && config.authenticated && (config.authMode === 'user' || config.authMode === 'token'),
  );
</script>

<header
  class="site-header bg-brutal-bg border-brutal-border sticky top-0 z-50 flex h-12 shrink-0 items-center justify-between gap-2 overflow-hidden border-b-[3px] px-3 md:grid md:h-16 md:grid-cols-[12rem_auto_12rem] md:justify-stretch md:gap-4 md:px-6"
>
  <h1 class="flex shrink-0 items-center gap-1.5 md:gap-2">
    <a
      data-sveltekit-reload
      href="./"
      class="group flex items-center gap-1.5 no-underline md:gap-2"
    >
      <img src="static/logo.png" alt="Hister logo" class="h-6 w-6 md:h-8 md:w-8" />
      <span
        class="font-space text-text-brand text-lg font-extrabold tracking-[1px] uppercase group-hover:underline md:text-[28px] md:tracking-[2px]"
      >
        Hister
      </span>
    </a>
  </h1>

  <nav class="flex items-center justify-self-center" aria-label="Primary">
    {#if showWriteNav}
      {#each navItems as item (item.href)}
        {@const active = $page.url.pathname === new URL(item.href, $page.url).pathname}
        <a
          class="{navLink} {active
            ? 'is-active text-text-brand font-bold'
            : 'text-text-brand-muted hover:bg-muted-surface hover:text-text-brand'}"
          style="--nav-color: {item.color};"
          aria-current={active ? 'page' : undefined}
          href={item.href}>{item.label}</a
        >
      {/each}
    {/if}
  </nav>

  <div class="flex items-center justify-self-end">
    {#if config?.authMode === 'user'}
      {#if config?.authenticated && config?.username}
        <Button
          variant="ghost"
          size="icon"
          class={iconBtn}
          title="Profile"
          onclick={() => (window.location.href = base + '/profile')}
        >
          <UserRound class="size-5" />
        </Button>
      {:else if showLogin}
        <Button
          variant="ghost"
          size="icon"
          class={iconBtn}
          title="Login"
          onclick={() => (window.location.href = base + '/auth')}
        >
          <LogIn class="size-5" />
        </Button>
      {/if}
    {:else if config?.authMode === 'token' && showLogin}
      <Button
        variant="ghost"
        size="icon"
        class={iconBtn}
        title="Login"
        onclick={() => (window.location.href = base + '/auth')}
      >
        <LogIn class="size-5" />
      </Button>
    {/if}
    {#if showLogout}
      <Button
        variant="ghost"
        size="icon"
        class={iconBtn}
        title={config?.username ? `Logout ${config.username}` : 'Logout'}
        onclick={onLogout}
      >
        <LogOut class="size-5" />
      </Button>
    {/if}
  </div>
</header>

<style>
  .site-header {
    box-shadow: 0 1px 0 color-mix(in srgb, white 6%, transparent) inset;
  }

  .primary-link:hover {
    color: color-mix(in srgb, var(--nav-color) 76%, var(--text-primary-brand));
  }

  .primary-link.is-active {
    background: color-mix(in srgb, var(--nav-color) 9%, transparent);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--nav-color) 10%, transparent) inset;
  }

  :global(.dark) .site-header {
    box-shadow: 0 1px 0 color-mix(in srgb, white 8%, transparent) inset;
  }

  :global(.dark) .primary-link:hover {
    color: color-mix(in srgb, var(--nav-color) 84%, white);
  }

  :global(.dark) .primary-link.is-active {
    background: color-mix(in srgb, var(--nav-color) 13%, transparent);
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--nav-color) 16%, transparent) inset;
  }
</style>
