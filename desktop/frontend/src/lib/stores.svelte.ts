// App state stores (Svelte 5 runes at module scope live in .svelte.ts).

export type ViewName = 'home' | 'projects' | 'search' | 'sessions' | 'stats' | 'viewer' | 'diagnostics' | 'settings';

export const view = $state({ current: 'home' as ViewName });

// Build metadata, filled once by the shell from getSettings — headers stamp
// the version without each view re-fetching it.
export const appMeta = $state({ version: '' });

export function go(v: ViewName) {
  view.current = v;
}

export type Theme = '' | 'system' | 'dark' | 'light';

export const theme = $state({ value: '' as Theme });

export function applyTheme(t: Theme) {
  theme.value = t;
  const root = document.documentElement;
  if (t === 'system') {
    // the OS scheme is resolved here so an explicit 'light' choice works on a
    // dark-OS machine too; the listener keeps 'system' live across OS changes
    const media = window.matchMedia('(prefers-color-scheme: light)');
    const resolve = () => (root.dataset.theme = media.matches ? 'light' : 'dark');
    resolve();
    media.onchange = () => {
      if (theme.value === 'system') resolve();
    };
  } else {
    root.dataset.theme = t;
  }
}
