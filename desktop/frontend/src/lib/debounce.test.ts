import { describe, expect, it, vi } from 'vitest';
import { debounce } from './debounce';

describe('debounce', () => {
  it('collapses bursts into one call (~250 ms keystroke debounce)', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const d = debounce(fn, 250);
    d('a');
    vi.advanceTimersByTime(100);
    d('b');
    vi.advanceTimersByTime(100);
    d('c');
    vi.advanceTimersByTime(249);
    expect(fn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenCalledWith('c');
    vi.useRealTimers();
  });
});
