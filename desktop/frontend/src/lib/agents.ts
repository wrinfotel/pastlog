import type { AgentRow } from './api';

// orderByDetection floats detected agents above not-found ones for the Home
// grid while keeping the registry order inside each group — the backend list
// order is a registry order, so without this the "NOT FOUND" cards can sit
// above live ones. Sort is stable (ES2019+), so ties never reshuffle.
export function orderByDetection(rows: AgentRow[]): AgentRow[] {
  return [...rows].sort((a, b) => Number(b.detected) - Number(a.detected));
}
