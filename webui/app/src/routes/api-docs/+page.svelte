<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Badge } from '@hister/components/ui/badge';
  import * as Card from '@hister/components/ui/card';
  import * as Table from '@hister/components/ui/table';
  import { PageHeader } from '@hister/components';

  interface EndpointArg {
    name: string;
    type: string;
    required: boolean;
    description: string;
  }

  interface JSONSchemaField {
    name: string;
    type: string;
    required: boolean;
    description: string;
    fields?: JSONSchemaField[];
  }

  interface APIEndpoint {
    name: string;
    path: string;
    method: string;
    csrf_required: boolean;
    public: boolean;
    requires_auth: boolean;
    mutates: boolean;
    description: string;
    args: EndpointArg[] | null;
    json_schema: JSONSchemaField[] | null;
  }

  let endpoints: APIEndpoint[] = $state([]);
  let loading = $state(true);

  onMount(async () => {
    try {
      const res = await apiFetch('', {
        headers: { Accept: 'application/json' },
      });
      if (res.ok) endpoints = await res.json();
    } finally {
      loading = false;
    }
  });

  function tocId(ep: APIEndpoint): string {
    return `ep-${ep.method.toLowerCase()}-${ep.path.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '')}`;
  }
</script>

<svelte:head>
  <title>Hister - API</title>
</svelte:head>

