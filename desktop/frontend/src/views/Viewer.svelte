<script lang="ts">
  // Session viewer: the full transcript — roles, timestamps, collapsible
  // tool calls and results, sanitized markdown for assistant text. Copy
  // affordances only; nothing is ever executed (spec §4.3/§4.4).
  import { api, type ListOutcome } from '../lib/api';
  import type { SessionRow } from '../lib/api';
  import { fmtBytes, fmtDate, fmtInt, idPrefix } from '../lib/format';
  import { fmtTime } from '../lib/format';
  import { renderMarkdown } from '../lib/markdown';
  import { go } from '../lib/stores.svelte';
  import { viewer } from '../lib/viewer.svelte';
  import Notes from '../components/Notes.svelte';
  import Loader from '../components/Loader.svelte';

  type Entry = { kind: string; role: string; text: string; timestamp: string | null };
  type Outcome = {
    status: 'ok' | 'ambiguous' | 'notfound';
    session?: SessionRow | null;
    entries?: Entry[] | null;
    candidates?: SessionRow[] | null;
    notes?: string[] | null;
  };

  type EntryView = { entry: Entry; expanded: boolean };

  let outcome = $state<Outcome | null>(null);
  // $state (not $derived): derived contents are not deep-reactive, so the
  // per-entry `expanded` mutation in toggle() would be lost and the
  // collapse/expand toggles would stay dead. Rebuilt per load below.
  let entries = $state<EntryView[]>([]);
  let error = $state('');
  // start in the loading state when opened with a target id, so the very
  // first paint shows the spinner instead of a blank frame
  let loading = $state(viewer.id !== '');
  let listEl: HTMLDivElement | undefined = $state();
  let hitIdx = $state(-1);

  async function load(id: string) {
    loading = true;
    hitIdx = -1;
    try {
      outcome = (await api.entries(id)) as Outcome;
      entries = (outcome?.entries ?? []).map((e) => ({
        entry: e,
        // tool activity and long results start collapsed; messages are open
        expanded: e.kind === 'message' || e.kind === 'summary',
      }));
      error = '';
    } catch (e) {
      error = String(e);
    } finally {
      loading = false;
    }
  }

  // (Re)load whenever the viewer target changes.
  $effect(() => {
    if (viewer.id) void load(viewer.id);
  });

  // Scroll to the search hit (R-D11): the first entry of the same kind/role
  // whose text starts with the backend's anchor (the head of the hit entry's
  // full text). The Line snippet is windowed/synthesized and usually not a
  // substring of the entry, so it cannot serve as the anchor.
  $effect(() => {
    if (!viewer.hitHead || entries.length === 0 || !listEl || hitIdx >= 0) return;
    const i = entries.findIndex(
      (ev) =>
        (!viewer.hitKind || ev.entry.kind === viewer.hitKind) &&
        (!viewer.hitRole || ev.entry.role === viewer.hitRole) &&
        ev.entry.text.startsWith(viewer.hitHead),
    );
    if (i >= 0) {
      hitIdx = i;
      const el = listEl.querySelector(`[data-idx="${i}"]`);
      el?.scrollIntoView({ block: 'center' });
    }
  });

  function toggle(i: number) {
    entries[i].expanded = !entries[i].expanded;
  }

  async function copyText(text: string) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      /* clipboard denied — the affordance stays best effort */
    }
  }

  async function exportSession(format: 'json' | 'md') {
    if (!outcome?.session) return;
    const ext = format;
    const name = `session-${idPrefix(outcome.session.id)}.${ext}`;
    const path = await api.pickSavePath(name);
    if (!path) return;
    try {
      const res = await api.exportSession(viewer.id, format, path);
      if (res.status === 'ambiguous' && res.candidates?.length) {
        error = 'ambiguous session id — pick a candidate in the search or sessions view';
      } else {
        error = '';
      }
    } catch (e) {
      error = String(e);
    }
  }

  const label = (kind: string, role: string) =>
    kind === 'tool_call' ? 'tool call'
    : kind === 'tool_result' ? 'tool result'
    : kind === 'summary' ? 'summary'
    : role || 'message';

  // the back affordance returns to wherever the session was opened from —
  // a project page keeps its selection in the module store
  const backLabel = $derived(
    viewer.from === 'projects' ? 'Back to project'
    : viewer.from === 'search' ? 'Back to search'
    : 'Back to sessions',
  );

  const roleClass = (kind: string, role: string) =>
    kind === 'message' ? `role-${role || 'message'}` : '';
</script>

<div class="backbar">
  <button class="btn" onclick={() => go(viewer.from)}>
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
    {backLabel}
  </button>
</div>

