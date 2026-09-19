// Sanitized markdown for assistant messages (spec §4.4): session logs are
// untrusted input. marked renders, DOMPurify strips everything that could
// execute or load remote content. Links stay clickable but the global
// interceptor sends them to the system browser — nothing loads in-app.
import { marked } from 'marked';
import DOMPurify from 'dompurify';

const purify = DOMPurify();

export function renderMarkdown(src: string): string {
  const html = marked.parse(src ?? '', { async: false, gfm: true, breaks: false }) as string;
  return purify.sanitize(html, {
    ALLOWED_TAGS: [
      'p', 'br', 'strong', 'em', 'code', 'pre', 'ul', 'ol', 'li', 'blockquote',
      'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'a', 'table', 'thead', 'tbody',
      'tr', 'th', 'td', 'hr', 'span', 'del',
    ],
    ALLOWED_ATTR: ['href', 'title'],
    FORBID_TAGS: ['style', 'script', 'iframe', 'object', 'embed', 'form', 'img'],
    FORBID_ATTR: ['style', 'srcset', 'onerror', 'onclick'],
  });
}