<div class="flex-1 overflow-y-auto px-3 py-4 md:px-6 md:py-5">
  <div class="mx-auto max-w-[80em]">
    <div class="mb-4 space-y-1 md:mb-5">
      <PageHeader color="hister-teal" size="sm">API Documentation</PageHeader>
      <p class="font-inter text-text-brand-secondary text-xs md:text-sm">
        Available HTTP endpoints for integrating with Hister
      </p>
    </div>

    {#if loading}
      <p class="font-inter text-text-brand-muted py-8 text-center text-sm">Loading endpoints...</p>
    {:else}
      <!-- Mobile ToC: horizontal scrollable pill list -->
      <div class="mb-3 md:hidden">
        <div class="flex gap-1.5 overflow-x-auto pb-1">
          {#each endpoints as ep}
            <a
              href="#{tocId(ep)}"
              class="font-inter text-text-brand-secondary bg-card-surface flex shrink-0 items-center gap-1 border border-black px-2 py-0.5 text-[11px] whitespace-nowrap"
            >
              <span
                class="font-bold {ep.method === 'GET' ? 'text-hister-teal' : 'text-hister-coral'}"
                >{ep.method}</span
              >
              {ep.name}
            </a>
          {/each}
        </div>
      </div>

      <div class="flex items-start gap-6">
        <!-- ToC sidebar: desktop only -->
        <nav class="sticky top-4 hidden w-56 shrink-0 self-start md:block">
          <div
            class="bg-card-surface border border-black p-3 shadow-[3px_3px_0_var(--brutal-shadow)]"
          >
            <p class="font-outfit text-text-brand mb-2 text-xs font-bold tracking-wider uppercase">
              Contents
            </p>
            <ul class="space-y-1">
              {#each endpoints as ep}
                <li>
                  <a
                    href="#{tocId(ep)}"
                    class="font-inter text-text-brand-secondary hover:text-text-brand flex items-center gap-1.5 py-0.5 text-xs leading-snug transition-colors"
                  >
                    <span
                      class="shrink-0 font-bold {ep.method === 'GET'
                        ? 'text-hister-teal'
                        : 'text-hister-coral'}">{ep.method}</span
                    >
                    <span class="truncate">{ep.name}</span>
                  </a>
                </li>
              {/each}
            </ul>
          </div>
        </nav>

        <!-- Endpoint cards -->
        <div class="min-w-0 flex-1 space-y-4">
          {#each endpoints as ep}
            <Card.Root
              id={tocId(ep)}
              class="bg-card-surface gap-0 overflow-hidden rounded-none border border-black py-0 shadow-[4px_4px_0_var(--brutal-shadow)]"
            >
              <Card.Header class="gap-0 px-4 py-3">
                <div class="flex flex-wrap items-center gap-2.5">
                  <Card.Title class="font-outfit text-text-brand text-base font-bold md:text-xl"
                    >{ep.name}</Card.Title
                  >
                  <Badge
                    variant="default"
                    class="border-0 px-2 py-0 text-[11px] leading-5 font-bold {ep.method === 'GET'
                      ? 'bg-hister-teal text-white'
                      : 'bg-hister-coral text-white'}"
                  >
                    {ep.method}
                  </Badge>
                  <code class="font-fira text-text-brand-secondary text-sm">{ep.path}</code>
                  {#if ep.csrf_required}
                    <Badge
                      variant="outline"
                      class="border-hister-amber text-hister-amber border-2 px-1.5 py-0 text-[10px] leading-5 font-semibold"
                    >
                      CSRF
                    </Badge>
                  {/if}
                  {#if ep.public}
                    <Badge
                      variant="outline"
                      class="border-hister-teal text-hister-teal border-2 px-1.5 py-0 text-[10px] leading-5 font-semibold"
                    >
                      Public
                    </Badge>
                  {/if}
                  {#if ep.requires_auth}
                    <Badge
                      variant="outline"
                      class="border-hister-rose text-hister-rose border-2 px-1.5 py-0 text-[10px] leading-5 font-semibold"
                    >
                      Auth
                    </Badge>
                  {/if}
                  {#if ep.mutates}
                    <Badge
                      variant="outline"
                      class="border-hister-coral text-hister-coral border-2 px-1.5 py-0 text-[10px] leading-5 font-semibold"
                    >
                      Write
                    </Badge>
                  {/if}
                </div>
              </Card.Header>

              <Card.Content class="border-t px-4 py-3">
                <p class="font-inter text-text-brand-secondary mb-3 text-sm">{ep.description}</p>

                {#if ep.args && ep.args.length > 0}
                  <h4
                    class="font-outfit text-text-brand-muted mb-2 text-xs font-bold tracking-wider uppercase"
                  >
                    Arguments
                  </h4>
                  <div class="hidden md:block">
                    <Table.Root>
                      <Table.Header>
                        <Table.Row
                          class="bg-muted-surface border-brutal-border hover:bg-muted-surface border-b"
                        >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Name</Table.Head
                          >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Type</Table.Head
                          >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Required</Table.Head
                          >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Description</Table.Head
                          >
                        </Table.Row>
                      </Table.Header>
                      <Table.Body>
                        {#each ep.args as arg}
                          <Table.Row class="border-brutal-border/30 border-b">
                            <Table.Cell
                              class="font-fira text-text-brand px-3 py-2 text-sm font-semibold"
                              ><code>{arg.name}</code></Table.Cell
                            >
                            <Table.Cell
                              class="font-fira text-text-brand-secondary px-3 py-2 text-sm"
                              ><code>{arg.type}</code></Table.Cell
                            >
                            <Table.Cell class="px-3 py-2">
                              {#if arg.required}
                                <Badge
                                  variant="default"
                                  class="bg-hister-rose border-0 px-1.5 py-0 text-[10px] text-white"
                                  >required</Badge
                                >
                              {:else}
                                <span class="font-inter text-text-brand-muted text-xs"
                                  >optional</span
                                >
                              {/if}
                            </Table.Cell>
                            <Table.Cell
                              class="font-inter text-text-brand-secondary px-3 py-2 text-sm whitespace-normal"
                              >{arg.description}</Table.Cell
                            >
                          </Table.Row>
                        {/each}
                      </Table.Body>
                    </Table.Root>
                  </div>
                  <div class="space-y-2.5 md:hidden">
                    {#each ep.args as arg}
                      <div class="space-y-0.5">
                        <div class="flex items-center gap-2">
                          <code class="font-fira text-text-brand text-sm font-semibold"
                            >{arg.name}</code
                          >
                          <code class="font-fira text-text-brand-muted text-xs">{arg.type}</code>
                          {#if arg.required}
                            <Badge
                              variant="default"
                              class="bg-hister-rose border-0 px-1.5 py-0 text-[10px] text-white"
                              >required</Badge
                            >
                          {/if}
                        </div>
                        <p class="font-inter text-text-brand-secondary text-xs">
                          {arg.description}
                        </p>
                      </div>
                    {/each}
                  </div>
                {/if}

                {#if ep.json_schema && ep.json_schema.length > 0}
                  {#snippet schemaTableRows(fields: JSONSchemaField[], depth: number)}
                    {#each fields as field}
                      <Table.Row class="border-brutal-border/30 border-b">
                        <Table.Cell
                          class="font-fira text-text-brand px-3 py-2 text-sm font-semibold"
                        >
                          {#if depth > 0}
                            <span class="text-text-brand-muted select-none"
                              >{'  '.repeat(depth)}↳&nbsp;</span
                            >
                          {/if}<code>{field.name}</code>
                        </Table.Cell>
                        <Table.Cell class="font-fira text-text-brand-secondary px-3 py-2 text-sm"
                          ><code>{field.type}</code></Table.Cell
                        >
                        <Table.Cell class="px-3 py-2">
                          {#if field.required}
                            <Badge
                              variant="default"
                              class="bg-hister-rose border-0 px-1.5 py-0 text-[10px] text-white"
                              >required</Badge
                            >
                          {:else}
                            <span class="font-inter text-text-brand-muted text-xs">optional</span>
                          {/if}
                        </Table.Cell>
                        <Table.Cell
                          class="font-inter text-text-brand-secondary px-3 py-2 text-sm whitespace-normal"
                          >{field.description}</Table.Cell
                        >
                      </Table.Row>
                      {#if field.fields && field.fields.length > 0}
                        {@render schemaTableRows(field.fields, depth + 1)}
                      {/if}
                    {/each}
                  {/snippet}

                  {#snippet schemaCards(fields: JSONSchemaField[], depth: number)}
                    {#each fields as field}
                      <div class="space-y-0.5" style="margin-left: {depth * 14}px">
                        <div class="flex items-center gap-2">
                          {#if depth > 0}
                            <span class="text-text-brand-muted text-xs">↳</span>
                          {/if}
                          <code class="font-fira text-text-brand text-sm font-semibold"
                            >{field.name}</code
                          >
                          <code class="font-fira text-text-brand-muted text-xs">{field.type}</code>
                          {#if field.required}
                            <Badge
                              variant="default"
                              class="bg-hister-rose border-0 px-1.5 py-0 text-[10px] text-white"
                              >required</Badge
                            >
                          {/if}
                        </div>
                        <p class="font-inter text-text-brand-secondary text-xs">
                          {field.description}
                        </p>
                      </div>
                      {#if field.fields && field.fields.length > 0}
                        {@render schemaCards(field.fields, depth + 1)}
                      {/if}
                    {/each}
                  {/snippet}

                  <h4
                    class="font-outfit text-text-brand-muted mt-3 mb-2 text-xs font-bold tracking-wider uppercase"
                  >
                    Request Body (JSON)
                  </h4>
                  <div class="hidden md:block">
                    <Table.Root>
                      <Table.Header>
                        <Table.Row
                          class="bg-muted-surface border-brutal-border hover:bg-muted-surface border-b"
                        >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Field</Table.Head
                          >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Type</Table.Head
                          >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Required</Table.Head
                          >
                          <Table.Head
                            class="font-inter text-text-brand-muted h-auto px-3 py-2 text-xs font-bold"
                            >Description</Table.Head
                          >
                        </Table.Row>
                      </Table.Header>
                      <Table.Body>
                        {@render schemaTableRows(ep.json_schema, 0)}
                      </Table.Body>
                    </Table.Root>
                  </div>
                  <div class="space-y-2.5 md:hidden">
                    {@render schemaCards(ep.json_schema, 0)}
                  </div>
                {/if}
              </Card.Content>
            </Card.Root>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</div>
