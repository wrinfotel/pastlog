<script lang="ts">
  // Adapter conditions (locked storage, skipped records) rendered once per
  // view load — the GUI's stderr line. Null-safe: the Go side may marshal
  // empty lists as null. Dismissal is per-mount only; notes return on the
  // next view load.
  import { go } from '../lib/stores.svelte';

  let { notes = [] }: { notes?: string[] | null } = $props();

  const list = $derived(notes ?? []);
  let dismissed = $state(false);
</script>

{#if list.length > 0 && !dismissed}
  <div class="notes" role="status">
    <div class="tile" aria-hidden="true">
      <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
        <line x1="12" y1="9" x2="12" y2="13" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
    </div>
    <div class="body">
      <div class="head">
        <b>adapter notes</b>
        <i>/</i>
        <span>{list.length} {list.length === 1 ? 'notice' : 'notices'}</span>
      </div>
      {#each list as note}
        <div>{note}</div>
      {/each}
    </div>
    <div class="actions">
      <button onclick={() => go('diagnostics')}>view logs</button>
      <button class="quiet" onclick={() => (dismissed = true)}>dismiss</button>
    </div>
  </div>
{/if}

<style>
  .notes {
    display: flex;
    align-items: flex-start;
    gap: 13px;
    padding: 13px 15px;
    margin-bottom: 26px;
    background: var(--panel);
    border: 1px solid color-mix(in srgb, var(--warn) 22%, transparent);
    border-radius: var(--radius-l);
  }
  .tile {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    flex-shrink: 0;
    border-radius: var(--radius-s);
    background: var(--warn-soft);
    color: var(--warn);
  }
  .body {
    flex: 1;
    min-width: 0;
    display: grid;
    gap: 2px;
    color: var(--text-2);
    font-size: 12.5px;
    line-height: 1.5;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 2px;
  }
  .head b {
    color: var(--warn);
    font: 600 10px var(--mono);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  .head i {
    color: var(--faint);
    font-style: normal;
  }
  .head span {
    color: var(--muted);
    font: 500 10px var(--mono);
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
  .actions button {
    padding: 4px 10px;
    border: 0;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--text-2);
    font: 500 10.5px var(--mono);
    transition:
      color var(--speed) ease,
      background var(--speed) ease;
  }
  .actions button:hover {
    color: var(--text);
    background: var(--inset);
  }
  .actions button.quiet {
    color: var(--muted);
  }
</style>
