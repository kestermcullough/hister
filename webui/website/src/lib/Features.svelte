<script lang="ts">
  import { PageHeader } from '@hister/components';

  const accentColors = [
    '--hister-lime',
    '--hister-coral',
    '--hister-amber',
    '--hister-rose',
    '--hister-cyan',
    '--hister-teal',
    '--hister-indigo',
  ];

  const features = [
    {
      title: 'Automatic save of visited websites',
      desc: 'The browser extension silently captures and indexes every page you visit. Your entire browsing history becomes searchable without any manual effort.',
    },
    {
      title: 'Full-text indexing',
      desc: 'Index the actual content of web pages and local files, not just titles and URLs. Find any document by the words it contains.',
    },
    {
      title: 'Semantic search',
      desc: 'Find content by meaning, not just keywords. Powered by optional AI embeddings for context-aware, intent-driven results.',
    },
    {
      title: 'Advanced querying',
      desc: 'Precision search with field filters, boolean operators, phrase matching, wildcards, and personal query aliases for faster recall.',
    },
    {
      title: 'Result preview',
      desc: 'Open a clean, reader-friendly offline preview of any indexed page directly inside Hister, no internet connection required.',
    },
    {
      title: 'Crawler with multiple backends',
      desc: 'Index entire websites with the built-in BFS crawler. Supports a standard HTTP backend and real browser backends for JavaScript-heavy pages.',
    },
    {
      title: 'MCP server',
      desc: 'Expose your personal search index as an MCP (Model Context Protocol) tool so AI assistants can query your browsing history directly.',
    },
    {
      title: 'Local file indexing',
      desc: 'Index and search documents on your filesystem alongside web pages. Directories are watched for changes and the index updates automatically.',
    },
    {
      title: 'Browser history import',
      desc: 'Bring your existing Firefox or Chrome browsing history into Hister.',
    },
    {
      title: 'History view',
      desc: 'Browse and revisit your past search queries and indexed documents.',
    },
    {
      title: 'User handling',
      desc: 'Create multiple accounts with separate indexes, individual API tokens, and per-user content rules. Ideal for shared or team deployments.',
    },
    {
      title: 'Privacy first',
      desc: 'Fully self-hosted with no telemetry, no cloud sync, and no third-party services. Your data never leaves your own infrastructure.',
    },
    {
      title: 'Content rules',
      desc: 'Define skip rules to block unwanted sites, priority rules to surface important domains, versioning rules to preserve document changes, and query aliases to create custom search shortcuts.',
    },
    {
      title: 'Browser extensions',
      desc: 'Extensions available for Chrome and Firefox. Install once and indexing happens automatically in the background as you browse.',
    },
    {
      title: 'Terminal UI',
      desc: 'A built-in interactive TUI lets you search directly from the terminal with full keyboard navigation, themes, and configurable hotkeys.',
    },
    {
      title: 'Multi-language support',
      desc: 'Automatic language detection routes documents to dedicated per-language indexes, improving relevance and ranking across multilingual content.',
    },
  ];
</script>

<section
  id="features"
  class="bg-brutal-bg mx-auto flex max-w-[2000px] flex-col gap-10 px-6 py-12 md:px-12 md:py-16"
>
  <div class="mx-auto flex w-full max-w-[70em] flex-col gap-10">
    <PageHeader color="hister-coral" size="lg" tag="h2">Features</PageHeader>

    <div class="bg-brutal-card shadow-brutal-lg border-2 border-[var(--brutal-border)]">
      {#each features as feature, i}
        <div
          style="border-left: 16px solid var({accentColors[i % accentColors.length]})"
          class="bg-brutal-card grid grid-cols-[1fr_auto] items-center
            {i < features.length - 1 ? 'border-b-2 border-[var(--brutal-border)]' : ''}"
        >
          <div class="px-6 py-5">
            <p class="text-base font-bold text-[var(--text-primary)] md:text-lg">{feature.title}</p>
            <p class="mt-1 text-sm text-[var(--text-secondary)]">{feature.desc}</p>
          </div>
          <div class="relative flex shrink-0 items-center justify-center px-6">
            <div
              class="absolute h-5 w-5 animate-ping rounded-full bg-green-400"
              style="animation-delay: {i * 0.22}s; animation-duration: 2.2s; opacity: 0.55"
            ></div>
            <div class="h-5 w-5 rounded-full bg-green-500"></div>
          </div>
        </div>
      {/each}
    </div>
  </div>
</section>
