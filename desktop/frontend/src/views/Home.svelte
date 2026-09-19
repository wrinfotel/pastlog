<script lang="ts">
  // Home: the bare-`pastlog` summary — one card per agent, plus the friendly
  // empty state when nothing is detected (TASK.md §14, last checkbox).
  import { api, type OverviewOutcome } from '../lib/api';
  import { fmtBytes, fmtInt } from '../lib/format';
  import { go } from '../lib/stores.svelte';
  import { openAgentProjects } from '../lib/projects.svelte';
  import Notes from '../components/Notes.svelte';

  let data = $state<OverviewOutcome | null>(null);
  let error = $state('');

  $effect(() => {
    api
      .overview()
      .then((o) => (data = o))
      .catch((e) => (error = String(e)));
  });

  const detected = $derived(data?.agents.filter((a) => a.detected) ?? []);
</script>

<h1>Home</h1>
{#if error}
  <p class="err">cannot load the overview: {error}</p>
{:else if data}
  {#if data.error}
    <div class="empty">
      <p>{data.error}</p>
      <button onclick={() => go('settings')}>Open settings</button>
    </div>
  {:else if detected.length === 0}
    <div class="empty">
      <p>nothing found — install an agent or set the home override</p>
      <button onclick={() => go('settings')}>Open settings</button>
    </div>
  {:else}
    <div class="cards">
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
          <div class="name">{agent.name}</div>
          {#if agent.detected}
            <div class="num">{fmtInt(agent.sessions)} sessions</div>
            <div class="meta">{fmtBytes(agent.bytes)}</div>
          {:else}
            <div class="meta">not found</div>
          {/if}
        </button>
      {/each}
    </div>
    <Notes notes={data.warnings} />
  {/if}
{/if}

<style>
  .cards {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-top: 12px;
  }
  .card {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 14px 18px;
    min-width: 190px;
    text-align: left;
    font: inherit;
    color: var(--text);
  }
  .card:not(:disabled) {
    cursor: pointer;
  }
  .card:not(:disabled):hover {
    border-color: var(--accent);
  }
  .card.off {
    opacity: 0.55;
  }
  .name {
    font-weight: 600;
    color: var(--accent);
  }
  .num {
    font-size: 20px;
    margin-top: 4px;
  }
  .meta {
    color: var(--muted);
    font-size: 12px;
  }
  .empty {
    margin-top: 24px;
    padding: 28px;
    border: 1px dashed var(--border);
    border-radius: 10px;
    text-align: center;
    color: var(--muted);
  }
  button {
    background: var(--accent);
    color: #08131a;
    border: 0;
    border-radius: 6px;
    padding: 7px 14px;
    cursor: pointer;
  }
  .err {
    color: #f87171;
  }
</style>
