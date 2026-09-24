<script lang="ts">
  // Search view: debounced live search (~250 ms), the full CLI filter set,
  // progress bar + cancel for large corpora, highlighted result cards, and
  // click-through to the viewer at the hit (R-D11).
  import { api, emptyFilter, onProgress, type FilterOptions, type SearchOutcome } from '../lib/api';
  import { fmtDay, shortProject, idPrefix } from '../lib/format';
  import { debounce } from '../lib/debounce';
  import { go, view } from '../lib/stores.svelte';
  import { openSession } from '../lib/viewer.svelte';
  import FilterBar from '../components/FilterBar.svelte';
  import Highlight from '../components/Highlight.svelte';
  import Notes from '../components/Notes.svelte';
  import Loader from '../components/Loader.svelte';

  type Result = {
    session: { id: string; agent: string; project: string; started_at: string | null };
    hits: { kind: string; role: string; timestamp: string | null; context: string; line: string; match_start: number; match_end: number }[];
  };

  let query = $state('');
  let filters = $state<FilterOptions>(emptyFilter());
  let agents = $state<string[]>([]);
  let maxHits = $state(200);
  let caseSensitive = $state(false);
  let regex = $state(false);

  let outcome = $state<SearchOutcome | null>(null);
  let results = $state<Result[]>([]);
  let error = $state('');
  let searched = $state(false); // true once a run completed (drives "no matches")
  let progress = $state<{ scanned: number; hits: number } | null>(null);
  let searching = $state(false); // true while a run is in flight (hides stale results)

  api.diagnostics().then((d) => (agents = d.agents.map((a) => a.name)));
  const stopProgress = onProgress((e) => {
    if (e.op === 'search') progress = { scanned: e.scanned, hits: e.hits };
  });

  let seq = 0;

  async function run(q: string) {
    const mine = ++seq;
    progress = null;
    if (q.trim() === '') {
      if (mine !== seq) return;
      outcome = null;
      results = [];
      searched = false;
      error = '';
      searching = false;
      return;
    }
    searching = true;
    try {
      const o = await api.search(q, {
        filter: { ...filters },
        maxHits: maxHits,
        caseSensitive,
        regex,
      });
      if (mine !== seq) return; // a newer query superseded this run
      outcome = o;
      results = o.results as unknown as Result[];
      searched = true;
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      outcome = null;
      results = [];
      searched = false;
      error = String(e);
    } finally {
      if (mine === seq) {
        progress = null;
        searching = false;
      }
    }
  }
  const runDebounced = debounce(run, 250);

  function onInput() {
    runDebounced(query);
  }
  function onEnter() {
    void run(query);
  }

  function openAt(res: Result, hit: Result['hits'][number]) {
    openSession(res.session.id, { line: hit.line, kind: hit.kind, role: hit.role }, 'search');
    go('viewer');
  }

  const running = $derived(progress !== null);
</script>

<header class="page-head">
  <div>
    <div class="kicker">// 02 · SEARCH</div>
    <h1>Search sessions</h1>
    <p class="lede">Find messages and inspect the exact session context around each match.</p>
  </div>
  <div class="page-mark">LOCAL INDEX<br /><strong>LIVE QUERY</strong></div>
</header>
<div class="queryrow">
  <div class="searchwrap">
    <svg
      class="sicon"
      viewBox="0 0 24 24"
      width="15"
      height="15"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="11" cy="11" r="7" />
      <path d="m21 21-4.3-4.3" />
    </svg>
    <input
      class="query"
      type="text"
      bind:value={query}
      oninput={onInput}
      onkeydown={(e) => e.key === 'Enter' && onEnter()}
      placeholder="search all session entries across agents"
    />
  </div>
  <label class="toggle"><input type="checkbox" bind:checked={caseSensitive} oninput={onInput} /> Aa</label>
  <label class="toggle"><input type="checkbox" bind:checked={regex} oninput={onInput} /> .*</label>
  <label class="toggle cap">
    max hits <input class="maxhits" type="number" min="0" bind:value={maxHits} oninput={onInput} />
  </label>
