import { describe, expect, it } from 'vitest';
import { window2 } from './virtual';

describe('window2', () => {
  const rows = 10_000;
  const itemH = 30;

  it('starts at the top', () => {
    // 600/30 = 20 visible rows + 2*6 overscan
    expect(window2(0, 600, rows, itemH)).toEqual({ start: 0, end: 32 });
  });

  it('slides with scroll and overscans both ends', () => {
    const w = window2(3000, 600, rows, itemH); // row 100 at top
    expect(w.start).toBe(100 - 6);
    expect(w.end).toBeGreaterThan(100);
  });

  it('clamps at the end', () => {
    const w = window2(10_000 * itemH, 600, rows, itemH);
    expect(w.end).toBe(rows);
    expect(w.start).toBeLessThan(rows);
  });

  it('handles empty and degenerate inputs', () => {
    expect(window2(0, 600, 0, itemH)).toEqual({ start: 0, end: 0 });
    expect(window2(0, 600, rows, 0)).toEqual({ start: 0, end: 0 });
  });

  it('handles tiny lists', () => {
    const w = window2(0, 600, 5, itemH);
    expect(w.start).toBe(0);
    expect(w.end).toBe(5);
  });
});
