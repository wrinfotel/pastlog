import { describe, expect, it } from 'vitest';
import { orderByDetection } from './agents';
import type { AgentRow } from './api';

function row(name: string, detected: boolean): AgentRow {
  return { name, detected, path: detected ? `C:\\dev\\${name}` : null, sessions: 0, bytes: 0 };
}

describe('orderByDetection', () => {
  it('floats detected agents above not-found ones', () => {
    const rows = [row('a', false), row('b', true), row('c', false), row('d', true)];
    expect(orderByDetection(rows).map((r) => r.name)).toEqual(['b', 'd', 'a', 'c']);
  });

  it('keeps registry order within each group (stable)', () => {
    const rows = [
      row('gemini-cli', false),
      row('claude-code', true),
      row('opencode', false),
      row('codex', true),
      row('zcode', true),
    ];
    expect(orderByDetection(rows).map((r) => r.name)).toEqual([
      'claude-code',
      'codex',
      'zcode',
      'gemini-cli',
      'opencode',
    ]);
  });

  it('passes through an all-detected list unchanged without mutating it', () => {
    const rows = [row('codex', true), row('zcode', true)];
    expect(orderByDetection(rows).map((r) => r.name)).toEqual(['codex', 'zcode']);
    expect(rows.map((r) => r.name)).toEqual(['codex', 'zcode']);
  });
});
