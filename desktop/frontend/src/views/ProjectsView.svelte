<script lang="ts">
  // Projects: one agent's projects (the CLI's `stats --by project`) drilling
  // down into a project's per-model usage (`stats --by model`) and sessions.
  // The drill-down key is the exact project path — the CLI's --project is
  // only a substring filter, so the session list is narrowed client-side to
  // the project's own rows. Selection lives in the module store and survives
  // the round trip through the viewer.
  import { api, emptyFilter, onProgress } from '../lib/api';
  import type { ProjectRow, SessionRow, StatsRow } from '../lib/api';
  import { fmtBytes, fmtDate, fmtInt, idPrefix } from '../lib/format';
  import { go } from '../lib/stores.svelte';
  import { openSession } from '../lib/viewer.svelte';
  import { closeProject, openProject, projectsNav } from '../lib/projects.svelte';
  import Notes from '../components/Notes.svelte';
  import VirtualList from '../components/VirtualList.svelte';
  import Loader from '../components/Loader.svelte';

  let agents = $state<string[]>([]);
  let list = $state<ProjectRow[] | null>(null);
  let modelRows = $state<StatsRow[] | null>(null);
  let sessions = $state<SessionRow[] | null>(null);
  let notes = $state<string[]>([]);
  let error = $state('');
  let progress = $state<{ scanned: number } | null>(null);
  let loading = $state(true);

  api.diagnostics().then((d) => (agents = d.agents.map((a) => a.name)));
  const stopProgress = onProgress((e) => {
    if (e.op === 'projects') progress = { scanned: e.scanned };
  });

  let seq = 0;

  async function loadList(ag: string) {
    const mine = ++seq;
    loading = true;
    progress = null;
    try {
      const o = await api.projects(ag);
      if (mine !== seq) return; // a newer selection superseded this load
      list = o.rows;
      notes = o.notes ?? [];
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      list = null;
      notes = [];
      error = String(e);
    } finally {
      if (mine === seq) {
        progress = null;
        loading = false;
      }
    }
  }

  async function loadDetail(ag: string, proj: string) {
    const mine = ++seq;
    loading = true;
    progress = null;
    try {
      const [stats, sess] = await Promise.all([
        api.projectStats(ag, proj),
        api.sessions({ ...emptyFilter(), agent: ag, project: proj }),
      ]);
      if (mine !== seq) return;
      modelRows = stats.rows;
      sessions = (sess.sessions ?? []).filter((s) => s.project === proj);
      notes = stats.notes ?? [];
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      modelRows = null;
      sessions = null;
      notes = [];
      error = String(e);
    } finally {
      if (mine === seq) {
        progress = null;
        loading = false;
      }
    }
  }

  $effect(() => {
    const ag = projectsNav.agent;
    const proj = projectsNav.project;
    if (proj === '') {
      if (ag !== '') void loadList(ag);
    } else {
      void loadDetail(ag, proj);
    }
  });

  // first entry via the nav (no agent chosen yet): default to the first
  $effect(() => {
    if (projectsNav.agent === '' && agents.length > 0) {
      projectsNav.agent = agents[0] ?? '';
    }
  });

  const hasCost = $derived(
    (list ?? []).some((r) => r.cost_usd !== null) || (modelRows ?? []).some((r) => r.cost_usd !== null),
  );
  const maxModelTotal = $derived(Math.max(1, ...(modelRows ?? []).map((r) => r.tokens.total)));

  function cancel() {
    void api.cancel();
  }
</script>

