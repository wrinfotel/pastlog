// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './markdown';

describe('renderMarkdown (untrusted-content policy, spec §4.4)', () => {
  it('renders simple markdown', () => {
    expect(renderMarkdown('# Hi\n\nsome **bold** text')).toContain('<h1>Hi</h1>');
    expect(renderMarkdown('some **bold** text')).toContain('<strong>bold</strong>');
  });

  it('strips script tags', () => {
    const out = renderMarkdown('hello <script>alert(1)</script> world');
    expect(out).not.toContain('<script');
    expect(out).not.toContain('alert(1)');
  });

  it('strips event handlers', () => {
    const out = renderMarkdown('<em onmouseover="alert(1)">x</em>');
    expect(out).not.toContain('onmouseover');
  });

  it('strips javascript: links but keeps https links', () => {
    const evil = renderMarkdown('[x](javascript:alert(1))');
    expect(evil).not.toContain('javascript:');
    const ok = renderMarkdown('[docs](https://example.com)');
    expect(ok).toContain('href="https://example.com"');
  });

  it('never emits img/style/iframe from log content', () => {
    const out = renderMarkdown('<img src=x onerror=alert(1)><style>body{}</style><iframe src="https://x"></iframe>');
    expect(out).not.toContain('<img');
    expect(out).not.toContain('<style');
    expect(out).not.toContain('<iframe');
  });
});
