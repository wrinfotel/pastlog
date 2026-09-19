import { describe, expect, it } from 'vitest';
import { fmtBytes, fmtDay, fmtInt, idPrefix, shortProject } from './format';

describe('fmtBytes', () => {
  it('mirrors render.HumanBytes', () => {
    expect(fmtBytes(0)).toBe('0 B');
    expect(fmtBytes(512)).toBe('512 B');
    expect(fmtBytes(2048)).toBe('2.0 KB');
    expect(fmtBytes(1024 * 1024 * 2.4)).toBe('2.4 MB');
    expect(fmtBytes(1024 * 1024 * 1024 * 1.4)).toBe('1.4 GB');
  });
});

describe('fmtDay', () => {
  it('renders null as dash', () => {
    expect(fmtDay(null)).toBe('-');
  });
  it('formats a local day', () => {
    expect(fmtDay('2026-08-02T14:03:22.150Z')).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

describe('fmtInt', () => {
  it('groups thousands like the CLI', () => {
    expect(fmtInt(1234567)).toBe('1,234,567');
  });
});

describe('shortProject', () => {
  it('keeps the last two components', () => {
    expect(shortProject('/home/dev/myapp/api')).toBe('myapp/api');
    expect(shortProject('C:\\dev\\myapp')).toBe('dev/myapp');
    expect(shortProject('solo')).toBe('solo');
    expect(shortProject('')).toBe('-');
  });
});

describe('idPrefix', () => {
  it('cuts at 8 chars like the CLI', () => {
    expect(idPrefix('3f9c81a2-1111')).toBe('3f9c81a2');
    expect(idPrefix('short')).toBe('short');
  });
});
