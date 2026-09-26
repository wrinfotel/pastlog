// Dev-only mock of the generated Wails App bindings (wailsjs/go/app/App).
// Loaded when vite runs with PASTLOG_UI_MOCK=1 so every view can be
// rendered and screenshotted in a plain browser with fixture data.
// Never part of the production bundle.

export type Row = {
  name: string;
  detected: boolean;
  path: string | null;
  sessions: number;
  bytes: number;
};

const agents: Row[] = [
  { name: 'claude-code', detected: true, path: 'C:\\Users\\dev\\.claude\\projects', sessions: 412, bytes: 2_814_000_000 },
  { name: 'codex', detected: true, path: 'C:\\Users\\dev\\.codex\\sessions', sessions: 187, bytes: 943_000_000 },
  { name: 'zcode', detected: true, path: 'C:\\Users\\dev\\.zcode\\sessions', sessions: 96, bytes: 418_500_000 },
  { name: 'gemini-cli', detected: true, path: 'C:\\Users\\dev\\.gemini\\tmp', sessions: 41, bytes: 87_200_000 },
  { name: 'opencode', detected: false, path: null, sessions: 0, bytes: 0 },
];

const projects = [
  'C:\\dev\\shop-backend', 'C:\\dev\\shop-frontend', 'C:\\dev\\infra-terraform',
  'C:\\seoproject\\cliapp', 'C:\\dev\\ml-notes',
];

const models = ['claude-sonnet-4-5', 'claude-opus-4-1', 'claude-haiku-4-5'];

function sessionRows(n: number) {
  const out = [];
  for (let i = 0; i < n; i++) {
    const ag = agents[i % 4]!;
    const day = new Date(2026, 8, 19 - Math.floor(i / 2), 9 + (i % 9), (i * 13) % 60);
    out.push({
      id: `f${(i + 1).toString().padStart(3, '0')}${(i * 7 + 11).toString(16)}a9c3e1b7d5`,
      agent: ag.name,
      project: projects[i % projects.length]!,
      title:
        i % 3 === 0
          ? 'fix flaky checkout tests'
          : i % 3 === 1
            ? 'refactor auth middleware'
            : `investigate latency spike ${i}`,
      started_at: day.toISOString(),
      ended_at: new Date(day.getTime() + 40_000_000).toISOString(),
      messages: 8 + ((i * 17) % 140),
      size_bytes: 120_000 + ((i * 943_991) % 9_000_000),
    });
  }
  return out;
}

const allSessions = sessionRows(46);

function statsRows(by: string) {
  if (by === 'model')
    return models.map((m, i) => ({
      key: m,
      sessions: 40 + i * 23,
      messages: 900 + i * 640,
      tokens: {
        input: 1_200_000 - i * 180_000,
        output: 340_000 - i * 60_000,
        reasoning: 90_000 - i * 22_000,
        cache_read: 4_100_000 - i * 700_000,
        cache_write: 620_000 - i * 90_000,
        total: 6_350_000 - i * 1_052_000,
      },
      cost_usd: by === 'model' ? 41.2 - i * 9.7 : null,
    }));
  if (by === 'day')
    return Array.from({ length: 10 }, (_, i) => ({
      key: `2026-09-${(10 + i).toString().padStart(2, '0')}`,
      sessions: 6 + ((i * 5) % 20),
      messages: 120 + i * 47,
      tokens: {
        input: 240_000 + i * 31_000,
        output: 68_000 + i * 9_400,
        reasoning: 19_000 + i * 2_100,
        cache_read: 810_000 + i * 96_000,
        cache_write: 130_000 + i * 15_000,
        total: 1_267_000 + i * 153_500,
      },
      cost_usd: null,
    }));
  if (by === 'project')
    return projects.map((p, i) => ({
      key: p,
      sessions: 12 + i * 9,
      messages: 260 + i * 190,
      tokens: {
        input: 480_000 + i * 120_000,
        output: 132_000 + i * 34_000,
        reasoning: 38_000 + i * 8_800,
        cache_read: 1_640_000 + i * 380_000,
        cache_write: 260_000 + i * 61_000,
        total: 2_550_000 + i * 603_800,
      },
      cost_usd: 12.4 + i * 5.9,
    }));
  return agents
    .filter((a) => a.detected)
    .map((a, i) => ({
      key: a.name,
      sessions: a.sessions,
      messages: a.sessions * 34,
      tokens: {
        input: 2_400_000 - i * 380_000,
        output: 690_000 - i * 118_000,
        reasoning: 196_000 - i * 41_000,
        cache_read: 8_200_000 - i * 1_500_000,
        cache_write: 1_310_000 - i * 264_000,
        total: 12_796_000 - i * 2_303_000,
      },
      cost_usd: i < 3 ? 87.3 - i * 24.6 : null,
    }));
}