{#if projectsNav.project === ''}
  <h1>Projects</h1>
  <div class="toolbar">
    <label>
      agent
      <select bind:value={projectsNav.agent}>
        {#each agents as name}
          <option value={name}>{name}</option>
        {/each}
      </select>
    </label>
  </div>
  <Notes notes={!loading ? notes : []} />
  {#if progress}
    <div class="progress">
      <span>scanning… {progress.scanned} sessions</span>
      <button class="btn" onclick={cancel}>Cancel</button>
    </div>
  {/if}
  {#if !loading && error}<p class="err">{error}</p>{/if}
  {#if loading}
    {#if !progress}
      <Loader label="scanning projects…" />
    {/if}
  {:else if list}
    {#if list.length > 0}
      <div class="panel tablewrap fadein">
        <div class="head" class:withcost={hasCost}>
          <span>project</span>
          <span class="r">sessions</span>
          <span class="r">messages</span>
          <span class="r">input</span>
          <span class="r">output</span>
          <span class="r">total</span>
          {#if hasCost}<span class="r">cost usd</span>{/if}
        </div>
        <div class="plist">
          <VirtualList items={list} itemHeight={32}>
            {#snippet row(p)}
              <button class="prow" class:withcost={hasCost} onclick={() => openProject(p.project)} title={p.project}>
                <span class="key">{p.project || '-'}</span>
                <span class="r">{fmtInt(p.sessions)}</span>
                <span class="r">{fmtInt(p.messages)}</span>
                <span class="r">{fmtInt(p.tokens.input)}</span>
                <span class="r">{fmtInt(p.tokens.output)}</span>
                <span class="r strong">{fmtInt(p.tokens.total)}</span>
                {#if hasCost}<span class="r">{p.cost_usd === null ? '-' : p.cost_usd.toFixed(2)}</span>{/if}
              </button>
            {/snippet}
          </VirtualList>
        </div>
      </div>
      <p class="meta fadein">{fmtInt(list.length)} projects</p>
    {:else}
      <p class="meta">no sessions recorded for this agent</p>
    {/if}
  {/if}
{:else}
  <div class="crumb">
    <button class="btn" onclick={closeProject}>
      <svg
        viewBox="0 0 24 24"
        width="13"
        height="13"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="m15 18-6-6 6-6" />
      </svg>
      All projects
    </button>
  </div>
  <div class="top">
    <div>
      <h1 class="proj">{projectsNav.project || '-'}</h1>
      <p class="meta"><span class="chip">{projectsNav.agent}</span> &middot; token usage by model</p>
    </div>
  </div>
  <Notes notes={!loading ? notes : []} />
  {#if progress}
    <div class="progress">
      <span>scanning… {progress.scanned} sessions</span>
      <button class="btn" onclick={cancel}>Cancel</button>
    </div>
  {/if}
  {#if !loading && error}<p class="err">{error}</p>{/if}
  {#if loading}
    {#if !progress}
      <Loader label="loading project…" />
    {/if}
  {:else}
    {#if modelRows}
      {#if modelRows.length > 0}
        <div class="panel tablewrap fadein">
        <table>
          <thead>
            <tr>
              <th>model</th>
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
            {#each modelRows as row}
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
                  <span class="bar" style="width: {(row.tokens.total / maxModelTotal) * 100}%"></span>
                  <span class="num">{fmtInt(row.tokens.total)}</span>
                </td>
                {#if hasCost}
                  <td class="r">{row.cost_usd === null ? '-' : row.cost_usd.toFixed(2)}</td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
        </div>
      {:else}
        <p class="meta">no usage data for this project</p>
      {/if}
    {/if}
    {#if sessions && sessions.length > 0}
    <h2>Sessions</h2>
    <div class="panel tablewrap fadein">
      <div class="shead">
        <span>date</span>
        <span>title</span>
        <span class="r">messages</span>
        <span class="r">size</span>
      </div>
      <div class="slist">
        <VirtualList items={sessions} itemHeight={32}>
          {#snippet row(s)}
            <button
              class="srow"
              onclick={() => {
                openSession(s.id, undefined, 'projects');
                go('viewer');
              }}
              title={s.title || s.id}
            >
              <span class="muted">{fmtDate(s.started_at)}</span>
              <span class="title">{s.title || idPrefix(s.id)}</span>
              <span class="r">{fmtInt(s.messages)}</span>
              <span class="r muted">{fmtBytes(s.size_bytes)}</span>
            </button>
          {/snippet}
        </VirtualList>
      </div>
    </div>
    <p class="meta">{fmtInt(sessions.length)} sessions</p>
  {/if}
  {/if}
{/if}

<style>
  .toolbar {
    display: flex;
    gap: 16px;
    align-items: end;
    padding-bottom: 4px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.05em;
    text-transform: uppercase;
    color: var(--muted);
  }
  select {
    min-width: 170px;
  }
  .tablewrap {
    margin-top: 12px;
    padding: 4px 16px 8px;
  }
  .head,
  .prow {
    display: grid;
    grid-template-columns: minmax(160px, 1fr) 80px 90px 110px 110px 110px;
    gap: 10px;
    padding: 0 4px;
    align-items: center;
  }
  .head.withcost,
  .prow.withcost {
    grid-template-columns: minmax(160px, 1fr) 80px 90px 110px 110px 110px 90px;
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
  .plist {
    height: calc(100vh - 360px);
    min-height: 180px;
  }
  .prow {
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
  .prow:hover {
    background: var(--panel-hover);
  }
  .key {
    color: var(--text-2);
    font-family: var(--mono);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .r {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
    font-size: 12.5px;
  }
  .strong {
    color: var(--text);
    font-weight: 600;
  }
  .crumb {
    margin-bottom: 12px;
  }
  .top {
    display: flex;
    justify-content: space-between;
    align-items: start;
    gap: 12px;
  }
  .proj {
    font-family: var(--mono);
    font-size: 15px;
    overflow-wrap: anywhere;
  }
  td.key {
    color: var(--text);
    font-weight: 600;
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
  .shead,
  .srow {
    display: grid;
    grid-template-columns: 130px minmax(160px, 1fr) 90px 80px;
    gap: 10px;
    padding: 0 4px;
    align-items: center;
  }
  .shead {
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
  .slist {
    height: calc(100vh - 600px);
    min-height: 140px;
  }
  .srow {
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
  .srow:hover {
    background: var(--panel-hover);
  }
  .srow .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
  }
  .srow .muted {
    font-family: var(--mono);
    font-size: 11.5px;
  }
  .muted {
    color: var(--muted);
  }
</style>
