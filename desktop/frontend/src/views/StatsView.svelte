<script lang="ts">
  // Stats view: the CLI's exact column set (key, sessions, messages, input,
  // output, reasoning, cache read/write, total, cost-when-present) with
  // plain CSS bars on the total column — no chart library (spec §4.3).
  import { api, emptyFilter, onProgress, type FilterOptions } from '../lib/api';
  import { fmtInt } from '../lib/format';
  import { debounce } from '../lib/debounce';
  import FilterBar from '../components/FilterBar.svelte';
  import Notes from '../components/Notes.svelte';
  import Loader from '../components/Loader.svelte';

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
  let loading = $state(true);

  api.diagnostics().then((d) => (agents = d.agents.map((a) => a.name)));
  const stopProgress = onProgress((e) => {
    if (e.op === 'stats') progress = { scanned: e.scanned };
  });

  let seq = 0;

  async function load() {
    const mine = ++seq;
    loading = true;
    progress = null;
    try {
      const o = await api.stats({
        filter: { ...filters },
        by,
        model,
      });
      if (mine !== seq) return; // a newer selection superseded this load
      rows = o.rows as Row[];
      notes = o.notes ?? [];
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      rows = [];
      notes = [];
      error = String(e);
    } finally {
      if (mine === seq) {
        progress = null;
        loading = false;
      }
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

<header class="page-head">
  <div>
    <div class="eyebrow">Analyze / 03</div>
    <h1>Statistics</h1>
    <p class="lede">Aggregate local usage by agent, project, day, or model.</p>
  </div>
  <div class="page-mark">USAGE REPORT<br /><strong>LOCAL DATA</strong></div>
</header>
<FilterBar {agents} bind:filters>
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
</FilterBar>
{#if !loading}<Notes {notes} />{/if}

{#if progress}
  <div class="progress">
    <span>aggregating… {progress.scanned} sessions</span>
    <button class="btn" onclick={cancel}>Cancel</button>
  </div>
{/if}
{#if !loading && error}<p class="err">{error}</p>{/if}

{#if loading}
  {#if !progress}
    <Loader label="aggregating…" />
  {/if}
{:else if rows.length > 0}
  <div class="panel tablewrap fadein">
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
  </div>
{:else if !error}
  <p class="meta">no usage data for this selection</p>
{/if}

<style>
  .page-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 22px;
    padding-bottom: 20px;
    border-bottom: 1px solid var(--border);
  }
  .eyebrow {
    margin-bottom: 7px;
    color: var(--accent);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }
  h1 { margin-bottom: 5px; font-size: 25px; letter-spacing: -0.025em; }
  .lede { margin: 0; color: var(--muted); font-size: 13px; }
  .page-mark { color: var(--faint); font: 10px/1.6 var(--mono); letter-spacing: 0.08em; text-align: right; }
  .page-mark strong { color: var(--accent); font-weight: 600; }
  /* snippet controls live inside FilterBar's bar but keep this view's scope —
     mirror the bar's own field metrics so the row reads as one */
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--muted);
  }
  select,
  input {
    height: 30px;
    min-width: 150px;
    padding: 0 9px;
    font-size: 12.5px;
    background: var(--panel);
  }
  select {
    min-width: 110px;
    padding-right: 26px;
  }
  .tablewrap {
    margin-top: 12px;
    padding: 6px 16px 8px;
  }
  .key {
    color: var(--text);
    font-weight: 600;
  }
  .r {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  td.r {
    color: var(--text-2);
  }
  .total {
    position: relative;
    min-width: 150px;
  }
  .bar {
    position: absolute;
    right: 14px;
    top: 20%;
    height: 60%;
    background: linear-gradient(90deg, var(--accent-soft), var(--accent-glow) 70%);
    border-radius: 3px;
  }
  .num {
    position: relative;
    color: var(--text);
    font-weight: 600;
  }
</style>
