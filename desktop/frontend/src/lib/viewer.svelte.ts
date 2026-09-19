// Session viewer navigation state: which session to open, and (from search)
// which hit line to scroll to and highlight.

export const viewer = $state({ id: '', hitLine: '', hitKind: '', hitRole: '' });

export function openSession(id: string, hit?: { line: string; kind: string; role: string }) {
  viewer.id = id;
  viewer.hitLine = hit?.line ?? '';
  viewer.hitKind = hit?.kind ?? '';
  viewer.hitRole = hit?.role ?? '';
}
