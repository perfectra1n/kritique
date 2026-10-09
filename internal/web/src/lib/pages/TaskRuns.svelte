<script lang="ts">
  // Every task run of the account, newest first: what ran on which issue
  // or pull request, on what event, and how it ended.
  import { tick, untrack } from 'svelte';
  import { getJSON } from '../api.svelte';
  import { href, navigate, replace } from '../router.svelte';
  import { TASK_RUN_STATUSES, taskRunFilter, type TaskRunFilter } from '../routes';
  import { Paged, Resource, live } from '../resource.svelte';
  import { accountApi, pullRoute, repoRoute } from '../links';
  import { duration, taskTone } from '../format';
  import { formatTokens, type Parsed, type TokenSpec } from '../tokensearch';
  import { listKeys } from '../listkeys';
  import type { Page, Repository, TaskRun, TaskRunStatus } from '../types';
  import StateView from '../components/StateView.svelte';
  import LoadMore from '../components/LoadMore.svelte';
  import TokenSearch from '../components/TokenSearch.svelte';
  import Segmented from '../components/Segmented.svelte';
  import Pill from '../components/Pill.svelte';
  import Time from '../components/Time.svelte';

  const STATUSES: readonly { value: TaskRunStatus | ''; label: string }[] = [
    { value: '', label: 'All' },
    { value: 'queued', label: 'Queued' },
    { value: 'running', label: 'Running' },
    { value: 'succeeded', label: 'Succeeded' },
    { value: 'failed', label: 'Failed' },
    { value: 'skipped', label: 'Skipped' },
  ];

  let { slug, filter }: { slug: string; filter?: TaskRunFilter } = $props();

  const account = $derived(accountApi(slug));
  function query(after?: string): string {
    const p = new URLSearchParams({ limit: '50' });
    for (const k of ['repo', 'task', 'status'] as const) {
      const v = filter?.[k];
      if (v) p.set(k, v);
    }
    if (after) p.set('cursor', after);
    return `${account}/task-runs?${p}`;
  }
  const paged = new Paged<TaskRun>(query, (r) => r.id);
  const res = paged.first;
  const repos = new Resource(() => getJSON<Page<Repository>>(`${account}/repos?limit=100`));
  $effect(() => {
    void paged.load();
  });
  $effect(() => {
    void repos.load();
  });
  $effect(() => live((e) => e.account === slug && e.kind === 'task_run', () => void paged.load()));

  // A bare word in the box is a task's name: the list has no other text to
  // search.
  const specs = $derived<TokenSpec[]>([
    { key: 'repo', hint: 'a repository', values: (repos.data?.items ?? []).map((r) => r.fullName) },
    { key: 'task', hint: 'a task name', values: [...new Set(paged.items.map((r) => r.task))].sort(), open: true },
    { key: 'status', hint: TASK_RUN_STATUSES.join(', '), values: TASK_RUN_STATUSES },
  ]);
  const boxText = (f: TaskRunFilter | undefined) => formatTokens(specs, { repo: f?.repo, task: f?.task, status: f?.status });

  // As on the findings list: written is the box's text for the filter it
  // last wrote to the URL, and any other change to the filter rewrites it.
  let searchEl = $state<HTMLInputElement | undefined>(undefined);
  let text = $state(untrack(() => boxText(filter)));
  let written = untrack(() => boxText(filter));
  function onapply(p: Parsed): void {
    const f = taskRunFilter({ ...p.tokens, task: p.tokens.task ?? p.q });
    written = boxText(f);
    replace({ name: 'tasks', slug, filter: f });
  }
  $effect(() => {
    const t = boxText(filter);
    if (t === written) return;
    written = t;
    text = t;
  });

  function clearFilters(): void {
    replace({ name: 'tasks', slug });
    searchEl?.focus();
  }

  const runOf = (r: TaskRun) => ({ name: 'taskRun' as const, slug, id: r.id });

  let selectedId = $state('');
  const selected = $derived(paged.items.findIndex((r) => r.id === selectedId));
  async function select(i: number): Promise<void> {
    const r = paged.items[i];
    if (!r) return;
    selectedId = r.id;
    await tick();
    document.querySelector(`.task-runs [data-index="${i}"]`)?.scrollIntoView({ block: 'nearest' });
  }
  $effect(() =>
    listKeys({
      count: () => paged.items.length,
      get: () => selected,
      set: (i) => void select(i),
      open: (i) => {
        const r = paged.items[i];
        if (r) navigate(runOf(r));
      },
      focusSearch: () => searchEl?.focus(),
    }),
  );

  function onRowClick(e: MouseEvent, r: TaskRun): void {
    if ((e.target as Element).closest('a, button') || getSelection()?.toString()) return;
    navigate(runOf(r));
  }
