<script lang="ts">
  // Shared loading affordance: a cyan spinner with an optional label,
  // centered in whatever container drops it in. Views hide previous data
  // while a load runs and render this instead, so screen and filter
  // switches never flash stale rows before the fresh ones arrive.
  let { label = '' }: { label?: string } = $props();
</script>

<div class="loader" role="status" aria-live="polite">
  <span class="spin" aria-hidden="true"></span>
  {#if label}<span class="text">{label}</span>{/if}
</div>

<style>
  .loader {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    width: 100%;
    height: 100%;
    min-height: 110px;
    animation: fadein var(--speed) ease;
  }
  .spin {
    width: 15px;
    height: 15px;
    flex-shrink: 0;
    border-radius: 50%;
    border: 2px solid var(--accent-soft-2);
    border-top-color: var(--accent);
    animation: spin 0.7s linear infinite;
  }
  .text {
    color: var(--muted);
    font-size: 12px;
    letter-spacing: 0.02em;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
