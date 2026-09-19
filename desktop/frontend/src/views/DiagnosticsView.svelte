<script lang="ts">
  // Diagnostics: the `pastlog agents` table plus its stderr conditions.
  import { api } from '../lib/api';
  import { fmtBytes, fmtInt } from '../lib/format';
  import Notes from '../components/Notes.svelte';

  type Row = { name: string; detected: boolean; path: string | null; sessions: number; bytes: number };
  type Outcome = { home: string; agents: Row[]; warnings: string[]; skipped: number; error?: string };

  let data = $state<Outcome | null>(null);

  $effect(() => {
    api.diagnostics().then((o) => (data = o));
  });
</script>

{#if data}
  <h1>Diagnostics</h1>
  {#if data.error}
    <p class="err">{data.error}</p>
  {:else}
    <table>
      <thead>
        <tr><th>agent</th><th>sessions</th><th>size</th><th>path</th></tr>
      </thead>
      <tbody>
        {#each data.agents as row}
          <tr class:off={!row.detected}>
            <td class="name">{row.name}</td>
            <td>{row.detected ? fmtInt(row.sessions) : '-'}</td>
            <td>{row.detected ? fmtBytes(row.bytes) : '(not found)'}</td>
            <td class="path">{row.detected ? row.path : ''}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    <Notes notes={data.warnings} />
    <p class="meta">home: {data.home}</p>
  {/if}
{/if}

<style>
  table {
    border-collapse: collapse;
    margin-top: 10px;
    width: 100%;
  }
  th,
  td {
    text-align: left;
    padding: 6px 14px 6px 0;
    border-bottom: 1px solid var(--border);
  }
  tr.off {
    opacity: 0.55;
  }
  .name {
    color: var(--accent);
    font-weight: 600;
  }
  .path,
  .meta {
    color: var(--muted);
    font-size: 12px;
  }
  .err {
    color: #f87171;
  }
</style>
