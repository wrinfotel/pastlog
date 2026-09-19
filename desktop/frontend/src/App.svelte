<script lang="ts">
  import { mount } from 'svelte';

  let lines: string[] = $state([]);
  let error = $state('');

  $effect(() => {
    // D0 spike: call the bound Go service and show what it detects.
    (window as any).go?.app?.App?.SpikeOverview()
      .then((r: string[]) => (lines = r))
      .catch((e: unknown) => (error = String(e)));
  });
</script>

<main>
  <h1>pastlog Desktop</h1>
  <p>D0 spike — detected agent storage:</p>
  {#if error}<p class="err">{error}</p>{/if}
  <ul>
    {#each lines as line}
      <li>{line}</li>
    {/each}
  </ul>
</main>

<style>
  main { padding: 24px; }
  .err { color: #f87171; }
</style>
