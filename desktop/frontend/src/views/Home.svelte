<script lang="ts">
  // Home: the bare-`pastlog` summary — KPI row, one card per agent, plus the
  // friendly empty state when nothing is detected (TASK.md §14, last
  // checkbox). Every number on screen is derived from the overview payload;
  // nothing is hardcoded.
  import { api, type AgentRow, type OverviewOutcome } from '../lib/api';
  import { fmtBytes, fmtInt } from '../lib/format';
  import { appMeta, go } from '../lib/stores.svelte';
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
  const totalSessions = $derived(detected.reduce((sum, a) => sum + a.sessions, 0));
  const totalBytes = $derived(detected.reduce((sum, a) => sum + a.bytes, 0));
  const leader = $derived([...detected].sort((a, b) => b.sessions - a.sessions)[0] ?? null);
  const detectedPct = $derived(
    data && data.agents.length > 0 ? Math.round((detected.length / data.agents.length) * 100) : 0,
  );

  function shareOf(a: AgentRow): number {
    return totalSessions > 0 ? Math.round((a.sessions / totalSessions) * 100) : 0;
  }
</script>

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
    <Notes notes={data.warnings} />

    <section class="hero fadein">
      <div class="facts"><span class="dot"></span>{detected.length}/{data.agents.length} sources ready · {fmtBytes(totalBytes)} indexed</div>
      <h1>Pastlog <span>index</span></h1>
      <p class="lede">Everything your AI coding agents wrote on this machine — searchable across every project.</p>
      <div class="home-line">~ {data.home}</div>
    </section>

    <section class="kpis fadein">
      <div class="kpi">
        <div class="kpi-top"><span class="kpi-label">SOURCES</span><span class="ping" aria-hidden="true"></span></div>
        <div class="kpi-mid">
          <div class="kpi-val">{detected.length}<small>/ {data.agents.length} ready</small></div>
          <span class="kpi-chip">AUTO-DETECT</span>
        </div>
        <div class="kpi-foot"><span>storage roots</span><b class="mint">{detectedPct}% active</b></div>
      </div>
      <div class="kpi">
        <div class="kpi-top"><span class="kpi-label">SESSIONS</span></div>
        <div class="kpi-mid">
          <div class="kpi-val">{fmtInt(totalSessions)}<small>logs</small></div>
        </div>
        <div class="kpi-foot"><span>top source</span><b class="mint">{leader?.name ?? '—'}</b></div>
      </div>
      <div class="kpi">
        <div class="kpi-top"><span class="kpi-label">FOOTPRINT</span><span class="kpi-chip">LOCAL</span></div>
        <div class="kpi-mid">
          <div class="kpi-val">{fmtBytes(totalBytes)}<small>on disk</small></div>
        </div>
        <div class="kpi-foot"><span>writes to sources</span><b>none · read-only</b></div>
      </div>
      <div class="kpi">
        <div class="kpi-top"><span class="kpi-label">EDITION</span><span class="kpi-chip">DESKTOP</span></div>
        <div class="kpi-mid">
          <div class="kpi-val">v{appMeta.version || '—'}</div>
        </div>
        <div class="kpi-foot"><span>network deps</span><b>none · air-gapped</b></div>
      </div>
    </section>

    <section class="agents fadein">
      <header class="agents-head">
        <div>
          <div class="kicker">// 01 · SOURCES</div>
          <div class="title-row">
            <h2>Connected agents</h2>
            <span class="count-chip">{data.agents.length} sources registered</span>
          </div>
        </div>
        <span class="head-note">auto-detected from default storage paths</span>
      </header>

      <div class="grid">
        {#each data.agents as agent}
          <!-- detected agents drill into their project list; the rest stay inert -->
          <button
            class="agent"
            class:off={!agent.detected}
            disabled={!agent.detected}
            title={agent.detected ? `show ${agent.name} projects` : 'not detected'}
            onclick={() => {
              openAgentProjects(agent.name, 'home');
              go('projects');
            }}
          >
            <div class="agent-top">
              <div class="agent-id">
                <span class="dot" class:off={!agent.detected}></span>
                <span class="agent-name">{agent.name}</span>
              </div>
              {#if agent.detected}
                <span class="status live">ACTIVE</span>
              {:else}
                <span class="status warn">NOT FOUND</span>
              {/if}
            </div>
            <p class="agent-path">{agent.path ?? 'no storage location'}</p>
            {#if agent.detected}
              <div class="agent-stats">
                <div><span>SESSIONS</span><b>{fmtInt(agent.sessions)}</b></div>
                <div><span>DISK USAGE</span><b>{fmtBytes(agent.bytes)}</b></div>
              </div>
              <div class="density">
                <div class="density-label"><span>index density</span><span>{shareOf(agent)}% corpus</span></div>
                <div class="bar"><span style="width: {Math.max(shareOf(agent), 3)}%"></span></div>
              </div>
              <div class="agent-foot"><span>mode=ro</span><span class="inspect">Inspect archive →</span></div>
            {:else}
              <div class="agent-stats dim">
                <div><span>SESSIONS</span><b>{fmtInt(agent.sessions)}</b></div>
                <div><span>STATUS</span><b class="warn-text">not indexed</b></div>
              </div>
              <p class="agent-note">Install the agent or set a home override to index its history.</p>
              <div class="agent-foot"><span>skipped · read-only</span></div>
            {/if}
          </button>
        {/each}
        <!-- completes the grid; the home override in Settings is the real
             "custom storage" affordance -->
        <div class="agent slot">
          <div class="slot-mark" aria-hidden="true">+</div>
          <div>
            <div class="slot-title">Custom storage home</div>
            <p class="agent-note">Point pastlog at a custom JSONL or SQLite session store with the home override.</p>
          </div>
          <div class="agent-foot">
            <span>read-only · zero config</span>
            <button class="inspect-btn" onclick={() => go('settings')}>Open settings →</button>
          </div>
        </div>
      </div>
    </section>

    <section class="cta fadein">
      <div>
        <div class="kicker">// READY TO QUERY</div>
        <h3>Search {fmtInt(totalSessions)} sessions across {detected.length} agent trees</h3>
        <p>Instant regex or substring streaming — straight from local storage, no index to build.</p>
      </div>
      <div class="cta-actions">
        <button class="btn primary" onclick={() => go('search')}>Launch search</button>
        <button class="btn" onclick={() => go('stats')}>Full stats</button>
      </div>
    </section>
  {/if}
{/if}

<style>
  .hero {
    margin-bottom: 26px;
  }
  .facts {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 14px;
    padding: 3px 11px;
    border-radius: 999px;
    background: var(--bg-deep);
    color: var(--muted);
    font: 500 10.5px var(--mono);
  }
  .hero h1 {
    margin: 0 0 7px;
    font-size: 31px;
    font-weight: 700;
    letter-spacing: -0.03em;
    line-height: 1.1;
  }
  .hero h1 span {
    color: var(--accent);
  }
  .lede {
    margin: 0;
    max-width: 560px;
    color: var(--text-2);
    font-size: 14px;
    line-height: 1.6;
  }
  .home-line {
    margin-top: 12px;
    color: var(--muted);
    font: 11px var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 560px;
  }

  .kpis {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 13px;
    margin-bottom: 40px;
  }
  .kpi {
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    gap: 14px;
    padding: 15px 16px 13px;
    background: var(--panel);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-l);
    transition: background var(--speed) ease, border-color var(--speed) ease;
  }
  .kpi:hover {
    background: var(--panel-hover);
    border-color: var(--border);
  }
  .kpi-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .kpi-label {
    color: var(--muted);
    font: 600 9.5px var(--mono);
    letter-spacing: 0.12em;
  }
  .kpi-chip {
    padding: 2px 7px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--muted);
    font: 600 8.5px var(--mono);
    letter-spacing: 0.1em;
  }
  .kpi-mid {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .kpi-val {
    display: flex;
    align-items: baseline;
    gap: 7px;
    font-size: 27px;
    font-weight: 700;
    letter-spacing: -0.02em;
    font-variant-numeric: tabular-nums;
    line-height: 1;
  }
  .kpi-val small {
    font-size: 11.5px;
    font-weight: 500;
    color: var(--muted);
    letter-spacing: 0;
  }
  .kpi-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-top: 10px;
    border-top: 1px solid var(--border-subtle);
    color: var(--muted);
    font: 10.5px var(--mono);
  }
  .kpi-foot b {
    font-weight: 500;
    color: var(--text-2);
  }
  .kpi-foot b.mint {
    color: var(--accent);
  }
  .ping {
    position: relative;
    width: 8px;
    height: 8px;
    flex-shrink: 0;
  }
  .ping::before,
  .ping::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: 50%;
    background: var(--accent-vivid);
  }
  .ping::before {
    opacity: 0.6;
    animation: ping 2.4s cubic-bezier(0, 0, 0.2, 1) infinite;
  }
  @keyframes ping {
    75%,
    100% {
      transform: scale(2.4);
      opacity: 0;
    }
  }

  .agents-head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 14px;
  }
  .kicker {
    margin-bottom: 6px;
    color: var(--accent);
    font: 600 11px var(--mono);
    letter-spacing: 0.04em;
  }
  .title-row {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .agents-head h2 {
    margin: 0;
    font-size: 19px;
    font-weight: 700;
    letter-spacing: -0.02em;
    text-transform: none;
    color: var(--text);
  }
  .count-chip {
    padding: 2px 8px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--text-2);
    font: 500 10px var(--mono);
    white-space: nowrap;
  }
  .head-note {
    color: var(--faint);
    font: 10.5px var(--mono);
    white-space: nowrap;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 13px;
  }
  .agent {
    display: flex;
    flex-direction: column;
    gap: 11px;
    padding: 16px 17px 13px;
    background: var(--panel);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-l);
    text-align: left;
    font: inherit;
    color: var(--text);
    transition: background var(--speed) ease, border-color var(--speed) ease;
  }
  .agent:not(:disabled) {
    cursor: pointer;
  }
  .agent:not(:disabled):hover {
    background: var(--panel-hover);
    border-color: var(--border);
  }
  .agent.off {
    opacity: 0.85;
  }
  .agent-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  .agent-id {
    display: flex;
    align-items: center;
    gap: 9px;
    min-width: 0;
  }
  .agent-name {
    font-size: 14.5px;
    font-weight: 650;
    letter-spacing: -0.01em;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    padding: 2px 8px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    font: 600 9px var(--mono);
    letter-spacing: 0.1em;
    white-space: nowrap;
  }
  .status.live {
    color: var(--accent);
    background: var(--accent-soft);
  }
  .status.warn {
    color: var(--warn);
    background: var(--warn-soft);
  }
  .agent-path {
    margin: 0;
    color: var(--muted);
    font: 10.5px var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .agent-stats {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
    padding: 10px 12px;
    border-radius: var(--radius-s);
    background: var(--inset);
    border: 1px solid var(--border-subtle);
  }
  .agent-stats span {
    display: block;
    color: var(--faint);
    font: 600 8.5px var(--mono);
    letter-spacing: 0.12em;
  }
  .agent-stats b {
    font-size: 15.5px;
    font-weight: 650;
    letter-spacing: -0.01em;
    font-variant-numeric: tabular-nums;
  }
  .agent-stats.dim b {
    color: var(--muted);
    font-weight: 500;
    font-size: 13.5px;
    line-height: 1.6;
  }
  .agent-stats .warn-text {
    color: var(--warn);
  }
  .density {
    display: grid;
    gap: 5px;
  }
  .density-label {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font: 10px var(--mono);
  }
  .density-label span:last-child {
    color: var(--text-2);
  }
  .bar {
    height: 5px;
    border-radius: 3px;
    background: var(--inset);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: 3px;
    background: var(--accent-vivid);
    box-shadow: 0 0 8px var(--accent-glow);
  }
  .agent-note {
    margin: 0;
    color: var(--muted);
    font-size: 12px;
    line-height: 1.55;
  }
  .agent-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: auto;
    padding-top: 10px;
    border-top: 1px solid var(--border-subtle);
    color: var(--faint);
    font: 10px var(--mono);
  }
  .agent:not(:disabled):hover .inspect {
    color: var(--accent);
  }
  .inspect {
    color: var(--text-2);
    transition: color var(--speed) ease;
  }

  .slot {
    justify-content: center;
    gap: 13px;
  }
  .slot-mark {
    display: grid;
    place-items: center;
    width: 34px;
    height: 34px;
    border-radius: var(--radius-s);
    background: var(--inset);
    border: 1px solid var(--border-subtle);
    color: var(--accent);
    font: 600 16px var(--mono);
  }
  .slot-title {
    margin-bottom: 4px;
    font-size: 14px;
    font-weight: 650;
    letter-spacing: -0.01em;
  }
  .inspect-btn {
    border: 0;
    padding: 0;
    background: none;
    color: var(--text-2);
    font: 10px var(--mono);
    transition: color var(--speed) ease;
  }
  .inspect-btn:hover {
    color: var(--accent);
  }

  .cta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    margin-top: 40px;
    padding: 22px 24px;
    background: var(--panel);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-l);
  }
  .cta h3 {
    margin: 4px 0 6px;
    font-size: 17px;
    font-weight: 700;
    letter-spacing: -0.015em;
  }
  .cta p {
    margin: 0;
    color: var(--muted);
    font-size: 12.5px;
  }
  .cta-actions {
    display: flex;
    gap: 10px;
    flex-shrink: 0;
  }

  @media (max-width: 1080px) {
    .kpis {
      grid-template-columns: repeat(2, 1fr);
    }
  }
  @media (max-width: 720px) {
    .kpis {
      grid-template-columns: 1fr;
    }
    .cta {
      flex-direction: column;
      align-items: flex-start;
    }
    .head-note {
      display: none;
    }
  }
</style>
