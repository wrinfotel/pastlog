<script lang="ts">
  // Adapter conditions (locked storage, skipped records) rendered once per
  // view load — the GUI's stderr line. Null-safe: the Go side may marshal
  // empty lists as null.
  let { notes = [] }: { notes?: string[] | null } = $props();

  const list = $derived(notes ?? []);
</script>

{#if list.length > 0}
  <div class="notes" role="status">
    {#each list as note}
      <div>{note}</div>
    {/each}
  </div>
{/if}

<style>
  .notes {
    color: var(--muted);
    font-size: 12px;
    padding: 4px 0;
  }
</style>
