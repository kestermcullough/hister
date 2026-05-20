<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchConfig, apiFetch } from '$lib/api';
  import { Button } from '@hister/components/ui/button';
  import { Input } from '@hister/components/ui/input';
  import { Badge } from '@hister/components/ui/badge';
  import * as Card from '@hister/components/ui/card';
  import * as Table from '@hister/components/ui/table';
  import { Shield, Link2, Plus, Trash2, Pencil, Check, X } from '@lucide/svelte';
  import { PageHeader } from '@hister/components';
  import * as Alert from '@hister/components/ui/alert';
  import AlertCircle from '@lucide/svelte/icons/circle-alert';
  import CheckCircle from '@lucide/svelte/icons/circle-check';

  interface RulesData {
    skip: string[];
    priority: string[];
    versioning: string[];
    aliases: Record<string, string>;
  }

  interface RuleRow {
    pattern: string;
    type: 'skip' | 'priority' | 'versioning';
  }

  let rules: RulesData = $state({ skip: [], priority: [], versioning: [], aliases: {} });
  let loading = $state(true);
  let saving = $state(false);
  let message = $state('');
  let isError = $state(false);
  let newAliasKeyword = $state('');
  let newAliasValue = $state('');
  let newRulePattern = $state('');
  let newRuleType: 'skip' | 'priority' | 'versioning' = $state('skip');

  // Editing state for aliases
  let editingAliasKey = $state<string | null>(null);
  let editAliasKeyword = $state('');
  let editAliasValue = $state('');

  // Editing state for rules
  let editingRuleIndex = $state<number | null>(null);
  let editRulePattern = $state('');
  let editRuleType: 'skip' | 'priority' | 'versioning' = $state('skip');

  const ruleRows = $derived.by(() => {
    const rows: RuleRow[] = [];
    for (const p of rules.skip) rows.push({ pattern: p, type: 'skip' });
    for (const p of rules.priority) rows.push({ pattern: p, type: 'priority' });
    for (const p of rules.versioning) rows.push({ pattern: p, type: 'versioning' });
    return rows;
  });

  onMount(async () => {
    await fetchConfig();
    await loadRules();
  });

  async function loadRules() {
    loading = true;
    try {
      const res = await apiFetch('/rules', { headers: { Accept: 'application/json' } });
      if (!res.ok) throw new Error('Failed to load rules');
      const data = await res.json();
      rules = {
        skip: data.skip ?? [],
        priority: data.priority ?? [],
        versioning: data.versioning ?? [],
        aliases: data.aliases ?? {},
      };
    } catch (e) {
      message = String(e);
      isError = true;
    } finally {
      loading = false;
    }
  }

  async function saveRules() {
    if (saving) return;
    saving = true;
    message = '';
    try {
      const formData = new URLSearchParams();
      formData.set('skip', rules.skip.join('\n'));
      formData.set('priority', rules.priority.join('\n'));
      formData.set('versioning', rules.versioning.join('\n'));
      const res = await apiFetch('/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: formData.toString(),
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body.trim() || 'Failed to save rules');
      }
      message = 'Rules saved successfully';
      isError = false;
      await loadRules();
    } catch (e) {
      message = String(e);
      isError = true;
    } finally {
      saving = false;
    }
  }

  function removeRule(pattern: string, type: 'skip' | 'priority' | 'versioning') {
    if (type === 'skip') {
      rules.skip = rules.skip.filter((p) => p !== pattern);
    } else if (type === 'priority') {
      rules.priority = rules.priority.filter((p) => p !== pattern);
    } else {
      rules.versioning = rules.versioning.filter((p) => p !== pattern);
    }
    saveRules();
  }

  function addRule() {
    if (!newRulePattern.trim()) return;
    const pattern = newRulePattern.trim();
    if (
      rules.skip.includes(pattern) ||
      rules.priority.includes(pattern) ||
      rules.versioning.includes(pattern)
    ) {
      message = `Rule "${pattern}" already exists.`;
      isError = true;
      return;
    }
    if (newRuleType === 'skip') {
      rules.skip = [...rules.skip, pattern];
    } else if (newRuleType === 'priority') {
      rules.priority = [...rules.priority, pattern];
    } else {
      rules.versioning = [...rules.versioning, pattern];
    }
    newRulePattern = '';
    saveRules();
  }

  async function deleteAlias(keyword: string) {
    const formData = new URLSearchParams({ alias: keyword });
    const res = await apiFetch('/delete_alias', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formData.toString(),
    });
    if (res.ok) await loadRules();
  }

  async function addAlias(e: Event) {
    e.preventDefault();
    if (!newAliasKeyword || !newAliasValue) return;
    const keyword = newAliasKeyword.trim();
    if (Object.prototype.hasOwnProperty.call(rules.aliases, keyword)) {
      message = `Alias "${keyword}" already exists.`;
      isError = true;
      return;
    }
    const formData = new URLSearchParams({
      'alias-keyword': keyword,
      'alias-value': newAliasValue,
    });
    const res = await apiFetch('/add_alias', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: formData.toString(),
    });
    if (res.ok) {
      newAliasKeyword = '';
      newAliasValue = '';
      await loadRules();
    }
  }

  function startEditAlias(keyword: string, value: string) {
    editingAliasKey = keyword;
    editAliasKeyword = keyword;
    editAliasValue = value;
  }

  function cancelEditAlias() {
    editingAliasKey = null;
  }

  async function saveEditAlias() {
    const trimmedKeyword = editAliasKeyword.trim();
    const trimmedValue = editAliasValue.trim();
    if (!trimmedKeyword || !trimmedValue) return;
    const oldKey = editingAliasKey!;

    // If the keyword is being renamed, check the new keyword doesn't already exist
    if (
      trimmedKeyword !== oldKey &&
      Object.prototype.hasOwnProperty.call(rules.aliases, trimmedKeyword)
    ) {
      message = `Alias "${trimmedKeyword}" already exists.`;
      isError = true;
      return;
    }

    // Add/overwrite with new keyword+value
    const addForm = new URLSearchParams({
      'alias-keyword': trimmedKeyword,
      'alias-value': trimmedValue,
    });
    const addRes = await apiFetch('/add_alias', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: addForm.toString(),
    });
    if (!addRes.ok) return;

    // If the keyword was renamed, delete the old key
    if (trimmedKeyword !== oldKey) {
      await apiFetch('/delete_alias', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams({ alias: oldKey }).toString(),
      });
    }

    editingAliasKey = null;
    await loadRules();
  }

  function startEditRule(index: number) {
    const row = ruleRows[index];
    editingRuleIndex = index;
    editRulePattern = row.pattern;
    editRuleType = row.type;
  }

  function cancelEditRule() {
    editingRuleIndex = null;
  }

  function saveEditRule() {
    const trimmed = editRulePattern.trim();
    if (!trimmed) return;
    const row = ruleRows[editingRuleIndex!];
    // Reject if the new pattern already exists elsewhere (different item)
    const isDuplicate =
      (rules.skip.includes(trimmed) ||
        rules.priority.includes(trimmed) ||
        rules.versioning.includes(trimmed)) &&
      trimmed !== row.pattern;
    if (isDuplicate) {
      message = `Rule "${trimmed}" already exists.`;
      isError = true;
      editingRuleIndex = null;
      return;
    }
    // Update in the appropriate array
    if (row.type === 'skip') {
      rules.skip = rules.skip.map((p) => (p === row.pattern ? trimmed : p));
    } else if (row.type === 'priority') {
      rules.priority = rules.priority.map((p) => (p === row.pattern ? trimmed : p));
    } else {
      rules.versioning = rules.versioning.map((p) => (p === row.pattern ? trimmed : p));
    }
    // If type changed, move between arrays
    if (editRuleType !== row.type) {
      // Remove from old array
      if (row.type === 'skip') {
        rules.skip = rules.skip.filter((p) => p !== trimmed);
      } else if (row.type === 'priority') {
        rules.priority = rules.priority.filter((p) => p !== trimmed);
      } else {
        rules.versioning = rules.versioning.filter((p) => p !== trimmed);
      }
      // Add to new array
      if (editRuleType === 'skip') {
        rules.skip = [...rules.skip, trimmed];
      } else if (editRuleType === 'priority') {
        rules.priority = [...rules.priority, trimmed];
      } else {
        rules.versioning = [...rules.versioning, trimmed];
      }
    }
    editingRuleIndex = null;
    saveRules();
  }
