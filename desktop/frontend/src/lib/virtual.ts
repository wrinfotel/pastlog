// Virtual-list windowing math, kept pure for tests: which slice of `count`
// rows is visible for a scroll position, with overscan padding on both ends.
export function window2(
  scrollTop: number,
  viewportH: number,
  count: number,
  itemH: number,
  overscan = 6,
): { start: number; end: number } {
  if (count <= 0 || itemH <= 0) return { start: 0, end: 0 };
  const start = Math.max(0, Math.min(count, Math.floor(scrollTop / itemH) - overscan));
  const visible = Math.ceil(viewportH / itemH) + 2 * overscan;
  const end = Math.min(count, start + visible);
  return { start, end };
}
