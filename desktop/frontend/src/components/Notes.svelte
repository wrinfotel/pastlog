<script lang="ts">
  // Adapter conditions (locked storage, skipped records) rendered once per
  // view load — the GUI's stderr line. Null-safe: the Go side may marshal
  // empty lists as null.
  let { notes = [] }: { notes?: string[] | null } = $props();

  const list = $derived(notes ?? []);
</script>

{#if list.length > 0}
  <div class="notes" role="status">
    <svg
      class="icon"
      viewBox="0 0 24 24"
      width="13"
      height="13"
      fill="none"
      stroke="currentColor"
      stroke-width="1.8"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
      <line x1="12" y1="9" x2="12" y2="13" />
      <line x1="12" y1="17" x2="12.01" y2="17" />
    </svg>
    <div class="list">
      {#each list as note}
        <div>{note}</div>
      {/each}
    </div>
  </div>
{/if}

<style>
  .notes {
    display: flex;
    gap: 10px;
    padding: 8px 12px;
    margin: 10px 0;
    background: var(--warn-soft);
    border: 1px solid color-mix(in srgb, var(--warn) 25%, transparent);
    border-left: 2px solid var(--warn);
    border-radius: var(--radius);
    color: var(--text-2);
    font-size: 12px;
  }
  .icon {
    flex-shrink: 0;
    margin-top: 2px;
    color: var(--warn);
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
</style>
