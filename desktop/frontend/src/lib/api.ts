// Single import point over the generated Wails bindings (frontend/wailsjs).
// Views never import the generated modules directly, so backend renames land
// in exactly one file.
import * as bindings from '../../wailsjs/go/app/App';
import * as bridge from '../../wailsjs/go/main/guiBridge';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

export type FilterOptions = {
  agent: string;
  project: string;
  since: string;
  until: string;
  limit: number;
};

export const emptyFilter = (): FilterOptions => ({
  agent: '',
  project: '',
  since: '',
  until: '',
  limit: 0,
});

export type AgentRow = {
  name: string;
  detected: boolean;
  path: string | null;
  sessions: number;
  bytes: number;
};

export type OverviewOutcome = {
  home: string;
  agents: AgentRow[];
  warnings: string[];
  error?: string;
};

export type SessionRow = {
  id: string;
  agent: string;
  project: string;
  title: string;
  started_at: string | null;
  ended_at: string | null;
  messages: number;
  size_bytes: number;
  parent_id: string; // "" for top-level sessions (0.2.3)
};

export type ListOutcome = {
  sessions: SessionRow[];
  notes: string[];
};

export type SearchHit = {
  kind: string;
  role: string;
  timestamp: string | null;
  context: string;
  line: string;
  match_start: number; // rune offset into line
  match_end: number;
  // head of the hit entry's full text — the viewer's scroll anchor (the line
  // snippet is windowed/synthesized and usually not a substring of the entry)
  entry_head: string;
};

export type SearchResult = {
  session: SessionRow;
  hits: SearchHit[];
};

export type SearchOutcome = {
  results: SearchResult[];
  hits: number;
  truncated: boolean;
  cancelled: boolean;
  notes: string[];
};

export type StatsRow = {
  key: string;
  sessions: number;
  messages: number;
  tokens: { input: number; output: number; reasoning: number; cache_read: number; cache_write: number; total: number };
  cost_usd: number | null;
};

export type ProjectRow = {
  project: string;
  sessions: number;
  messages: number;
  tokens: { input: number; output: number; reasoning: number; cache_read: number; cache_write: number; total: number };
  cost_usd: number | null;
};

export type ProjectsOutcome = {
  agent: string;
  rows: ProjectRow[];
  notes: string[];
};

export type ProjectStatsOutcome = {
  agent: string;
  project: string;
  rows: StatsRow[];
  notes: string[];
};

export type ContextFinding = {
  rule: string; // R1..R5
  tool?: string;
  desc: string;
  bytes: number;
};

export type ContextAdvice = {
  rule: string;
  text: string;
};

export type ContextProfile = {
  final: number;
  final_exact: boolean;
  turns: number;
  compactions: number;
  sparkline: string;
  findings: ContextFinding[];
  advice: ContextAdvice[];
  precision: string; // "exact tokens" | "estimated tokens"
};

export type ContextOutcome = {
  status: 'ok' | 'ambiguous' | 'notfound' | 'unsupported';
  session?: SessionRow | null;
  profile?: ContextProfile | null;
  candidates?: SessionRow[] | null;
  notes: string[];
};

export type RelatedOutcome = {
  status: 'ok' | 'ambiguous' | 'notfound';
  session?: SessionRow | null;
  parent?: SessionRow | null;
  children?: SessionRow[] | null;
  adjacent?: SessionRow[] | null;
  candidates?: SessionRow[] | null;
  notes: string[];
};

export type SessionModelRow = {
  model: string;
  tokens: { input: number; output: number; reasoning: number; cache_read: number; cache_write: number; total: number };
};

export type SessionModelsOutcome = {
  status: 'ok' | 'ambiguous' | 'notfound';
  session?: SessionRow | null;
  rows?: SessionModelRow[] | null;
  candidates?: SessionRow[] | null;
  notes: string[];
};

export type TimelineEvent = {
  timestamp: string | null;
  agent: string;
  role: string;
  text: string;
  session: string; // full id — the viewer opens it directly
};

export type TimelineOutcome = {
  events: TimelineEvent[];
  notes: string[];
};

export type OptimizeFinding = {
  rule: string; // R6..R9 (SPEC-optimize.md)
  tool?: string;
  label?: string; // the recurring call; composed with desc in the UI
  desc: string;
  sessions: number;
  count: number;
  bytes: number;
  tokens?: number; // R9's exact-token exposure
};

export type OptimizeAdvice = {
  rule: string;
  text: string;
};

export type OptimizeReport = {
  sessions: number; // sessions analyzed
  findings: OptimizeFinding[];
  advice: OptimizeAdvice[];
  truncated: number; // smaller patterns beyond the top list
};

export type OptimizeOutcome = {
  report?: OptimizeReport | null;
  notes: string[];
};

export const api = {
  overview: bindings.Overview,
  diagnostics: bindings.Diagnostics,
  sessions: bindings.Sessions,
  search: bindings.Search,
  entries: bindings.Entries,
  context: bindings.Context,
  related: bindings.Related,
  sessionModels: bindings.SessionModels,
  timeline: bindings.Timeline,
  optimize: bindings.Optimize,
  stats: bindings.Stats,
  projects: bindings.Projects,
  projectStats: bindings.ProjectStats,
  cancel: bindings.Cancel,
  getSettings: bindings.GetSettings,
  setHome: bindings.SetHome,
  setTheme: bindings.SetTheme,
  setMaskSecrets: bindings.SetMaskSecrets,
  exportSession: bindings.ExportSession,
  exportSessions: bindings.ExportSessions,
  exportSearch: bindings.ExportSearch,
  exportStats: bindings.ExportStats,
  pickSavePath: bridge.PickSavePath,
  openExternal: bridge.OpenExternal,
} as const;

export type ProgressEvent = { op: string; scanned: number; hits: number };

export function onProgress(cb: (e: ProgressEvent) => void): () => void {
  EventsOn('progress', cb);
  return () => EventsOff('progress');
}