</script>

<svelte:head>
  <title>Hister - Rules</title>
</svelte:head>

<div class="flex flex-1 flex-col gap-8 overflow-y-auto px-6 py-8 md:gap-10 md:px-12 md:py-12">
  <!-- Section Header -->
  <div class="flex flex-col gap-4">
    <PageHeader color="hister-coral" size="lg" class="uppercase">Rules &amp; Aliases</PageHeader>
    <p class="font-inter text-text-brand-secondary max-w-175 text-base leading-relaxed md:text-lg">
      Configure Hister rules
    </p>
  </div>

  {#if message}
    <Alert.Root variant={isError ? 'error' : 'success'} class="shadow-brutal border-[3px]">
      {#if isError}
        <AlertCircle class="size-5 shrink-0" />
      {:else}
        <CheckCircle class="size-5 shrink-0" />
      {/if}
      <Alert.Description class="font-inter text-sm">{message}</Alert.Description>
    </Alert.Root>
  {/if}

  {#if loading}
    <div class="flex items-center justify-center py-16">
      <p class="font-inter text-text-brand-muted text-lg">Loading rules...</p>
    </div>
  {:else}
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Search Aliases Card -->
      <Card.Root>
        <Card.Header color="hister-indigo">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center bg-white/20">
            <Link2 class="text-background size-6" />
          </div>
          <div class="flex flex-col gap-1">
            <Card.Title
              class="font-space text-background text-xl font-extrabold tracking-[1px] uppercase"
              >Search aliases</Card.Title
            >
            <Card.Description class="font-inter text-background/70 text-sm"
              >{Object.keys(rules.aliases).length} aliases configured</Card.Description
            >
          </div>
        </Card.Header>

        <div
          class="bg-muted-surface border-brutal-border flex items-center border-b-[3px] px-4 py-4 md:px-5 md:py-5"
        >
          <form
            onsubmit={addAlias}
            class="flex w-full flex-col items-stretch gap-3 md:flex-row md:items-center"
          >
            <div class="flex items-center gap-3 md:contents">
              <Input
                type="text"
                variant="brutal"
                bind:value={newAliasKeyword}
                placeholder="keyword..."
                class="bg-card-surface focus-visible:border-hister-indigo h-10 w-28 px-3 md:w-35"
              />
              <Input
                type="text"
                variant="brutal"
                bind:value={newAliasValue}
                placeholder="expands to..."
                class="bg-card-surface focus-visible:border-hister-indigo h-10 flex-1 px-3"
              />
            </div>
            <Button
              type="submit"
              class="bg-hister-indigo font-space border-brutal-border brutal-press text-background h-10 gap-2 border-[3px] px-5 text-sm font-bold tracking-[1px] uppercase"
            >
              <Plus class="size-4 shrink-0" />
              Add
            </Button>
          </form>
        </div>

        <Card.Content class="flex-1 p-0">
          <!-- Aliases table -->
          <Table.Root>
            <Table.Header>
              <Table.Row
                class="bg-muted-surface border-brutal-border hover:bg-muted-surface border-b-[3px]"
              >
                <Table.Head
                  class="font-space text-text-brand-muted h-auto w-20 px-2 py-3 text-xs font-bold tracking-[1px] uppercase md:w-35 md:px-5"
                  >Keyword</Table.Head
                >
                <Table.Head
                  class="font-space text-text-brand-muted h-auto px-2 py-3 text-xs font-bold tracking-[1px] uppercase md:px-5"
                  >Expands to</Table.Head
                >
                <Table.Head class="h-auto w-16 px-2 py-3 md:w-20 md:px-5"></Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each Object.entries(rules.aliases) as [keyword, value]}
                <Table.Row class="border-brutal-border border-b-[3px]">
                  {#if editingAliasKey === keyword}
                    <Table.Cell class="px-2 py-2 md:px-3" colspan={2}>
                      <div class="flex items-center gap-2">
                        <Input
                          type="text"
                          variant="brutal"
                          bind:value={editAliasKeyword}
                          class="bg-card-surface focus-visible:border-hister-indigo h-8 w-20 px-2 text-sm md:w-28"
                        />
                        <Input
                          type="text"
                          variant="brutal"
                          bind:value={editAliasValue}
                          class="bg-card-surface focus-visible:border-hister-indigo h-8 flex-1 px-2 text-sm"
                          onkeydown={(e) => {
                            if (e.key === 'Enter') saveEditAlias();
                            if (e.key === 'Escape') cancelEditAlias();
                          }}
                        />
                      </div>
                    </Table.Cell>
                    <Table.Cell class="w-16 px-1 py-2 md:w-20 md:px-3">
                      <div class="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-hister-teal shrink-0 transition-colors"
                          onclick={saveEditAlias}
                        >
                          <Check class="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted shrink-0 transition-colors"
                          onclick={cancelEditAlias}
                        >
                          <X class="size-4" />
                        </Button>
                      </div>
                    </Table.Cell>
                  {:else}
                    <Table.Cell
                      class="font-fira text-text-brand w-20 px-2 py-3 text-sm font-semibold md:w-35 md:px-5"
                      >{keyword}</Table.Cell
                    >
                    <Table.Cell
                      class="font-fira text-text-brand-secondary max-w-0 truncate px-2 py-3 text-sm md:px-5"
                      >{value}</Table.Cell
                    >
                    <Table.Cell class="w-16 px-1 py-3 md:w-20 md:px-3">
                      <div class="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted hover:text-hister-indigo shrink-0 transition-colors"
                          onclick={() => startEditAlias(keyword, value)}
                        >
                          <Pencil class="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted hover:text-hister-rose shrink-0 transition-colors"
                          onclick={() => deleteAlias(keyword)}
                        >
                          <Trash2 class="size-4" />
                        </Button>
                      </div>
                    </Table.Cell>
                  {/if}
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>

          {#if Object.keys(rules.aliases).length === 0}
            <div class="flex flex-col items-center justify-center gap-3 py-10">
              <div
                class="flex h-12 w-12 items-center justify-center"
                style="background-color: color-mix(in srgb, var(--hister-indigo) 10%, transparent); color: var(--hister-indigo);"
              >
                <Link2 class="size-5" />
              </div>
              <p class="font-inter text-text-brand-muted text-sm">No aliases defined yet.</p>
            </div>
          {/if}
        </Card.Content>
      </Card.Root>

      <!-- Indexing Rules Card -->
      <Card.Root>
        <Card.Header color="hister-coral">
          <div class="flex h-12 w-12 shrink-0 items-center justify-center bg-white/20">
            <Shield class="text-background size-6" />
          </div>
          <div class="flex flex-col gap-1">
            <Card.Title
              class="font-space text-background text-xl font-extrabold tracking-[1px] uppercase"
              >Indexing rules</Card.Title
            >
            <Card.Description class="font-inter text-background/70 text-sm"
              >{ruleRows.length} rules configured · patterns use
              <a
                href="https://pkg.go.dev/regexp/syntax"
                target="_blank"
                class="text-page-bg underline opacity-80 hover:opacity-100">Go regexp</a
              > syntax</Card.Description
            >
          </div>
        </Card.Header>

        <div
          class="bg-muted-surface border-brutal-border flex items-center border-b-[3px] px-4 py-4 md:px-5 md:py-5"
        >
          <div class="flex w-full flex-col items-stretch gap-3 md:flex-row md:items-center">
            <div class="flex items-center gap-3 md:contents">
              <Input
                type="text"
                variant="brutal"
                bind:value={newRulePattern}
                placeholder="Enter Go regexp pattern"
                class="bg-card-surface focus-visible:border-hister-coral h-10 flex-1 px-3"
              />
              <select
                bind:value={newRuleType}
                class="bg-card-surface border-brutal-border font-space text-text-brand h-10 w-25 shrink-0 cursor-pointer appearance-none border-[3px] px-3 text-center text-xs font-bold tracking-[0.5px] outline-none md:w-27.5"
              >
                <option value="skip">SKIP</option>
                <option value="priority">PRIORITY</option>
                <option value="versioning">VERSION</option>
              </select>
            </div>
            <Button
              type="button"
              onclick={addRule}
              class="bg-hister-coral font-space border-brutal-border brutal-press text-background h-10 gap-2 border-[3px] px-5 text-sm font-bold tracking-[1px] uppercase"
            >
              <Plus class="size-4 shrink-0" />
              Add
            </Button>
          </div>
        </div>

        <Card.Content class="flex-1 p-0">
          <!-- Rules table -->
          <Table.Root>
            <Table.Header>
              <Table.Row
                class="bg-muted-surface border-brutal-border hover:bg-muted-surface border-b-[3px]"
              >
                <Table.Head
                  class="font-space text-text-brand-muted h-auto px-2 py-3 text-xs font-bold tracking-[1px] uppercase md:px-5"
                  >Pattern</Table.Head
                >
                <Table.Head
                  class="font-space text-text-brand-muted h-auto w-20 px-2 py-3 text-xs font-bold tracking-[1px] uppercase md:w-28 md:px-5"
                  >Type</Table.Head
                >
                <Table.Head class="h-auto w-16 px-2 py-3 md:w-20 md:px-5"></Table.Head>
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each ruleRows as row, i}
                <Table.Row class="border-brutal-border border-b-[3px]">
                  {#if editingRuleIndex === i}
                    <Table.Cell class="px-2 py-2 md:px-3" colspan={2}>
                      <div class="flex items-center gap-2">
                        <Input
                          type="text"
                          variant="brutal"
                          bind:value={editRulePattern}
                          class="bg-card-surface focus-visible:border-hister-coral h-8 flex-1 px-2 text-sm"
                          onkeydown={(e) => {
                            if (e.key === 'Enter') saveEditRule();
                            if (e.key === 'Escape') cancelEditRule();
                          }}
                        />
                        <select
                          bind:value={editRuleType}
                          class="bg-card-surface border-brutal-border font-space text-text-brand h-8 w-20 shrink-0 cursor-pointer appearance-none border-[3px] px-2 text-center text-xs font-bold tracking-[0.5px] outline-none md:w-25 md:px-3"
                        >
                          <option value="skip">SKIP</option>
                          <option value="priority">PRIORITY</option>
                          <option value="versioning">VERSION</option>
                        </select>
                      </div>
                    </Table.Cell>
                    <Table.Cell class="w-16 px-1 py-2 md:w-20 md:px-3">
                      <div class="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-hister-teal shrink-0 transition-colors"
                          onclick={saveEditRule}
                        >
                          <Check class="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted shrink-0 transition-colors"
                          onclick={cancelEditRule}
                        >
                          <X class="size-4" />
                        </Button>
                      </div>
                    </Table.Cell>
                  {:else}
                    <Table.Cell
                      class="font-fira text-text-brand max-w-0 truncate px-2 py-3 text-sm md:px-5"
                      >{row.pattern}</Table.Cell
                    >
                    <Table.Cell class="w-20 px-2 py-3 md:w-28 md:px-5">
                      <Badge
                        variant="default"
                        class="font-space border-0 px-2 py-1 text-xs font-bold tracking-[0.5px] uppercase md:px-3 {row.type ===
                        'skip'
                          ? 'bg-hister-rose text-background'
                          : row.type === 'priority'
                            ? 'bg-hister-teal text-background'
                            : 'text-background bg-violet-500'}"
                      >
                        {row.type}
                      </Badge>
                    </Table.Cell>
                    <Table.Cell class="w-16 px-1 py-3 md:w-20 md:px-3">
                      <div class="flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted hover:text-hister-coral shrink-0 transition-colors"
                          onclick={() => startEditRule(i)}
                        >
                          <Pencil class="size-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          class="text-text-brand-muted hover:text-hister-rose shrink-0 transition-colors"
                          onclick={() => removeRule(row.pattern, row.type)}
                        >
                          <Trash2 class="size-4" />
                        </Button>
                      </div>
                    </Table.Cell>
                  {/if}
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>

          {#if ruleRows.length === 0}
            <div class="flex flex-col items-center justify-center gap-3 py-10">
              <div
                class="flex h-12 w-12 items-center justify-center"
                style="background-color: color-mix(in srgb, var(--hister-coral) 10%, transparent); color: var(--hister-coral);"
              >
                <Shield class="size-5" />
              </div>
              <p class="font-inter text-text-brand-muted text-sm">No rules defined yet.</p>
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>
  {/if}
</div>
