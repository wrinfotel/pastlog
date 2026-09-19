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
};

export type ListOutcome = {
  sessions: SessionRow[];
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

export const api = {
  overview: bindings.Overview,
  diagnostics: bindings.Diagnostics,
  sessions: bindings.Sessions,
  search: bindings.Search,
  entries: bindings.Entries,
  stats: bindings.Stats,
  projects: bindings.Projects,
  projectStats: bindings.ProjectStats,
  cancel: bindings.Cancel,
  getSettings: bindings.GetSettings,
  setHome: bindings.SetHome,
  setTheme: bindings.SetTheme,
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
