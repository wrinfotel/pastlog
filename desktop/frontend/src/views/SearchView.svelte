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

  api.diagnostics().then((d) => (agents = d.agents.map((a) => a.name)));
  const stopProgress = onProgress((e) => {
    if (e.op === 'search') progress = { scanned: e.scanned, hits: e.hits };
  });

  async function run(q: string) {
    progress = null;
    if (q.trim() === '') {
      outcome = null;
      results = [];
      searched = false;
      error = '';
      return;
    }
    try {
      const o = await api.search(q, {
        filter: { ...filters },
        maxHits: maxHits,
        caseSensitive,
        regex,
      });
      outcome = o;
      results = o.results as unknown as Result[];
      searched = true;
      error = '';
    } catch (e) {
      outcome = null;
      results = [];
      searched = false;
      error = String(e);
    } finally {
      progress = null;
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
    openSession(res.session.id, { line: hit.line, kind: hit.kind, role: hit.role });
    go('viewer');
  }

  const running = $derived(progress !== null);
</script>

<h1>Search</h1>
<div class="queryrow">
  <input
    class="query"
    type="text"
    bind:value={query}
    oninput={onInput}
    onkeydown={(e) => e.key === 'Enter' && onEnter()}
    placeholder="search all session entries across agents"
  />
  <label class="toggle"><input type="checkbox" bind:checked={caseSensitive} oninput={onInput} /> Aa</label>
  <label class="toggle"><input type="checkbox" bind:checked={regex} oninput={onInput} /> .*</label>
  <label class="toggle">
    max hits <input class="maxhits" type="number" min="0" bind:value={maxHits} oninput={onInput} />
  </label>
</div>
<FilterBar {agents} bind:filters />
<Notes notes={outcome?.notes ?? []} />

{#if running}
  <div class="progress">
    <span>scanning… {progress.scanned} sessions, {progress.hits} hits</span>
    <div class="bar"><div class="fill" style="width: {Math.min(100, progress.scanned * 2)}%"></div></div>
    <button onclick={() => api.cancel()}>Cancel</button>
  </div>
{/if}
{#if error}<p class="err">{error}</p>{/if}

{#if results.length > 0}
  <div class="cards">
    {#each results as res}
      <div class="card">
        <div class="head">
          {res.session.agent} · {shortProject(res.session.project)} · {fmtDay(res.session.started_at)}
          · sess {idPrefix(res.session.id)}
        </div>
        {#each res.hits as hit}
          <button class="hit" onclick={() => openAt(res, hit)} title="open session at this hit">
            {#if hit.context}<div class="context">{hit.context}</div>{/if}
            <div class="line">→ <Highlight line={hit.line} start={hit.match_start} end={hit.match_end} /></div>
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
    margin-bottom: 4px;
  }
  .query {
    flex: 1;
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 9px 12px;
    font-size: 15px;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--muted);
    font-size: 12px;
    white-space: nowrap;
  }
  .maxhits {
    width: 70px;
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 5px 8px;
  }
  .progress {
    display: flex;
    align-items: center;
    gap: 12px;
    margin: 8px 0;
    color: var(--muted);
    font-size: 12px;
  }
  .bar {
    flex: 1;
    height: 6px;
    background: var(--panel);
    border-radius: 3px;
    overflow: hidden;
  }
  .fill {
    height: 100%;
    background: var(--accent);
    transition: width 0.2s;
  }
  .cards {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-top: 10px;
  }
  .card {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 10px 14px;
  }
  .head {
    color: var(--accent);
    font-weight: 600;
    margin-bottom: 6px;
  }
  .hit {
    display: block;
    width: 100%;
    text-align: left;
    background: none;
    border: 0;
    color: var(--text);
    font: inherit;
    padding: 3px 0;
    cursor: pointer;
    border-radius: 6px;
  }
  .hit:hover {
    background: var(--bg);
  }
  .context {
    color: var(--muted);
    font-size: 12px;
  }
  .line {
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 13px;
  }
  .empty,
  .meta {
    color: var(--muted);
    margin-top: 14px;
  }
  .err {
    color: #f87171;
  }
</style>
