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
  let error = $state('');
  let loading = $state(false);
  let listEl: HTMLDivElement | undefined = $state();
  let hitIdx = $state(-1);

  async function load(id: string) {
    loading = true;
    hitIdx = -1;
    try {
      outcome = (await api.entries(id)) as Outcome;
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

  const entries = $derived<EntryView[]>(
    (outcome?.entries ?? []).map((e) => ({
      entry: e,
      // tool activity and long results start collapsed; messages are open
      expanded: e.kind === 'message' || e.kind === 'summary',
    })),
  );

  // Scroll to the search hit (R-D11): first entry of the same kind/role whose
  // text contains the hit line.
  $effect(() => {
    if (!viewer.hitLine || entries.length === 0 || !listEl || hitIdx >= 0) return;
    const i = entries.findIndex(
      (ev) =>
        (!viewer.hitKind || ev.entry.kind === viewer.hitKind) &&
        (!viewer.hitRole || ev.entry.role === viewer.hitRole) &&
        ev.entry.text.includes(viewer.hitLine),
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
</script>

{#if loading}
  <p class="meta">loading…</p>
{:else if error}
  <p class="err">{error}</p>
  <button class="btn" onclick={() => go('sessions')}>Back to sessions</button>
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
      <h1>{outcome.session.title || `session ${idPrefix(outcome.session.id)}`}</h1>
      <p class="meta">
        {outcome.session.agent} · {outcome.session.id} · {outcome.session.project || '-'} ·
        {fmtDate(outcome.session.started_at)} · {fmtInt(outcome.session.messages)} messages ·
        {fmtBytes(outcome.session.size_bytes)}
      </p>
    </div>
    <div class="actions">
      <button class="btn" onclick={() => exportSession('json')}>Export JSON</button>
      <button class="btn" onclick={() => exportSession('md')}>Export MD</button>
    </div>
  </div>
  <Notes notes={outcome.notes ?? []} />
  <div class="transcript" bind:this={listEl}>
    {#each entries as ev, i}
      <div class="entry" class:hit={i === hitIdx} data-idx={i}>
        <div class="ehead">
          <button class="lbl" class:tool={ev.entry.kind !== 'message'} onclick={() => toggle(i)}>
            {#if ev.entry.kind !== 'message' && ev.entry.kind !== 'summary'}
              <span class="tri">{ev.expanded ? '▾' : '▸'}</span>
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
  .actions {
    display: flex;
    gap: 8px;
    flex-shrink: 0;
  }
  .btn {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font: inherit;
  }
  .btn:hover {
    border-color: var(--accent);
  }
  .transcript {
    margin-top: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-height: calc(100vh - 220px);
    overflow-y: auto;
  }
  .entry {
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 6px 10px;
  }
  .entry.hit {
    border-color: var(--accent);
  }
  .ehead {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .lbl {
    background: none;
    border: 0;
    color: var(--accent);
    font-weight: 600;
    cursor: pointer;
    padding: 2px 0;
    font: inherit;
  }
  .lbl.tool {
    color: var(--muted);
  }
  .tri {
    display: inline-block;
    width: 12px;
  }
  .time {
    color: var(--muted);
    font-size: 12px;
  }
  .copy {
    margin-left: auto;
    background: none;
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--muted);
    font-size: 11px;
    cursor: pointer;
  }
  .copy:hover {
    color: var(--text);
  }
  .body {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    margin: 6px 0 2px;
    font-size: 13.5px;
  }
  .body.md {
    white-space: normal;
  }
  .body.md :global(pre) {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    overflow-x: auto;
  }
  .body.md :global(code) {
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 12.5px;
  }
  .body.md :global(a) {
    color: var(--accent);
  }
  .cands {
    list-style: none;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .meta {
    color: var(--muted);
    font-size: 12px;
  }
  .err {
    color: #f87171;
  }
</style>