</script>

<svelte:head><title>Task runs · {slug} · kritique</title></svelte:head>

<main class="page">
  <div class="page-inner">
    <header class="page-head"><h1>Task runs</h1></header>
    <div class="toolbar" role="search">
      <TokenSearch
        id="task-run-search"
        label="Search task runs"
        placeholder="Task name, or filter by repo:, task: or status:"
        {specs}
        bind:text
        bind:input={searchEl}
        ready={!!repos.data}
        {onapply}
      />
      <Segmented
        label="Status"
        options={STATUSES}
        value={filter?.status ?? ''}
        onchange={(status) => replace({ name: 'tasks', slug, filter: taskRunFilter({ ...filter, status }) })}
      />
    </div>
    <StateView {res} retry={() => res.load()}>
      {#snippet children()}
        {#if paged.items.length === 0 && filter}
          <div class="state-msg">
            <span>No task runs match these filters.</span>
            <button class="btn btn-small" onclick={clearFilters}>Clear filters</button>
          </div>
        {:else if paged.items.length === 0}
          <p class="state-msg">No task runs yet: they appear here as events reach the tasks a repository runs.</p>
        {:else}
          <div class="table-wrap table-card">
            <table class="data task-runs">
              <thead>
                <tr>
                  <th scope="col">Status</th>
                  <th scope="col">Task</th>
                  <th scope="col">Subject</th>
                  <th scope="col">Trigger</th>
                  <th scope="col">Mode</th>
                  <th scope="col">Outcome</th>
                  <th scope="col" class="num">Queued</th>
                  <th scope="col" class="num">Took</th>
                </tr>
              </thead>
              <tbody>
                {#each paged.items as r, i (r.id)}
                  <tr class="task-run-row" class:selected={i === selected} data-index={i} onclick={(e) => onRowClick(e, r)}>
                    <td><Pill tone={taskTone[r.status]} label={r.status} /></td>
                    <td class="mono">
                      <a href={href(runOf(r))} aria-current={i === selected ? 'true' : undefined}>{r.task}</a>
                    </td>
                    <td class="mono small">
                      {#if r.subjectKind === 'pull'}
                        <a href={href(pullRoute(slug, { repository: r.repository, number: r.subjectNumber }))} title="Pull request">{r.repository}#{r.subjectNumber}</a>
                      {:else if r.subjectNumber}
                        <span title="Issue">{r.repository}#{r.subjectNumber}</span>
                      {:else}
                        <a href={href(repoRoute(slug, r.repository))}>{r.repository}</a>
                      {/if}
                    </td>
                    <td class="mono small">{r.trigger || '—'}</td>
                    <td>{r.mode}</td>
                    <td class="small">
                      {#if r.error}<span class="error-text">{r.error}</span>
                      {:else if r.reason}<span class="muted">{r.reason}</span>
                      {:else if r.droppedCount}<span>{r.droppedCount} dropped</span>
                      {:else}—{/if}
                    </td>
                    <td class="num"><Time iso={r.createdAt} /></td>
                    <td class="num">{r.durationMs !== null ? duration(r.durationMs) : '…'}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          <LoadMore {paged} />
        {/if}
      {/snippet}
    </StateView>
    <p class="muted small key-hints"><kbd>j</kbd>/<kbd>k</kbd> move · <kbd>⏎</kbd> open · <kbd>/</kbd> search</p>
  </div>
</main>
