<script lang="ts">
  // Timeline: the `pastlog timeline --messages` stream — every matching
  // session's user/assistant messages merged chronologically, the
  // "how did we get here" view. Entered from Home (all agents) or a project
  // page (that project only); no sidebar item, like sessions.
  import { api, onProgress } from '../lib/api';
  import type { TimelineEvent, TimelineOutcome } from '../lib/api';
  import { fmtDate, fmtInt, idPrefix } from '../lib/format';
  import { go } from '../lib/stores.svelte';
  import { openSession } from '../lib/viewer.svelte';
  import { timelineNav } from '../lib/timeline.svelte';
  import Notes from '../components/Notes.svelte';
  import VirtualList from '../components/VirtualList.svelte';
  import Loader from '../components/Loader.svelte';

  let out = $state<TimelineOutcome | null>(null);
  let notes = $state<string[]>([]);
  let error = $state('');
  let progress = $state<{ scanned: number } | null>(null);
  let loading = $state(true);

  const stopProgress = onProgress((e) => {
    if (e.op === 'timeline') progress = { scanned: e.scanned };
  });

  let seq = 0;

  async function load() {
    const mine = ++seq;
    loading = true;
    progress = null;
    try {
      const o = await api.timeline({ agent: timelineNav.agent, project: timelineNav.project, since: '', until: '', limit: 0 });
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

  function open(e: TimelineEvent) {
    openSession(e.session, undefined, 'timeline');
    go('viewer');
  }

  function cancel() {
    void api.cancel();
  }
</script>

<div class="backbar">
  <button class="btn" onclick={() => go(timelineNav.from)}>
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
    {timelineNav.from === 'home' ? 'Back to home' : 'Back to project'}
  </button>
</div>
<header class="page-head">
  <div>
    <div class="kicker">// TIMELINE · MERGED MESSAGE STREAM</div>
    {#if timelineNav.project}
      <h1 class="proj">{timelineNav.project}</h1>
      <p class="meta"><span class="chip">{timelineNav.agent}</span> &middot; every message in one stream</p>
    {:else}
      <h1>Timeline</h1>
      <p class="lede">All agents' messages in one chronological stream — how the work interleaved.</p>
    {/if}
  </div>
  <div class="page-mark">CROSS-SESSION<br /><strong>OLDEST FIRST</strong></div>
</header>
<Notes notes={!loading ? notes : []} />
{#if progress}
  <div class="progress">
    <span>scanning… {progress.scanned} sessions</span>
    <button class="btn" onclick={cancel}>Cancel</button>
  </div>
{/if}
{#if !loading && error}<p class="err">{error}</p>{/if}
{#if loading}
  {#if !progress}
    <Loader label="merging message stream…" />
  {/if}
{:else if out}
  {#if out.events.length > 0}
    <div class="panel tablewrap fadein">
      <div class="thead">
        <span>time</span>
        <span>agent</span>
        <span>role</span>
        <span>message</span>
        <span class="r">session</span>
      </div>
      <div class="tlist">
        <VirtualList items={out.events} itemHeight={36}>
          {#snippet row(e)}
            <button class="trow" onclick={() => open(e)} title={e.text}>
              <span class="muted">{fmtDate(e.timestamp)}</span>
              <span><span class="chip">{e.agent}</span></span>
              <span><span class="role" class:user={e.role === 'user'}>{e.role || 'message'}</span></span>
              <span class="text">{e.text}</span>
              <span class="r muted">{idPrefix(e.session)}</span>
            </button>
          {/snippet}
        </VirtualList>
      </div>
    </div>
    <p class="meta">{fmtInt(out.events.length)} events · newest 500 kept</p>
  {:else}
    <p class="meta">no messages found for this scope</p>
  {/if}
{/if}

<style>
  .proj {
    font-family: var(--mono);
    font-size: 16px;
    letter-spacing: 0;
    overflow-wrap: anywhere;
  }
  .tablewrap {
    margin-top: 12px;
    padding: 4px 16px 8px;
  }
  .thead,
  .trow {
    display: grid;
    grid-template-columns: 130px 110px 90px minmax(200px, 1fr) 80px;
    gap: 10px;
    padding: 0 4px;
    align-items: center;
  }
  .thead {
    position: sticky;
    top: 0;
    z-index: 1;
    color: var(--muted);
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    padding: 8px 14px 7px 4px; /* +10px: the list's reserved scrollbar lane */
    background: var(--panel);
    border-bottom: 1px solid var(--border);
  }
  .tlist {
    height: calc(100vh - 360px);
    min-height: 180px;
  }
  .trow {
    width: 100%;
    height: 100%;
    background: none;
    border: 0;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--speed) ease;
  }
  .trow:hover {
    background: var(--panel-hover);
  }
  .text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
  }
  .role {
    padding: 1px 7px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--text-2);
    font: 600 9px var(--mono);
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .role.user {
    color: var(--accent);
    background: var(--accent-soft);
  }
  .muted {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11.5px;
  }
  .r {
    text-align: right;
  }
</style>