</div>
<FilterBar {agents} bind:filters />
{#if !searching}<Notes notes={outcome?.notes ?? []} />{/if}

{#if running}
  <div class="progress">
    <span class="scan">scanning… {progress.scanned} sessions, {progress.hits} hits</span>
    <div class="track"><div class="fill" style="width: {Math.min(100, progress.scanned * 2)}%"></div></div>
    <button class="btn" onclick={() => api.cancel()}>Cancel</button>
  </div>
{/if}
{#if !searching && error}<p class="err">{error}</p>{/if}

{#if searching}
  {#if !running}
    <Loader label="searching…" />
  {/if}
{:else if results.length > 0}
  <div class="cards fadein">
    {#each results as res}
      <div class="card">
        <div class="head">
          <span class="chip">{res.session.agent}</span>
          <span class="chip plain">{shortProject(res.session.project)}</span>
          <span class="fact">{fmtDay(res.session.started_at)}</span>
          <span class="fact">sess {idPrefix(res.session.id)}</span>
        </div>
        {#each res.hits as hit}
          <button class="hit" onclick={() => openAt(res, hit)} title="open session at this hit">
            {#if hit.context}<div class="context">{hit.context}</div>{/if}
            <div class="line">
              <span class="arr">&rarr;</span>
              <Highlight line={hit.line} start={hit.match_start} end={hit.match_end} />
            </div>
          </button>
        {/each}
      </div>
    {/each}
  </div>
  {#if outcome?.truncated}<p class="meta">max hits reached — refine the query or raise the cap</p>{/if}
  <p class="meta">{outcome?.hits} hits in {results.length} sessions</p>
{:else if searched && !error}
  <div class="empty">no matches</div>
{/if}
<style>
  .queryrow {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-bottom: 6px;
  }
  .searchwrap {
    position: relative;
    flex: 1;
  }
  .sicon {
    position: absolute;
    left: 11px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--muted);
    pointer-events: none;
  }
  .query {
    width: 100%;
    padding: 9px 12px 9px 34px;
    font-size: 14px;
    box-shadow: var(--shadow-1);
  }
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    border: 1px solid var(--border);
    border-radius: 999px;
    color: var(--muted);
    font-family: var(--mono);
    font-size: 12px;
    white-space: nowrap;
    cursor: pointer;
    user-select: none;
    transition:
      color var(--speed) ease,
      background var(--speed) ease,
      border-color var(--speed) ease;
  }
  .toggle:hover {
    color: var(--text-2);
    border-color: var(--border-strong);
  }
  .toggle:has(input:checked) {
    color: var(--accent);
    background: var(--accent-soft);
    border-color: color-mix(in srgb, var(--accent) 35%, transparent);
  }
  .toggle input[type='checkbox'] {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .toggle:has(input:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  .toggle.cap {
    font-family: inherit;
    gap: 7px;
    color: var(--text-2);
  }
  .maxhits {
    width: 58px;
    padding: 2px 6px;
    font-family: var(--mono);
    font-size: 12px;
    background: var(--inset);
  }
  .cards {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-top: 12px;
  }
  .card {
    background: var(--panel);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-l);
    padding: 12px 16px;
    transition: border-color var(--speed) ease;
  }
  .card:hover {
    border-color: var(--border);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-bottom: 8px;
  }
  .fact {
    color: var(--muted);
    font-size: 11.5px;
    font-family: var(--mono);
  }
  .hit {
    display: block;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    color: var(--text);
    font: inherit;
    padding: 5px 8px;
    margin: 0 -8px;
    cursor: pointer;
    border-radius: var(--radius-s);
    transition: background var(--speed) ease;
  }
  .hit:hover {
    background: var(--panel-hover);
  }
  .context {
    color: var(--muted);
    font-size: 11.5px;
  }
  .line {
    font-family: var(--mono);
    font-size: 12.5px;
    display: flex;
    gap: 8px;
    align-items: baseline;
  }
  .arr {
    color: var(--accent);
    flex-shrink: 0;
  }
</style>
