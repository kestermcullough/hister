<script lang="ts">
  import { page } from '$app/stores';
  import { Button } from '@hister/components/ui/button';
  import { LogIn, LogOut, UserRound } from '@lucide/svelte';
  import type { AppConfig } from '$lib/api';

  let { config, onLogout }: { config: AppConfig | null; onLogout: () => void } = $props();

  const navItems = [
    { label: 'History', href: 'history' },
    { label: 'Rules', href: 'rules' },
    { label: 'Add', href: 'add' },
  ];

  const iconBtn =
    'text-text-brand-muted hover:text-hister-indigo size-8 shrink-0 transition-all hover:scale-110 md:size-10';
  const navLink =
    'font-space p-3 text-[11px] font-semibold tracking-[1px] uppercase no-underline hover:underline md:p-6 md:text-[13px] md:tracking-[1.5px]';
</script>

<header
  class="bg-brutal-bg border-brutal-border sticky top-0 z-50 flex h-12 shrink-0 items-center justify-between gap-2 overflow-hidden border-b-[3px] px-3 md:grid md:h-16 md:grid-cols-[4rem_auto_4rem] md:justify-stretch md:gap-4 md:px-6"
>
  <h1 class="flex shrink-0 items-center gap-1.5 md:gap-2">
    <img src="static/logo.png" alt="Hister logo" class="h-6 w-6 md:h-8 md:w-8" />
    <a
      data-sveltekit-reload
      href="./"
      class="font-space text-text-brand text-lg font-extrabold tracking-[1px] uppercase no-underline hover:underline md:text-[28px] md:tracking-[2px]"
    >
      Hister
    </a>
  </h1>

  <nav class="flex items-center justify-self-center">
    {#each navItems as item (item.href)}
      {@const active = $page.url.pathname === new URL(item.href, $page.url).pathname}
      <a
        class="{navLink} {active
          ? 'text-text-brand font-bold'
          : 'text-text-brand-secondary hover:text-text-brand'}"
        href={item.href}>{item.label}</a
      >
    {/each}
  </nav>

  <div class="flex items-center justify-self-end">
    {#if config?.authMode === 'user'}
      {#if config?.username}
        <Button
          variant="ghost"
          size="icon"
          class={iconBtn}
          title="Profile"
          onclick={() => (window.location.href = '/profile')}
        >
          <UserRound class="size-5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          class={iconBtn}
          title="Logout {config.username}"
          onclick={onLogout}
        >
          <LogOut class="size-5" />
        </Button>
      {:else}
        <Button
          variant="ghost"
          size="icon"
          class={iconBtn}
          title="Login"
          onclick={() => (window.location.href = '/auth')}
        >
          <LogIn class="size-5" />
        </Button>
      {/if}
    {/if}
  </div>
</header>
