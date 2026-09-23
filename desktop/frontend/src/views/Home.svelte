<script lang="ts">
  // Home: the bare-`pastlog` summary — one card per agent, plus the friendly
  // empty state when nothing is detected (TASK.md §14, last checkbox).
  import { api, type OverviewOutcome } from '../lib/api';
  import { fmtBytes, fmtInt } from '../lib/format';
  import { go } from '../lib/stores.svelte';
  import { openAgentProjects } from '../lib/projects.svelte';
  import Notes from '../components/Notes.svelte';
  import Loader from '../components/Loader.svelte';

  let data = $state<OverviewOutcome | null>(null);
  let error = $state('');
  let loading = $state(true);

  $effect(() => {
    loading = true;
    api
      .overview()
      .then((o) => {
        data = o;
        error = '';
      })
      .catch((e) => (error = String(e)))
      .finally(() => (loading = false));
  });

  const detected = $derived(data?.agents.filter((a) => a.detected) ?? []);
</script>

<header class="page-head">
  <div class="masthead">
    <div class="issue">Issue 01 <span>·</span> Local archive</div>
    <h1>Pastlog <em>index</em></h1>
    <p class="lede">A considered view of the sessions stored on this machine.</p>
  </div>
  {#if data && !data.error}
    <div class="location">
      <span class="dot"></span>
      <div><strong>Local workspace</strong><small>{data.home}</small></div>
    </div>
  {/if}
</header>
{#if loading}
  <Loader label="loading overview…" />
{:else if error}
  <p class="err">cannot load the overview: {error}</p>
{:else if data}
  {#if data.error}
    <div class="empty">
      <p>{data.error}</p>
      <button class="btn primary" onclick={() => go('settings')}>Open settings</button>
    </div>
  {:else if detected.length === 0}
    <div class="empty">
      <p>nothing found — install an agent or set the home override</p>
      <button class="btn primary" onclick={() => go('settings')}>Open settings</button>
    </div>
  {:else}
    <div class="summary-row fadein">
      <div class="summary"><span>Sources</span><strong>{detected.length}</strong><small>connected agents</small></div>
      <div class="summary"><span>Sessions</span><strong>{fmtInt(detected.reduce((sum, agent) => sum + agent.sessions, 0))}</strong><small>indexed entries</small></div>
      <div class="summary"><span>Footprint</span><strong>{fmtBytes(detected.reduce((sum, agent) => sum + agent.bytes, 0))}</strong><small>local storage</small></div>
      <div class="summary edition"><span>Edition</span><strong>2026</strong><small>read-only archive</small></div>
    </div>
    <div class="section-head"><div><span class="section-kicker">01 / Sources</span><h2>Connected agents</h2><p>Choose a source to browse its projects and sessions.</p></div><span class="section-count">{data.agents.length} sources</span></div>
    <div class="cards fadein">
      {#each data.agents as agent, index}
        <!-- detected agents drill into their project list; the rest stay inert -->
        <button
          class="card"
          class:off={!agent.detected}
          disabled={!agent.detected}
          title={agent.detected ? `show ${agent.name} projects` : 'not detected'}
          onclick={() => {
            openAgentProjects(agent.name);
            go('projects');
          }}
        >
          <div class="row1">
            <span class="index">{String(index + 1).padStart(2, '0')}</span>
            <span class="name">{agent.name}</span>
            <span class="dot" class:off={!agent.detected}></span>
          </div>
          {#if agent.detected}
            <div class="count">
              {fmtInt(agent.sessions)}<span class="unit">sessions</span>
            </div>
            <div class="meta">{fmtBytes(agent.bytes)}</div>
          {:else}
            <div class="meta absent">not found</div>
          {/if}
          <svg
            class="chev"
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="m9 18 6-6-6-6" />
          </svg>
        </button>
      {/each}
    </div>
    <Notes notes={data.warnings} />
  {/if}
{/if}

<style>
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(215px, 1fr));
    gap: 12px;
  }
  .page-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 28px;
    padding-bottom: 22px;
    border-bottom: 1px solid var(--border);
  }
  .masthead { position: relative; }
  .issue, .section-kicker {
    color: var(--accent);
    font: 10px var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .issue { margin-bottom: 9px; }
  .issue span { color: var(--faint); padding: 0 4px; }
  h1 {
    margin-bottom: 5px;
    font-family: var(--display);
    font-size: 31px;
    font-weight: 600;
    letter-spacing: -0.03em;
  }
  h1 em { color: var(--accent); font-style: normal; }
  .lede, .section-head p {
    margin: 0;
    color: var(--muted);
    font-size: 13px;
  }
  .location {
    display: flex;
    align-items: center;
    gap: 9px;
    min-width: 220px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-s);
    background: var(--panel);
  }
  .location div { display: grid; gap: 1px; }
  .location strong { font-size: 12px; font-weight: 600; }
  .location small { color: var(--muted); font: 10px var(--mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 180px; }
  .summary-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 0; margin-bottom: 34px; border-top: 1px solid var(--border); border-bottom: 1px solid var(--border); }
  .summary { padding: 12px 16px 13px 0; margin-right: 16px; display: grid; gap: 3px; border-right: 1px solid var(--border); }
  .summary:last-child { border-right: 0; }
  .summary span, .summary small { color: var(--muted); font-size: 10.5px; }
  .summary strong { font-size: 22px; font-weight: 650; letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
  .summary.edition strong { color: var(--accent); font-family: var(--display); font-weight: 500; }
  .section-head { display: flex; align-items: end; justify-content: space-between; gap: 16px; margin-bottom: 12px; }
  .section-head h2 { margin: 3px 0; color: var(--text); font-family: var(--display); font-size: 18px; font-weight: 500; letter-spacing: -0.01em; text-transform: none; }
  .section-head p { margin: 0; }
  .section-count { color: var(--muted); font: 10px var(--mono); }
  .card {
    position: relative;
    background: transparent;
    border: 0;
    border-top: 1px solid var(--border);
    border-radius: 0;
    padding: 15px 12px 15px 0;
    min-height: 102px;
    text-align: left;
    font: inherit;
    color: var(--text);
    display: flex;
    flex-direction: column;
    transition:
      transform var(--speed) ease,
      border-color var(--speed) ease,
      box-shadow var(--speed) ease;
  }
  .card:not(:disabled) {
    cursor: pointer;
  }
  .card:not(:disabled):hover {
    transform: translateX(5px);
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .card.off {
    opacity: 0.62;
  }
  .row1 {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .index { color: var(--accent); font: 10px var(--mono); width: 22px; }
  .name {
    font-weight: 650;
    font-size: 14px;
    letter-spacing: -0.01em;
  }
  .count {
    margin-top: 12px;
    font-size: 24px;
    font-weight: 650;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
    display: flex;
    align-items: baseline;
    gap: 7px;
  }
  .unit {
    font-size: 11.5px;
    font-weight: 500;
    color: var(--muted);
    letter-spacing: 0;
  }
  .absent {
    margin-top: 24px;
    color: var(--text-2);
    font-size: 13px;
  }
  .chev {
    position: absolute;
    top: 14px;
    right: 14px;
    color: var(--accent);
    opacity: 0;
    transform: translateX(-4px);
    transition:
      opacity var(--speed) ease,
      transform var(--speed) ease;
  }
  .card:not(:disabled):hover .chev {
    opacity: 1;
    transform: none;
  }
</style>
