<script lang="ts">
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import Server from '@lucide/svelte/icons/server';
  import Lock from '@lucide/svelte/icons/lock';
  import Code2 from '@lucide/svelte/icons/code-2';
  import { Button } from '@hister/components';
</script>

<section class="bg-brutal-bg flex w-full flex-col items-center gap-8 px-6 py-12 md:py-24">
  <h1
    class="font-outfit max-w-[1000px] text-center text-5xl leading-[0.95] font-black tracking-[-2px] text-[var(--text-primary)] uppercase md:text-7xl lg:text-[100px]"
  >
    Your Own <br /> Search Engine
  </h1>

  <p
    class="font-inter text-center text-base leading-relaxed text-[var(--text-secondary)] md:text-2xl"
  >
    Full-text search across your files, visited sites and beyond.
  </p>

  <nav class="my-4 flex flex-wrap items-center justify-center gap-8 md:my-16">
    <Button
      href="https://demo.hister.org/"
      class="bg-hister-indigo hover:bg-hister-indigo/90 font-space border-brutal-border brutal-press-lg h-auto rounded-none border-[3px] px-10 py-5 text-lg font-bold tracking-[1.5px] text-white uppercase no-underline md:px-14"
    >
      Try the demo
      <ArrowRight size={20} />
    </Button>
    <Button
      href="https://github.com/asciimoo/hister"
      target="_blank"
      rel="noopener noreferrer"
      class="bg-brutal-card hover:bg-brutal-card font-space border-brutal-border brutal-press-lg h-auto rounded-none border-[3px] px-10 py-5 text-lg font-bold tracking-[1.5px] text-[var(--text-primary)] uppercase no-underline md:px-14"
    >
      View on github
      <ArrowRight size={20} />
    </Button>
  </nav>
  <ul class="my-0 flex list-none flex-wrap items-center justify-center gap-4 p-0 md:gap-8">
    <li class="flex items-center gap-2 text-[var(--text-secondary)]">
      <Code2 size={20} class="shrink-0" />
      <span class="font-space text-xl font-bold tracking-[1px] uppercase">Free Software</span>
    </li>
    <li class="hidden text-[var(--text-secondary)]/40 md:block" aria-hidden="true">|</li>
    <li class="flex items-center gap-2 text-[var(--text-secondary)]">
      <Server size={20} class="shrink-0" />
      <span class="font-space text-xl font-bold tracking-[1px] uppercase">Self Hosted</span>
    </li>
    <li class="hidden text-[var(--text-secondary)]/40 md:block" aria-hidden="true">|</li>
    <li class="flex items-center gap-2 text-[var(--text-secondary)]">
      <Lock size={20} class="shrink-0" />
      <span class="font-space text-xl font-bold tracking-[1px] uppercase">Privacy First</span>
    </li>
  </ul>
</section>
