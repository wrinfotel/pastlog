<script lang="ts">
  // Stats view: the CLI's exact column set (key, sessions, messages, input,
  // output, reasoning, cache read/write, total, cost-when-present) with
  // plain CSS bars on the total column — no chart library (spec §4.3).
  import { api, emptyFilter, onProgress, type FilterOptions } from '../lib/api';
  import { fmtInt } from '../lib/format';
  import { debounce } from '../lib/debounce';
  import FilterBar from '../components/FilterBar.svelte';
  import Notes from '../components/Notes.svelte';

  type Row = {
    key: string;
    sessions: number;
    messages: number;
    tokens: { input: number; output: number; reasoning: number; cache_read: number; cache_write: number; total: number };
    cost_usd: number | null;
  };

  let filters = $state<FilterOptions>(emptyFilter());
  let agents = $state<string[]>([]);
  let by = $state<'agent' | 'project' | 'day' | 'model'>('agent');
  let model = $state('');
  let rows = $state<Row[]>([]);
  let notes = $state<string[]>([]);
  let error = $state('');
  let progress = $state<{ scanned: number } | null>(null);

  api.diagnostics().then((d) => (agents = d.agents.map((a) => a.name)));
  const stopProgress = onProgress((e) => {
    if (e.op === 'stats') progress = { scanned: e.scanned };
  });

  async function load() {
    progress = null;
    try {
      const o = await api.stats({
        filter: { ...filters },
        by,
        model,
      });
      rows = o.rows as Row[];
      notes = o.notes ?? [];
      error = '';
    } catch (e) {
      rows = [];
      notes = [];
      error = String(e);
    } finally {
      progress = null;
    }
  }
  const loadDebounced = debounce(load, 250);

  $effect(() => {
    void filters;
    void by;
    void model;
    loadDebounced();
  });

  const maxTotal = $derived(Math.max(1, ...rows.map((r) => r.tokens.total)));
  const hasCost = $derived(rows.some((r) => r.cost_usd !== null && r.cost_usd !== undefined));

  function cancel() {
    void api.cancel();
  }
</script>

<h1>Stats</h1>
<div class="byrow">
  <label>
    by
    <select bind:value={by} onchange={loadDebounced}>
      <option value="agent">agent</option>
      <option value="project">project</option>
      <option value="day">day</option>
      <option value="model">model</option>
    </select>
  </label>
  <label>
    model
    <input type="text" bind:value={model} placeholder="substring" oninput={loadDebounced} />
  </label>
</div>
<FilterBar {agents} bind:filters />
<Notes {notes} />

{#if progress}
  <div class="progress">
    <span>aggregating… {progress.scanned} sessions</span>
    <button onclick={cancel}>Cancel</button>
  </div>
{/if}
{#if error}<p class="err">{error}</p>{/if}

{#if rows.length > 0}
  <table>
    <thead>
      <tr>
        <th>{by}</th>
        <th class="r">sessions</th>
        <th class="r">messages</th>
        <th class="r">input</th>
        <th class="r">output</th>
        <th class="r">reasoning</th>
        <th class="r">cache read</th>
        <th class="r">cache write</th>
        <th class="r total">total</th>
        {#if hasCost}<th class="r">cost usd</th>{/if}
      </tr>
    </thead>
    <tbody>
      {#each rows as row}
        <tr>
          <td class="key">{row.key}</td>
          <td class="r">{fmtInt(row.sessions)}</td>
          <td class="r">{fmtInt(row.messages)}</td>
          <td class="r">{fmtInt(row.tokens.input)}</td>
          <td class="r">{fmtInt(row.tokens.output)}</td>
          <td class="r">{fmtInt(row.tokens.reasoning)}</td>
          <td class="r">{fmtInt(row.tokens.cache_read)}</td>
          <td class="r">{fmtInt(row.tokens.cache_write)}</td>
          <td class="r total">
            <span class="bar" style="width: {(row.tokens.total / maxTotal) * 100}%"></span>
            <span class="num">{fmtInt(row.tokens.total)}</span>
          </td>
          {#if hasCost}
            <td class="r">{row.cost_usd === null || row.cost_usd === undefined ? '-' : row.cost_usd.toFixed(2)}</td>
          {/if}
        </tr>
      {/each}
    </tbody>
  </table>
{:else if !error && !progress}
  <p class="meta">no usage data for this selection</p>
{/if}

<style>
  .byrow {
    display: flex;
    gap: 14px;
    align-items: end;
    margin-bottom: 6px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
    color: var(--muted);
  }
  select,
  input {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 5px 8px;
  }
  table {
    border-collapse: collapse;
    margin-top: 10px;
    width: 100%;
  }
  th,
  td {
    padding: 6px 12px 6px 0;
    border-bottom: 1px solid var(--border);
    font-size: 13px;
  }
  th {
    color: var(--muted);
    font-size: 12px;
    text-align: right;
  }
  th:first-child {
    text-align: left;
  }
  .r {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .key {
    color: var(--accent);
  }
  .total {
    position: relative;
    min-width: 140px;
  }
  .bar {
    position: absolute;
    right: 0;
    top: 25%;
    height: 50%;
    background: color-mix(in srgb, var(--accent) 25%, transparent);
    border-radius: 2px;
  }
  .num {
    position: relative;
  }
  .meta {
    color: var(--muted);
    margin-top: 14px;
  }
  .progress {
    display: flex;
    align-items: center;
    gap: 12px;
    margin: 8px 0;
    color: var(--muted);
    font-size: 12px;
  }
  .progress button {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 4px 10px;
    cursor: pointer;
  }
  .err {
    color: #f87171;
  }
</style>
