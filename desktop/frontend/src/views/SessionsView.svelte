<script lang="ts">
  // Sessions: the virtualized `pastlog sessions` table with the full filter
  // set, newest first; a row opens the viewer (T14 wires the target).
  import { api, emptyFilter, type FilterOptions, type ListOutcome } from '../lib/api';
  import { fmtBytes, fmtDate, fmtInt, idPrefix } from '../lib/format';
  import { debounce } from '../lib/debounce';
  import { go } from '../lib/stores.svelte';
  import { openSession } from '../lib/viewer.svelte';
  import FilterBar from '../components/FilterBar.svelte';
  import Notes from '../components/Notes.svelte';
  import VirtualList from '../components/VirtualList.svelte';
  import Loader from '../components/Loader.svelte';

  let filters = $state<FilterOptions>(emptyFilter());
  let agents = $state<string[]>([]);
  let data = $state<ListOutcome | null>(null);
  let error = $state('');
  let loading = $state(true);

  api.diagnostics().then((d) => {
    agents = d.agents.map((a) => a.name);
  });

  let seq = 0;

  async function load(f: FilterOptions) {
    const mine = ++seq;
    loading = true;
    try {
      const d = await api.sessions(f);
      if (mine !== seq) return; // a newer filter superseded this load
      data = d;
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      error = String(e);
    } finally {
      if (mine === seq) loading = false;
    }
  }
  const loadDebounced = debounce(load, 250);

  $effect(() => {
    const snapshot: FilterOptions = { ...filters };
    void snapshot;
    loadDebounced({ ...filters });
  });

  function open(id: string) {
    openSession(id, undefined, 'sessions');
    go('viewer');
  }
</script>

<header class="page-head">
  <div>
    <div class="kicker">// 03 · SESSIONS</div>
    <h1>Sessions</h1>
    <p class="lede">Every indexed session across agents, newest first.</p>
  </div>
  <div class="page-mark">SESSION LIST<br /><strong>READ-ONLY</strong></div>
</header>
<FilterBar {agents} bind:filters />
{#if !loading}
  <Notes notes={data?.notes ?? []} />
  {#if error}<p class="err">{error}</p>{/if}
{/if}
<div class="panel tablewrap">
  <div class="head">
    <span class="c-agent">agent</span>
    <span class="c-proj">project</span>
    <span class="c-date">date</span>
    <span class="c-msg">messages</span>
    <span class="c-size">size</span>
    <span class="c-id">id</span>
  </div>
  <div class="list">
    {#if loading}
      <Loader label="loading sessions…" />
    {:else if data}
      <div class="vwrap fadein">
        <VirtualList items={data.sessions} itemHeight={32}>
          {#snippet row(s)}
            <button class="row" onclick={() => open(s.id)} title={s.title || s.id}>
              <span class="c-agent"><span class="chip">{s.agent}</span></span>
              <span class="c-proj">{s.project || '-'}</span>
              <span class="c-date">{fmtDate(s.started_at)}</span>
              <span class="c-msg">{fmtInt(s.messages)}</span>
              <span class="c-size">{fmtBytes(s.size_bytes)}</span>
              <span class="c-id">{idPrefix(s.id)}</span>
            </button>
          {/snippet}
        </VirtualList>
      </div>
    {/if}
  </div>
</div>
{#if !loading && data}
  <p class="meta fadein">{fmtInt(data.sessions.length)} sessions</p>
{/if}

<style>
  .tablewrap {
    margin-top: 12px;
    padding: 4px 14px 6px;
  }
  .head,
  .row {
    display: grid;
    grid-template-columns: 110px minmax(120px, 1fr) 130px 90px 80px 90px;
    gap: 10px;
    padding: 0 4px;
    align-items: center;
  }
  .head {
    position: sticky;
    top: 0;
    z-index: 1;
    color: var(--muted);
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    padding: 8px 14px 7px 4px; /* +10px: the list's reserved scrollbar lane */
    background: var(--panel);
    border-bottom: 1px solid var(--border);
  }
  .list {
    height: calc(100vh - 318px);
    min-height: 220px;
  }
  .vwrap {
    height: 100%;
  }
  .row {
    width: 100%;
    height: 100%;
    background: none;
    border: 0;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--speed) ease;
  }
  .row:hover {
    background: var(--panel-hover);
  }
  .c-agent .chip {
    font-size: 10.5px;
    padding: 1px 7px;
  }
  .c-date,
  .c-msg,
  .c-size,
  .c-id,
  .c-proj {
    color: var(--muted);
    font-size: 12.5px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .c-date,
  .c-id {
    font-family: var(--mono);
    font-size: 11.5px;
  }
  .c-proj {
    color: var(--text-2);
  }
  .c-msg,
  .c-size {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
</style>
