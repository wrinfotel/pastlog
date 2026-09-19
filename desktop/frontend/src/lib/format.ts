// Presentation helpers. Number/byte formats mirror the CLI's render package
// so the GUI reads like the same tool (HumanBytes, thousands separators).

export function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
  let v = n;
  for (const u of units) {
    v /= 1024;
    if (v < 1024) return `${v.toFixed(1)} ${u}`;
  }
  return `${v.toFixed(1)} EB`;
}

const pad = (n: number) => String(n).padStart(2, '0');

export function fmtDate(iso: string | null): string {
  if (!iso) return '-';
  const d = new Date(iso);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fmtTime(iso: string | null): string {
  if (!iso) return '-';
  const d = new Date(iso);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function fmtDay(iso: string | null): string {
  if (!iso) return '-';
  const d = new Date(iso);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function fmtInt(n: number): string {
  return n.toLocaleString('en-US');
}

// shortProject mirrors render.shortProject: the last two path components.
export function shortProject(project: string): string {
  const parts = project.replaceAll('\\', '/').split('/').filter((p) => p !== '');
  if (parts.length === 0) return '-';
  if (parts.length === 1) return parts[0];
  return `${parts[parts.length - 2]}/${parts[parts.length - 1]}`;
}

// idPrefix mirrors the CLI's 8-char session id prefix.
export function idPrefix(id: string): string {
  return id.length <= 8 ? id : id.slice(0, 8);
}
