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

  let agents = $state<string[]>([]);
  let list = $state<ProjectRow[] | null>(null);
  let modelRows = $state<StatsRow[] | null>(null);
  let sessions = $state<SessionRow[] | null>(null);
  let notes = $state<string[]>([]);
  let error = $state('');
  let progress = $state<{ scanned: number } | null>(null);

  api.diagnostics().then((d) => (agents = d.agents.map((a) => a.name)));
  const stopProgress = onProgress((e) => {
    if (e.op === 'projects') progress = { scanned: e.scanned };
  });

  let seq = 0;

  async function loadList(ag: string) {
    const mine = ++seq;
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
      if (mine === seq) progress = null;
    }
  }

  async function loadDetail(ag: string, proj: string) {
    const mine = ++seq;
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
      if (mine === seq) progress = null;
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
  <Notes {notes} />
  {#if progress}
    <div class="progress">
      <span>scanning… {progress.scanned} sessions</span>
      <button onclick={cancel}>Cancel</button>
    </div>
  {/if}
  {#if error}<p class="err">{error}</p>{/if}
  {#if list}
    {#if list.length > 0}
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
        <VirtualList items={list} itemHeight={30}>
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
      <p class="meta">{fmtInt(list.length)} projects</p>
    {:else if !progress}
      <p class="meta">no sessions recorded for this agent</p>
    {/if}
  {:else if !error && !progress}
    <p class="meta">loading…</p>
  {/if}
{:else}
  <div class="top">
    <div>
      <h1 class="proj">{projectsNav.project || '-'}</h1>
      <p class="meta">{projectsNav.agent} · token usage by model</p>
    </div>
    <button class="btn" onclick={closeProject}>← All projects</button>
  </div>
  <Notes {notes} />
  {#if progress}
    <div class="progress">
      <span>scanning… {progress.scanned} sessions</span>
      <button onclick={cancel}>Cancel</button>
    </div>
  {/if}
  {#if error}<p class="err">{error}</p>{/if}
  {#if modelRows}
    {#if modelRows.length > 0}
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
    {:else if !progress}
      <p class="meta">no usage data for this project</p>
    {/if}
  {/if}
  {#if sessions && sessions.length > 0}
    <h2>Sessions</h2>
    <div class="shead">
      <span>date</span>
      <span>title</span>
      <span class="r">messages</span>
      <span class="r">size</span>
    </div>
    <div class="slist">
      <VirtualList items={sessions} itemHeight={30}>
        {#snippet row(s)}
          <button
            class="srow"
            onclick={() => {
              openSession(s.id);
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
    <p class="meta">{fmtInt(sessions.length)} sessions</p>
  {/if}
{/if}

<style>
  .toolbar {
    display: flex;
    gap: 14px;
    align-items: end;
    padding-bottom: 6px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
    color: var(--muted);
  }
  select {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 5px 8px;
    min-width: 160px;
  }
  .head,
  .prow {
    display: grid;
    grid-template-columns: minmax(160px, 1fr) 80px 90px 110px 110px 110px;
    gap: 8px;
    padding: 0 4px;
    align-items: center;
  }
  .head.withcost,
  .prow.withcost {
    grid-template-columns: minmax(160px, 1fr) 80px 90px 110px 110px 110px 90px;
  }
  .head {
    color: var(--muted);
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .plist {
    height: calc(100vh - 340px);
    min-height: 180px;
  }
  .prow {
    width: 100%;
    background: none;
    border: 0;
    border-bottom: 1px solid var(--border);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .prow:hover {
    background: var(--panel);
  }
  .key {
    color: var(--accent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .r {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--muted);
  }
  .strong {
    color: var(--text);
  }
  .top {
    display: flex;
    justify-content: space-between;
    align-items: start;
    gap: 12px;
  }
  .proj {
    overflow-wrap: anywhere;
  }
  .btn {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font: inherit;
    flex-shrink: 0;
  }
  .btn:hover {
    border-color: var(--accent);
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
  td.r {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  td.key {
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
  h2 {
    margin-top: 22px;
    font-size: 16px;
  }
  .shead,
  .srow {
    display: grid;
    grid-template-columns: 130px minmax(160px, 1fr) 90px 80px;
    gap: 8px;
    padding: 0 4px;
    align-items: center;
  }
  .shead {
    color: var(--muted);
    font-size: 12px;
    border-bottom: 1px solid var(--border);
  }
  .slist {
    height: calc(100vh - 560px);
    min-height: 140px;
  }
  .srow {
    width: 100%;
    background: none;
    border: 0;
    border-bottom: 1px solid var(--border);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .srow:hover {
    background: var(--panel);
  }
  .srow .title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .muted {
    color: var(--muted);
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
  .meta {
    color: var(--muted);
    font-size: 12px;
  }
  .err {
    color: #f87171;
  }
</style>
