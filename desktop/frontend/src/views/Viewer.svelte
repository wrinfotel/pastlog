<script lang="ts">
  // Session viewer: the full transcript — roles, timestamps, collapsible
  // tool calls and results, sanitized markdown for assistant text. Copy
  // affordances only; nothing is ever executed (spec §4.3/§4.4). The context
  // panel runs the `pastlog context` analysis on demand (a second pass over
  // the session, so it loads lazily on first open).
  import { api, type ContextOutcome, type ListOutcome } from '../lib/api';
  import type { SessionRow } from '../lib/api';
  import { fmtBytes, fmtDate, fmtInt, fmtTok, idPrefix } from '../lib/format';
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

  // context panel state; reset with every session load
  let ctxOpen = $state(false);
  let ctxLoading = $state(false);
  let ctxOutcome = $state<ContextOutcome | null>(null);
  let ctxError = $state('');

  async function load(id: string) {
    loading = true;
    hitIdx = -1;
    ctxOpen = false;
    ctxOutcome = null;
    ctxError = '';
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

  async function loadContext() {
    ctxLoading = true;
    try {
      ctxOutcome = await api.context(viewer.id);
      ctxError = '';
    } catch (e) {
      ctxError = String(e);
    } finally {
      ctxLoading = false;
    }
  }

  function toggleContext() {
    ctxOpen = !ctxOpen;
    if (ctxOpen && !ctxOutcome && !ctxLoading) void loadContext();
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
      <button class="btn" class:on={ctxOpen} onclick={toggleContext} title="why the context grew">
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
          <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
        </svg>
        CONTEXT
      </button>
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
  {#if ctxOpen}
    <section class="ctxpanel fadein">
      {#if ctxLoading}
        <Loader label="analyzing context…" />
      {:else if ctxError}
        <p class="err">{ctxError}</p>
      {:else if ctxOutcome?.status === 'ok' && ctxOutcome.profile}
        <div class="ctxhead">
          <span class="ctxtitle">context profile</span>
          <span class="ctxfact">~{fmtTok(ctxOutcome.profile.final)} tokens final</span>
          <span class="ctxfact">{fmtInt(ctxOutcome.profile.turns)} turns</span>
          {#if ctxOutcome.profile.compactions > 0}
            <span class="ctxfact">{fmtInt(ctxOutcome.profile.compactions)} compaction{ctxOutcome.profile.compactions === 1 ? '' : 's'}</span>
          {/if}
          <span class="ctxprec">{ctxOutcome.profile.precision}</span>
        </div>
        {#if ctxOutcome.profile.sparkline}
          <div class="spark">{ctxOutcome.profile.sparkline}</div>
        {/if}
        {#if ctxOutcome.profile.findings.length === 0}
          <p class="lean">no context-bloat signals — the session stayed lean</p>
        {:else}
          <ul class="findings">
            {#each ctxOutcome.profile.findings as f}
              <li>
                <span class="rule" class:danger={f.rule === 'R3'} class:warn={f.rule !== 'R3'}>{f.rule}</span>
                <span class="fdesc">{f.desc}</span>
              </li>
            {/each}
          </ul>
          {#if ctxOutcome.profile.advice.length > 0}
            <ul class="advice">
              {#each ctxOutcome.profile.advice as adv}
                <li>• {adv.text} <span class="arule">({adv.rule})</span></li>
              {/each}
            </ul>
          {/if}
        {/if}
      {:else if ctxOutcome}
        <p class="lean">context analysis unavailable for this session</p>
      {/if}
    </section>
  {/if}
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
  .btn.on {
    border-color: var(--accent);
    color: var(--accent);
  }
  .ctxpanel {
    margin-top: 12px;
    background: var(--panel);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-l);
    padding: 10px 14px 12px;
  }
  .ctxhead {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .ctxtitle {
    font-family: var(--mono);
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.07em;
    text-transform: uppercase;
    color: var(--muted);
  }
  .ctxfact {
    font-family: var(--mono);
    font-size: 11px;
    color: var(--text-2);
  }
  .ctxprec {
    margin-left: auto;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 10.5px;
    padding: 1px 7px;
    border-radius: 5px;
  }
  .spark {
    font-family: var(--mono);
    font-size: 18px;
    line-height: 1.2;
    color: var(--accent);
    letter-spacing: 2px;
    margin-top: 8px;
  }
  .findings,
  .advice {
    list-style: none;
    margin: 10px 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .findings li {
    display: flex;
    align-items: baseline;
    gap: 9px;
  }
  .rule {
    flex-shrink: 0;
    font-family: var(--mono);
    font-size: 10.5px;
    font-weight: 600;
    border-radius: 5px;
    padding: 1px 7px;
  }
  .rule.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .rule.danger {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .fdesc {
    font-size: 12.5px;
    color: var(--text-2);
  }
  .advice {
    margin-top: 9px;
    gap: 4px;
    font-size: 12px;
    color: var(--muted);
  }
  .arule {
    font-family: var(--mono);
    font-size: 10.5px;
    color: var(--faint);
  }
  .lean {
    margin: 6px 0 0;
    font-size: 12.5px;
    color: var(--muted);
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
