<script lang="ts">
  import { page } from '$app/stores';
  import { Button } from '@hister/components/ui/button';
  import { toggleMode, mode } from 'mode-watcher';
  import { Sun, Moon, Keyboard } from '@lucide/svelte';
  import { showHelp } from '$lib/stores';

  const links = [
    { label: 'Help', href: 'help' },
    { label: 'Extractors', href: 'extractors' },
    { label: 'About', href: 'about' },
    { label: 'API', href: 'api-docs' },
    { label: 'GitHub', href: 'https://github.com/asciimoo/hister/', external: true },
  ];

  const iconBtn =
    'text-text-brand-muted hover:text-hister-indigo size-8 shrink-0 transition-all hover:scale-110';
  const linkCls =
    'font-space text-text-brand-secondary hover:text-hister-indigo text-[11px] tracking-[1px] uppercase no-underline hover:underline md:text-[13px]';
</script>

<footer
  class="bg-brutal-bg border-brutal-border grid h-12 shrink-0 grid-cols-[1fr_auto_1fr] items-center border-t-[3px] px-6 text-sm"
>
  <span></span>

  <nav class="flex items-center gap-4 md:gap-6" aria-label="Secondary">
    {#each links as link (link.href)}
      <a
        href={link.href}
        class={linkCls}
        target={link.external ? '_blank' : undefined}
        rel={link.external ? 'noopener' : undefined}>{link.label}</a
      >
    {/each}
  </nav>

  <div class="flex items-center justify-end gap-1">
    <Button variant="ghost" size="icon" class={iconBtn} title="Toggle theme" onclick={toggleMode}>
      {#if mode.current === 'dark'}<Sun class="size-5" />{:else}<Moon class="size-5" />{/if}
    </Button>
    {#if $page.url.pathname === '/'}
      <Button
        variant="ghost"
        size="icon"
        class={iconBtn}
        title="Keyboard shortcuts (?)"
        aria-label="Show keyboard shortcuts"
        onclick={() => ($showHelp = !$showHelp)}
      >
        <Keyboard class="size-5" />
      </Button>
    {/if}
  </div>
</footer>
