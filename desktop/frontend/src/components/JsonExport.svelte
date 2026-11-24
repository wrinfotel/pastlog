<script lang="ts">
  import { api } from '../lib/api';

  let { filename, write }: { filename: string; write: ((path: string) => Promise<void>) | null } = $props();
  let busy = $state(false);
  let status = $state('');
  let error = $state('');

  async function exportJSON() {
    const save = write;
    if (!save || busy) return;
    busy = true;
    status = '';
    error = '';
    try {
      const path = await api.pickSavePath(filename);
      if (path) {
        await save(path);
        status = 'Export saved';
      }
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="export">
  <button class="btn" type="button" onclick={exportJSON} disabled={!write || busy} title="JSON exports can contain unmasked data">
    {busy ? 'Exporting…' : 'Export JSON'}
  </button>
  {#if status}<span class="meta" role="status">{status}</span>{/if}
  {#if error}<span class="err" role="alert">{error}</span>{/if}
</div>

<style>
  .export {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.6rem;
  }
</style>
