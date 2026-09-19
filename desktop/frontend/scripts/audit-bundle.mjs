// Bundle audit: the packaged frontend must load zero external assets
// (TASK-DESKTOP.md §2.2). Scans the built dist/ for external-resource
// patterns — script/style/img src, hyperlinks, CSS url(), fetch and dynamic
// import with an absolute http(s) URL. Plain "https?://" occurrences inside
// JS string literals (license text, regex constants) are not asset loads,
// so the patterns stay narrow and purposeful.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const dist = new URL('../dist', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');

const patterns = [
  [/\b(?:src|href)\s*=\s*["']https?:\/\//i, 'external src/href attribute'],
  [/url\(\s*["']?https?:\/\//i, 'external CSS url()'],
  [/fetch\(\s*["'`]https?:\/\//i, 'external fetch()'],
  [/import\(\s*["']https?:\/\//i, 'external dynamic import'],
];

function walk(dir) {
  let out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) out = out.concat(walk(p));
    else out.push(p);
  }
  return out;
}

let files;
try {
  files = walk(dist);
} catch {
  console.error(`audit:bundle: cannot read ${dist} — run "npm run build" first`);
  process.exit(2);
}

const offenders = [];
for (const file of files) {
  if (!/\.(html|css|js|mjs)$/.test(file)) continue;
  const text = readFileSync(file, 'utf8');
  const lines = text.split('\n');
  for (const [re, why] of patterns) {
    lines.forEach((line, i) => {
      if (re.test(line)) offenders.push(`${relative(dist, file)}:${i + 1}  ${why}  ${line.trim().slice(0, 120)}`);
    });
  }
}

if (offenders.length > 0) {
  console.error(`audit:bundle: external asset references found (${offenders.length}):\n` + offenders.join('\n'));
  process.exit(1);
}
console.log(`audit:bundle: clean — no external asset references in ${files.length} files`);
