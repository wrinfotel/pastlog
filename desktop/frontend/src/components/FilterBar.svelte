<script lang="ts">
  // The shared filter bar: agent / project / since / until / limit — the CLI
  // filter set 1:1 (TASK-DESKTOP.md §4.2). Owned by the parent view.
  import type { FilterOptions } from '../lib/api';

  let { agents = [], filters = $bindable() }: { agents?: string[]; filters: FilterOptions } = $props();
</script>

<div class="bar">
  <label>
    agent
    <select bind:value={filters.agent}>
      <option value="">all</option>
      {#each agents as name}
        <option value={name}>{name}</option>
      {/each}
    </select>
  </label>
  <label>
    project
    <input class="project" type="text" bind:value={filters.project} placeholder="path substring" />
  </label>
  <label>
    since
    <input class="mono" type="text" bind:value={filters.since} placeholder="7d / 2w / 2026-01-01" />
  </label>
  <label>
    until
    <input class="mono" type="text" bind:value={filters.until} placeholder="7d / 2w / 2026-01-01" />
  </label>
  <label>
    limit
    <input class="limit mono" type="number" min="0" bind:value={filters.limit} />
  </label>
</div>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    gap: 10px 14px;
    align-items: end;
    padding: 10px 14px;
    margin: 4px 0 8px;
    background: var(--bg-deep);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius);
  }
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
    min-width: 130px;
    padding-right: 26px;
  }
  input.project {
    min-width: 230px;
  }
  .mono {
    font-family: var(--mono);
    font-size: 12px;
  }
  .limit {
    min-width: 76px;
  }
</style>
