<script lang="ts">
  // Optimize: the `pastlog optimize` report — waste patterns that repeat
  // across a scope's sessions (SPEC-optimize.md). Entered from a project
  // page (that project only); no sidebar item, like the timeline. Advice
  // only: nothing is ever applied.
  import { api, emptyFilter, onProgress } from '../lib/api';
  import type { OptimizeOutcome } from '../lib/api';
  import { fmtBytes, fmtInt, fmtTok } from '../lib/format';
  import { go } from '../lib/stores.svelte';
  import { optimizeNav } from '../lib/optimize.svelte';
  import Notes from '../components/Notes.svelte';
  import Loader from '../components/Loader.svelte';

  let out = $state<OptimizeOutcome | null>(null);
  let notes = $state<string[]>([]);
  let error = $state('');
  let progress = $state<{ scanned: number } | null>(null);
  let loading = $state(true);

  const stopProgress = onProgress((e) => {
    if (e.op === 'optimize') progress = { scanned: e.scanned };
  });

  let seq = 0;

  async function load() {
    const mine = ++seq;
    loading = true;
    progress = null;
    try {
      const o = await api.optimize({
        filter: { ...emptyFilter(), agent: optimizeNav.agent, project: optimizeNav.project },
      });
      if (mine !== seq) return; // a newer scope superseded this load
      out = o;
      notes = o.notes ?? [];
      error = '';
    } catch (e) {
      if (mine !== seq) return;
      out = null;
      notes = [];
      error = String(e);
    } finally {
      if (mine === seq) {
        progress = null;
        loading = false;
      }
    }
  }

  $effect(() => {
    void load();
  });

  function cancel() {
    void api.cancel();
  }
</script>

<div class="backbar">
  <button class="btn" onclick={() => go(optimizeNav.from)}>
    <svg
      viewBox="0 0 24 24"
      width="13"
      height="13"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="m15 18-6-6 6-6" />
    </svg>
    {optimizeNav.from === 'home' ? 'Back to home' : 'Back to project'}
  </button>
</div>
<header class="page-head">
  <div>
    <div class="kicker">// OPTIMIZE · CROSS-SESSION WASTE</div>
    {#if optimizeNav.project}
      <h1 class="proj">{optimizeNav.project}</h1>
      <p class="meta"><span class="chip">{optimizeNav.agent}</span> &middot; what repeats across sessions</p>
    {:else}
      <h1>Optimize</h1>
      <p class="lede">What the context analysis keeps finding, folded across every session — advice only, nothing is applied.</p>
    {/if}
  </div>
  <div class="page-mark">READ-ONLY<br /><strong>ADVICE</strong></div>
</header>
<Notes notes={!loading ? notes : []} />
{#if progress}
  <div class="progress">
    <span>analyzing… {progress.scanned} sessions</span>
    <button class="btn" onclick={cancel}>Cancel</button>
  </div>
{/if}
{#if !loading && error}<p class="err">{error}</p>{/if}
{#if loading}
  {#if !progress}
    <Loader label="folding sessions into patterns…" />
  {/if}
{:else if out?.report}
  {#if out.report.findings.length === 0}
    <p class="meta">no cross-session waste patterns — this scope looks lean</p>
  {:else}
    {#if out.report.advice.length > 0}
      <section class="panel advice fadein">
        <div class="sect">advice</div>
        <ul>
          {#each out.report.advice as adv}
            <li>
              <span class="rule" class:danger={adv.rule === 'R8'} class:warn={adv.rule !== 'R8'}>{adv.rule}</span>
              <span>{adv.text}</span>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
    <section class="panel fadein">
      <div class="sect">findings</div>
      <ul class="findings">
        {#each out.report.findings as f}
          <li>
            <span class="rule" class:danger={f.rule === 'R8'} class:warn={f.rule !== 'R8'}>{f.rule}</span>
            <span class="what">
              {#if f.label}<span class="label" title={f.label}>{f.label}</span>{/if}
              <span class="desc">{f.desc}</span>
            </span>
            <span class="impact" title="{f.sessions} sessions · ×{fmtInt(f.count)}">
              {#if f.tokens}<span>~{fmtTok(f.tokens)} tok</span>
              {:else}<span>{fmtBytes(f.bytes)}</span>{/if}
              <span class="impactmeta">{fmtInt(f.sessions)} sess · ×{fmtInt(f.count)}</span>
            </span>
          </li>
        {/each}
      </ul>
    </section>
    <p class="meta">
      top {out.report.findings.length} patterns over {fmtInt(out.report.sessions)} sessions analyzed
      {#if out.report.truncated > 0}&middot; {out.report.truncated} smaller omitted{/if}
    </p>
  {/if}
{/if}

<style>
  .proj {
    font-family: var(--mono);
    font-size: 16px;
    letter-spacing: 0;
    overflow-wrap: anywhere;
  }
  .advice {
    margin-top: 12px;
  }
  .sect {
    color: var(--muted);
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    padding: 12px 16px 8px;
    border-bottom: 1px solid var(--border);
  }
  .advice ul,
  .findings {
    list-style: none;
    margin: 0;
    padding: 6px 0;
  }
  .advice li {
    display: flex;
    gap: 12px;
    align-items: baseline;
    padding: 7px 16px;
    color: var(--text-2);
    font-size: 13px;
  }
  .findings li {
    display: grid;
    grid-template-columns: 52px minmax(200px, 1fr) 170px;
    gap: 12px;
    align-items: baseline;
    padding: 9px 16px;
    border-bottom: 1px solid var(--border-soft);
  }
  .findings li:last-child {
    border-bottom: 0;
  }
  .rule {
    padding: 1px 7px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--text-2);
    font: 600 10px var(--mono);
    letter-spacing: 0.08em;
    white-space: nowrap;
  }
  .rule.warn {
    color: var(--warn);
    background: var(--warn-soft);
  }
  .rule.danger {
    color: var(--danger);
    background: var(--danger-soft);
  }
  .what {
    min-width: 0;
  }
  .label {
    display: block;
    font-family: var(--mono);
    font-size: 12px;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .desc {
    display: block;
    color: var(--text-2);
    font-size: 12.5px;
  }
  .impact {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--text);
    font-size: 12.5px;
    font-weight: 600;
  }
  .impactmeta {
    display: block;
    color: var(--muted);
    font-size: 11px;
    font-weight: 400;
  }
</style>
