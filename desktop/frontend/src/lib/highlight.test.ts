import { describe, expect, it } from 'vitest';
import { splitMatch } from './highlight';

describe('splitMatch', () => {
  it('splits ascii lines around the match', () => {
    expect(splitMatch('hello jwt world', 6, 9)).toEqual(['hello ', 'jwt', ' world']);
  });

  it('handles CJK correctly (rune offsets, not UTF-16 units)', () => {
    const line = 'Hello World';
    // runes: H e l l o ' ' 世 界 で す — match at runes 3..5
    expect(splitMatch(line, 3, 5)).toEqual(['Hel', 'lo', ' World']);
  });

  it('handles emoji (surrogate pairs count as single runes)', () => {
    const line = 'a😀b';
    // rune 1 = the emoji (UTF-16 length 2)
    expect(splitMatch(line, 1, 2)).toEqual(['a', '😀', 'b']);
  });

  it('clamps degenerate offsets', () => {
    expect(splitMatch('abc', -5, 99)).toEqual(['', 'abc', '']);
    expect(splitMatch('abc', 2, 2)).toEqual(['ab', '', 'c']);
    expect(splitMatch('abc', 5, 2)).toEqual(['abc', '', '']);
  });
});
