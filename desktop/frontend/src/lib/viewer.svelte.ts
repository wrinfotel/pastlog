// Session viewer navigation state: which session to open, where it was
// opened from (the viewer's back button returns there), and (from search)
// which hit to scroll to and highlight.
import type { ViewName } from './stores.svelte';

export const viewer = $state({ id: '', hitHead: '', hitKind: '', hitRole: '', from: 'sessions' as ViewName });

// from is omitted when re-opening another candidate from inside the viewer:
// the origin stays whatever it was. hit carries the backend's viewer anchor
// (entry_head): the head of the hit entry's full text, which the transcript
// entry matches verbatim — the display snippet is windowed/synthesized and
// cannot.
export function openSession(id: string, hit?: { head: string; kind: string; role: string }, from?: ViewName) {
  viewer.id = id;
  viewer.hitHead = hit?.head ?? '';
  viewer.hitKind = hit?.kind ?? '';
  viewer.hitRole = hit?.role ?? '';
  if (from) viewer.from = from;
}