{#if loading}
  <Loader label="loading session…" />
{:else if error}
  <p class="err">{error}</p>
{:else if outcome?.status === 'ambiguous' && outcome.candidates}
  <h1>Ambiguous session id</h1>
  <p class="meta">several sessions match — pick one:</p>
  <ul class="cands">
    {#each outcome.candidates as c}
      <li>
        <button class="btn" onclick={() => { openSession(c.id); go('viewer'); }}>
          {idPrefix(c.id)} · {c.agent} · {fmtDate(c.started_at)} · {c.project || '-'}
        </button>
      </li>
    {/each}
  </ul>
{:else if outcome?.status === 'notfound'}
  <h1>Session not found</h1>
  <p class="meta">no session matches this id</p>
{:else if outcome?.session}
  <div class="top">
    <div>
      <div class="kicker">// SESSION</div>
      <h1>{outcome.session.title || `session ${idPrefix(outcome.session.id)}`}</h1>
      <p class="meta facts">
        <span class="chip">{outcome.session.agent}</span>
        <span class="fact">{outcome.session.id}</span>
        <span class="fact">{outcome.session.project || '-'}</span>
        <span class="fact">{fmtDate(outcome.session.started_at)}</span>
        <span class="fact">{fmtInt(outcome.session.messages)} messages</span>
        <span class="fact">{fmtBytes(outcome.session.size_bytes)}</span>
      </p>
    </div>
    <div class="actions">
      <button class="btn" onclick={() => exportSession('json')}>
        <svg
          viewBox="0 0 24 24"
          width="13"
          height="13"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="7 10 12 15 17 10" />
          <line x1="12" y1="15" x2="12" y2="3" />
        </svg>
        JSON
      </button>
      <button class="btn" onclick={() => exportSession('md')}>
        <svg
          viewBox="0 0 24 24"
          width="13"
          height="13"
          fill="none"
          stroke="currentColor"
          stroke-width="1.8"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="7 10 12 15 17 10" />
          <line x1="12" y1="15" x2="12" y2="3" />
        </svg>
        MD
      </button>
    </div>
  </div>
  <Notes notes={outcome.notes ?? []} />
  <div class="transcript fadein" bind:this={listEl}>
    {#each entries as ev, i}
      <div
        class="entry"
        class:tool={ev.entry.kind !== 'message' && ev.entry.kind !== 'summary'}
        class:hit={i === hitIdx}
        data-idx={i}
      >
        <div class="ehead">
          <button class="lbl {roleClass(ev.entry.kind, ev.entry.role)}" onclick={() => toggle(i)}>
            {#if ev.entry.kind !== 'message' && ev.entry.kind !== 'summary'}
              <svg
                class="tri"
                viewBox="0 0 24 24"
                width="10"
                height="10"
                fill="none"
                stroke="currentColor"
                stroke-width="2.4"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d={ev.expanded ? 'm6 9 6 6 6-6' : 'm9 6 6 6-6 6'} />
              </svg>
            {/if}
            {label(ev.entry.kind, ev.entry.role)}
          </button>
          <span class="time">{fmtTime(ev.entry.timestamp)}</span>
          <button class="copy" onclick={() => copyText(ev.entry.text)} title="copy text">copy</button>
        </div>
        {#if ev.expanded}
          {#if ev.entry.kind === 'message' && ev.entry.role === 'assistant'}
            <div class="body md">{@html renderMarkdown(ev.entry.text)}</div>
          {:else}
            <div class="body">{ev.entry.text}</div>
          {/if}
        {/if}
      </div>
    {/each}
  </div>
{/if}

<style>
  .top {
    display: flex;
    justify-content: space-between;
    align-items: start;
    gap: 12px;
  }
  .top h1 {
    margin-bottom: 6px;
    font-size: 18px;
    font-weight: 700;
    letter-spacing: -0.015em;
  }
  .facts {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .fact {
    font-family: var(--mono);
    font-size: 11px;
  }
  .fact + .fact::before {
    content: '·';
    margin-right: 8px;
    color: var(--faint);
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
  .transcript {
    margin-top: 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-height: calc(100vh - 212px);
    overflow-y: auto;
    padding-right: 2px;
  }
  .entry {
    background: var(--panel);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-l);
    padding: 8px 12px;
    transition:
      border-color var(--speed) ease,
      box-shadow var(--speed) ease;
  }
  .entry.tool {
    background: var(--inset);
    border-color: var(--border-subtle);
    box-shadow: none;
  }
  .entry.hit {
    border-color: var(--accent);
    box-shadow:
      var(--shadow-1),
      0 0 0 3px var(--accent-soft);
  }
  .ehead {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .lbl {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    background: var(--accent-soft);
    color: var(--accent);
    font-family: var(--mono);
    font-size: 11px;
    font-weight: 600;
    border: 0;
    border-radius: 5px;
    padding: 2px 8px;
    cursor: pointer;
  }
  .lbl:hover {
    background: var(--accent-soft-2);
  }
  .lbl.role-user {
    background: var(--panel-hover);
    color: var(--text-2);
  }
  .entry.tool .lbl {
    background: none;
    color: var(--muted);
    padding: 2px 2px;
  }
  .entry.tool .lbl:hover {
    color: var(--text-2);
  }
  .tri {
    flex-shrink: 0;
  }
  .time {
    color: var(--muted);
    font-family: var(--mono);
    font-size: 11px;
  }
  .copy {
    margin-left: auto;
    background: none;
    border: 1px solid transparent;
    border-radius: 5px;
    color: var(--muted);
    font-size: 11px;
    padding: 1px 7px;
    cursor: pointer;
    transition:
      color var(--speed) ease,
      border-color var(--speed) ease,
      background var(--speed) ease;
  }
  .copy:hover {
    color: var(--text-2);
    border-color: var(--border);
    background: var(--panel-hover);
  }
  .body {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    margin: 8px 0 4px;
    font-size: 13px;
    color: var(--text-2);
  }
  .entry.tool .body {
    font-family: var(--mono);
    font-size: 12px;
    color: var(--muted);
  }
  .body.md {
    white-space: normal;
    color: var(--text-2);
  }
  .body.md :global(pre) {
    background: var(--inset);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-s);
    padding: 10px 12px;
    overflow-x: auto;
  }
  .body.md :global(code) {
    font-family: var(--mono);
    font-size: 12px;
  }
  .body.md :global(a) {
    color: var(--accent);
  }
  .body.md :global(h1),
  .body.md :global(h2),
  .body.md :global(h3) {
    font-size: 14px;
    margin: 14px 0 6px;
  }
  .cands {
    list-style: none;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
</style>
