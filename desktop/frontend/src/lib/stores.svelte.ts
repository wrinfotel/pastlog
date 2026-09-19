// App state stores (Svelte 5 runes at module scope live in .svelte.ts).

export type ViewName = 'home' | 'search' | 'sessions' | 'stats' | 'viewer' | 'diagnostics' | 'settings';

export const view = $state({ current: 'home' as ViewName });

export function go(v: ViewName) {
  view.current = v;
}

export type Theme = '' | 'system' | 'dark' | 'light';

export const theme = $state({ value: '' as Theme });

export function applyTheme(t: Theme) {
  theme.value = t;
  const root = document.documentElement;
  if (t === 'dark' || t === 'light') root.dataset.theme = t;
  else delete root.dataset.theme;
}
