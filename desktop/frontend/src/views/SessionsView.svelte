<script lang="ts">
  // Sessions: the virtualized `pastlog sessions` table with the full filter
  // set, newest first; a row opens the viewer (T14 wires the target).
  import { api, emptyFilter, type FilterOptions, type ListOutcome } from '../lib/api';
  import { fmtBytes, fmtDate, fmtInt, idPrefix } from '../lib/format';
  import { debounce } from '../lib/debounce';
  import { openSession } from '../lib/viewer.svelte';
  import FilterBar from '../components/FilterBar.svelte';
  import Notes from '../components/Notes.svelte';
  import VirtualList from '../components/VirtualList.svelte';

  let filters = $state<FilterOptions>(emptyFilter());
  let agents = $state<string[]>([]);
  let data = $state<ListOutcome | null>(null);
  let error = $state('');

  api.diagnostics().then((d) => {
    agents = d.agents.map((a) => a.name);
  });

  async function load(f: FilterOptions) {
    try {
      data = await api.sessions(f);
      error = '';
    } catch (e) {
      error = String(e);
    }
  }
  const loadDebounced = debounce(load, 250);

  $effect(() => {
    const snapshot: FilterOptions = { ...filters };
    void snapshot;
    loadDebounced({ ...filters });
  });

  function open(id: string) {
    openSession(id);
  }
</script>

<h1>Sessions</h1>
<FilterBar {agents} bind:filters />
<Notes notes={data?.notes ?? []} />
{#if error}<p class="err">{error}</p>{/if}
{#if data}
  <div class="head">
    <span class="c-agent">agent</span>
    <span class="c-proj">project</span>
    <span class="c-date">date</span>
    <span class="c-msg">messages</span>
    <span class="c-size">size</span>
    <span class="c-id">id</span>
  </div>
  <div class="list">
    <VirtualList items={data.sessions} itemHeight={30}>
      {#snippet row(s)}
        <button class="row" onclick={() => open(s.id)} title={s.title || s.id}>
          <span class="c-agent">{s.agent}</span>
          <span class="c-proj">{s.project || '-'}</span>
          <span class="c-date">{fmtDate(s.started_at)}</span>
          <span class="c-msg">{fmtInt(s.messages)}</span>
          <span class="c-size">{fmtBytes(s.size_bytes)}</span>
          <span class="c-id">{idPrefix(s.id)}</span>
        </button>
      {/snippet}
    </VirtualList>
  </div>
  <p class="meta">{fmtInt(data.sessions.length)} sessions</p>
{/if}

<style>
  .head,
  .row {
    display: grid;
    grid-template-columns: 110px minmax(120px, 1fr) 130px 90px 80px 90px;
    gap: 8px;
    padding: 0 4px;
    align-items: center;
  }
  .head {
    color: var(--muted);
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .list {
    height: calc(100vh - 300px);
    min-height: 220px;
  }
  .row {
    width: 100%;
    background: none;
    border: 0;
    border-bottom: 1px solid var(--border);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .row:hover {
    background: var(--panel);
  }
  .c-agent {
    color: var(--accent);
  }
  .c-date,
  .c-msg,
  .c-size,
  .c-id,
  .c-proj {
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .c-msg,
  .c-size {
    text-align: right;
  }
  .meta {
    color: var(--muted);
    font-size: 12px;
  }
  .err {
    color: #f87171;
  }
</style>
