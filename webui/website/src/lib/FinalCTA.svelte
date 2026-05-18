<script lang="ts">
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import { Button } from '@hister/components';
</script>

<section
  class="border-hister-indigo flex w-full flex-col items-center gap-10 border-t-[3px] bg-[var(--text-primary)] px-6 py-16 md:py-24"
>
  <h2
    class="font-space max-w-[800px] text-center text-4xl leading-[0.95] font-black tracking-[-1px] text-white uppercase md:text-6xl lg:text-[64px]"
  >
    Preserve Knowledge
  </h2>

  <nav class="flex flex-wrap items-center justify-center gap-4">
    <Button
      href="/docs/quickstart"
      class="bg-hister-coral hover:bg-hister-coral/90 font-space border-brutal-border h-auto rounded-none border-[3px] px-8 py-4 text-lg font-bold tracking-[1.5px] text-white uppercase no-underline shadow-[5px_5px_0_var(--hister-coral)] transition-all hover:translate-x-[3px] hover:translate-y-[3px] hover:shadow-[2px_2px_0_var(--hister-coral)] md:px-12"
    >
      Get started
      <ArrowRight size={22} />
    </Button>
    <Button
      href="/docs"
      class="font-space h-auto rounded-none border-2 border-white/25 bg-white/10 px-8 py-4 text-lg font-bold tracking-[1.5px] text-white uppercase no-underline shadow-[5px_5px_0_rgba(255,255,255,0.12)] transition-all hover:translate-x-[3px] hover:translate-y-[3px] hover:bg-white/15 hover:shadow-[2px_2px_0_rgba(255,255,255,0.12)] md:px-12"
    >
      Documentation
    </Button>
  </nav>
</section>
