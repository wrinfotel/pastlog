<script lang="ts">
  // Stub — replaced by the full session viewer in D3 (T14).
  import { viewer } from '../lib/viewer.svelte';
</script>

<h1>Session</h1>
<p class="meta">viewer arrives with D3 — session {viewer.id || '(none)'}</p>

<style>
  .meta {
    color: var(--muted);
  }
</style>
