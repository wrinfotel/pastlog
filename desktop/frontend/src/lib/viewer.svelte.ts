// Session viewer navigation state: which session to open, where it was
// opened from (the viewer's back button returns there), and (from search)
// which hit line to scroll to and highlight.
import type { ViewName } from './stores.svelte';

export const viewer = $state({ id: '', hitLine: '', hitKind: '', hitRole: '', from: 'sessions' as ViewName });

// from is omitted when re-opening another candidate from inside the viewer:
// the origin stays whatever it was.
export function openSession(id: string, hit?: { line: string; kind: string; role: string }, from?: ViewName) {
  viewer.id = id;
  viewer.hitLine = hit?.line ?? '';
  viewer.hitKind = hit?.kind ?? '';
  viewer.hitRole = hit?.role ?? '';
  if (from) viewer.from = from;
}
