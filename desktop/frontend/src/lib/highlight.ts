// Splits a match line into before/match/after around RUNE offsets. The JSON
// schema carries rune offsets (render.SearchJSON), while JS strings index by
// UTF-16 units — Array.from iterates code points, which matches runes for
// all non-surrogate text and keeps emoji/CJK highlights correct.
export function splitMatch(line: string, start: number, end: number): [string, string, string] {
  const chars = Array.from(line);
  const s = Math.max(0, Math.min(start, chars.length));
  const e = Math.max(s, Math.min(end, chars.length));
  return [chars.slice(0, s).join(''), chars.slice(s, e).join(''), chars.slice(e).join('')];
}