const transcript = [
  { kind: 'summary', role: '', timestamp: '2026-09-18T09:12:04Z', text: 'Session continued from an earlier conversation; context carried over.' },
  { kind: 'message', role: 'user', timestamp: '2026-09-18T09:12:04Z', text: 'The checkout E2E suite is flaky on CI — three failures this night, all timing out at the payment step. Can you look at the test and the checkout flow?' },
  {
    kind: 'message',
    role: 'assistant',
    timestamp: '2026-09-18T09:12:21Z',
    text: "Looking at the timing, the payment step waits for a **3DS challenge** iframe that only appears for certain card fixtures.\n\nTwo things stand out:\n\n1. `waitForSelector('#challenge')` has no timeout budget of its own\n2. the stub server adds 800ms latency in CI but not locally\n\n```ts\nawait page.waitForSelector('#challenge', { timeout: 5_000 });\n```\n\nI'd pin the timeout and make the stub deterministic — details below.",
  },
  {
    kind: 'tool_call',
    role: '',
    timestamp: '2026-09-18T09:12:40Z',
    // one long single line, so the engine's snippet window (200 runes) has to
    // cut it — the hit line below is not a substring of this text
    text: 'Bash(c:\\dev\\shop-backend) — CI retry run: npx playwright test e2e/checkout.spec.ts --project=chromium --grep "3DS challenge" --retries=2 --timeout=45000 --reporter=line > ci-run-4211.log 2>&1; tail -n 40 ci-run-4211.log | grep -E "passed|failed|flaky"',
  },
  {
    kind: 'tool_result',
    role: '',
    timestamp: '2026-09-18T09:12:41Z',
    text: 'read 29 lines (offset 120) — e2e/checkout.spec.ts',
  },
  { kind: 'message', role: 'user', timestamp: '2026-09-18T09:14:02Z', text: 'Yes — pin the timeout and fix the stub. Ship it.' },
  {
    kind: 'message',
    role: 'assistant',
    timestamp: '2026-09-18T09:14:30Z',
    text: 'Pinned the challenge wait to a 5s budget and switched the stub to a fixed 120ms latency.\n\n- `e2e/checkout.spec.ts` — explicit timeout\n- `e2e/stubs/payment.ts` — deterministic delay\n\nRe-ran the suite 10x locally: green.',
  },
];

// Emulates the engine's hit line for a long single-line entry: the line
// windowed to 200 runes around the match, '…' marking the cuts — by
// construction not a substring of the entry text.
function windowedHit(text: string, needle: string) {
  const at = text.indexOf(needle);
  let start = Math.round(at - (200 - needle.length) / 2);
  start = Math.max(0, Math.min(start, text.length - 200));
  const end = start + 200;
  const prefix = start > 0 ? '…' : '';
  const suffix = end < text.length ? '…' : '';
  return {
    line: prefix + text.slice(start, end) + suffix,
    match_start: prefix.length + at - start,
    match_end: prefix.length + at - start + needle.length,
  };
}

// entry_head anchors the viewer scroll: the head of the served transcript
// entry's full text, exactly what the desktop backend computes per hit.
const head = (i: number) => transcript[i]!.text.slice(0, 120);
const toolHit = windowedHit(transcript[3]!.text, 'challenge');

const searchResults = [
  {
    session: allSessions[2]!,
    hits: [
      { kind: 'message', role: 'assistant', timestamp: null, context: 'fix flaky checkout tests', line: 'the payment step waits for a 3DS challenge iframe', match_start: 26, match_end: 30, entry_head: head(2) },
      { kind: 'tool_call', role: '', timestamp: null, context: '', ...toolHit, entry_head: head(3) },
    ],
  },
  {
    session: allSessions[5]!,
    hits: [{ kind: 'message', role: 'user', timestamp: null, context: 'refactor auth middleware', line: 'can you check the checkout webhook signature handling', match_start: 18, match_end: 26, entry_head: head(1) }],
  },
  {
    session: allSessions[8]!,
    hits: [{ kind: 'message', role: 'assistant', timestamp: null, context: '', line: 'moved checkout totals into a separate service — see checkout/totals.go', match_start: 6, match_end: 14, entry_head: head(2) }],
  },
];

let settings = { home: '', theme: 'system', version: '0.2.0', commit: 'afba759', date: '2026-09-18' };

const delay = (ms = 120) => new Promise((r) => setTimeout(r, ms));

export async function Overview() {
  await delay();
  // warnings stay empty: the banner is real-backend-only, the mock must not
  // invent adapter conditions
  return { home: 'C:\\Users\\dev', agents, warnings: [] };
}

export async function Diagnostics() {
  await delay();
  return { home: 'C:\\Users\\dev', agents, warnings: [], skipped: 0 };
}

export async function Sessions() {
  await delay();
  return { sessions: allSessions, notes: [] };
}

export async function Search(q: string) {
  await delay(400);
  if (!q.trim()) return { results: [], hits: 0, truncated: false, notes: [] };
  return { results: searchResults, hits: 4, truncated: false, notes: [] };
}

export async function Entries(id: string) {
  await delay();
  if (id.startsWith('amb')) {
    return {
      status: 'ambiguous',
      candidates: [allSessions[0]!, allSessions[1]!],
      notes: [],
    };
  }
  return {
    status: 'ok',
    session: allSessions[2]!,
    entries: transcript,
    notes: [],
  };
}

export async function Stats(_opts: { by: string }) {
  await delay();
  return { rows: statsRows(_opts.by), notes: [] };
}

export async function Projects(agent: string) {
  await delay();
  const rows = projects.map((p, i) => {
    const r = statsRows('project')[i]!;
    return { project: p, sessions: r.sessions, messages: r.messages, tokens: r.tokens, cost_usd: r.cost_usd };
  });
  return { agent, rows, notes: [] };
}

export async function ProjectStats(agent: string, project: string) {
  await delay();
  return { agent, project, rows: statsRows('model'), notes: [] };
}

export async function Cancel() {}

export async function GetSettings() {
  return { ...settings };
}

export async function SetHome(home: string) {
  settings = { ...settings, home };
  return { ...settings };
}

export async function SetTheme(theme: string) {
  settings = { ...settings, theme };
  return { ...settings };
}

export async function ExportSession() {
  return { status: 'ok' };
}

export async function ExportSessions() {
  return { status: 'ok' };
}

export async function ExportSearch() {
  return { status: 'ok' };
}

export async function ExportStats() {
  return { status: 'ok' };
}
