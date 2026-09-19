<script lang="ts">
  // Virtualized list (spec §6 mandates virtualization): renders only the
  // visible window plus overscan. Rows are supplied as a Svelte 5 snippet so
  // the caller controls markup; the list owns the scroll math.
  import { window2 } from '../lib/virtual';

  let {
    items,
    itemHeight,
    row,
  }: {
    items: unknown[];
    itemHeight: number;
    row: import('svelte').Snippet<[unknown, number]>;
  } = $props();

  let viewport = $state<HTMLDivElement | undefined>(undefined);
  let scrollTop = $state(0);
  let viewportH = $state(600);

  const win = $derived(window2(scrollTop, viewportH, items.length, itemHeight));

  $effect(() => {
    if (viewport) viewportH = viewport.clientHeight || 600;
  });

  function onScroll() {
    if (viewport) scrollTop = viewport.scrollTop;
  }
</script>

<div class="vlist" bind:this={viewport} onscroll={onScroll}>
  <div style="height: {items.length * itemHeight}px; position: relative;">
    {#each items.slice(win.start, win.end) as item, i}
      <div class="vrow" style="top: {(win.start + i) * itemHeight}px; height: {itemHeight}px;">
        {@render row(item, win.start + i)}
      </div>
    {/each}
  </div>
</div>

<style>
  .vlist {
    height: 100%;
    overflow-y: auto;
    position: relative;
  }
  .vrow {
    position: absolute;
    left: 0;
    right: 0;
  }
</style>
