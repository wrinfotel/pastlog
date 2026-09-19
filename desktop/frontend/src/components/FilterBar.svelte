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
    <input type="text" bind:value={filters.project} placeholder="path substring" />
  </label>
  <label>
    since
    <input type="text" bind:value={filters.since} placeholder="7d / 2w / 2026-01-01" />
  </label>
  <label>
    until
    <input type="text" bind:value={filters.until} placeholder="7d / 2w / 2026-01-01" />
  </label>
  <label>
    limit
    <input class="limit" type="number" min="0" bind:value={filters.limit} />
  </label>
</div>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: end;
    padding: 10px 0;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
    color: var(--muted);
  }
  select,
  input {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 5px 8px;
    min-width: 130px;
  }
  .limit {
    min-width: 80px;
  }
</style>
