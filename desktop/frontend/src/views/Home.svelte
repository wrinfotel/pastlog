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

<h1>Home</h1>
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
    <div class="cards fadein">
      {#each data.agents as agent}
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
    gap: 14px;
  }
  .card {
    position: relative;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: var(--radius-l);
    box-shadow:
      var(--hairline),
      var(--shadow-1);
    padding: 16px 18px 14px;
    min-height: 118px;
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
    transform: translateY(-2px);
    border-color: var(--border-strong);
    box-shadow:
      var(--hairline),
      var(--shadow-2),
      0 0 0 1px var(--accent-soft);
  }
  .card.off {
    opacity: 0.62;
  }
  .row1 {
    display: flex;
    align-items: center;
    gap: 8px;
  }
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
