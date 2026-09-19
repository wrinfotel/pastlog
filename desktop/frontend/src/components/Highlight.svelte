<script lang="ts">
  // Renders a match line as plain text with the matched span highlighted.
  // No {@html}: log content is untrusted (spec §4.4).
  import { splitMatch } from '../lib/highlight';

  let { line, start, end }: { line: string; start: number; end: number } = $props();

  const parts = $derived(splitMatch(line, start, end));
</script>

<span class="hitline">{parts[0]}<mark>{parts[1]}</mark>{parts[2]}</span>

<style>
  mark {
    background: var(--match);
    color: var(--text);
    border-radius: 3px;
    padding: 0 2px;
  }
</style>
