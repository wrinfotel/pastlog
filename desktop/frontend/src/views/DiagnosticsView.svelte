<script lang="ts">
  // Diagnostics: the `pastlog agents` table plus its stderr conditions.
  import { api } from '../lib/api';
  import { fmtBytes, fmtInt } from '../lib/format';
  import Notes from '../components/Notes.svelte';
  import Loader from '../components/Loader.svelte';

  type Row = { name: string; detected: boolean; path: string | null; sessions: number; bytes: number };
  type Outcome = { home: string; agents: Row[]; warnings: string[]; skipped: number; error?: string };

  let data = $state<Outcome | null>(null);
  let error = $state('');
  let loading = $state(true);

  $effect(() => {
    loading = true;
    api
      .diagnostics()
      .then((o) => {
        data = o;
        error = '';
      })
      .catch((e) => (error = String(e)))
      .finally(() => (loading = false));
  });
</script>

{#if loading}
  <header class="page-head"><div><div class="kicker">// 06 · DIAGNOSTICS</div><h1>Diagnostics</h1><p class="lede">Check detected sources, storage paths, and collection health.</p></div><div class="page-mark">SYSTEM CHECK<br /><strong>LOCAL ONLY</strong></div></header>
  <Loader label="probing agents…" />
{:else if error}
  <header class="page-head"><div><div class="kicker">// 06 · DIAGNOSTICS</div><h1>Diagnostics</h1><p class="lede">Check detected sources, storage paths, and collection health.</p></div><div class="page-mark">SYSTEM CHECK<br /><strong>LOCAL ONLY</strong></div></header>
  <p class="err">{error}</p>
{:else if data}
  <header class="page-head"><div><div class="kicker">// 06 · DIAGNOSTICS</div><h1>Diagnostics</h1><p class="lede">Check detected sources, storage paths, and collection health.</p></div><div class="page-mark">SYSTEM CHECK<br /><strong>LOCAL ONLY</strong></div></header>
  {#if data.error}
    <p class="err">{data.error}</p>
  {:else}
    <div class="panel tablewrap">
      <table>
        <thead>
          <tr><th>agent</th><th class="r">sessions</th><th class="r">size</th><th>path</th></tr>
        </thead>
        <tbody>
          {#each data.agents as row}
            <tr class:off={!row.detected}>
              <td class="name">
                <span class="dot" class:off={!row.detected}></span>
                {row.name}
              </td>
              <td class="r">{row.detected ? fmtInt(row.sessions) : '-'}</td>
              <td class="r">{row.detected ? fmtBytes(row.bytes) : '(not found)'}</td>
              <td class="path" title={row.path ?? ''}>{row.detected ? row.path : ''}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Notes notes={data.warnings} />
    <p class="meta">home: <span class="mono">{data.home}</span></p>
  {/if}
{/if}

<style>
  .tablewrap {
    margin-top: 4px;
    padding: 6px 16px 8px;
  }
  tr.off {
    opacity: 0.62;
  }
  th.r,
  td.r {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  td.r {
    color: var(--text-2);
  }
  .name {
    color: var(--text);
    font-weight: 600;
  }
  .name .dot {
    margin-right: 8px;
    vertical-align: 1px;
  }
  .path {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11.5px;
    max-width: 340px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mono {
    font-family: var(--mono);
  }
</style>
