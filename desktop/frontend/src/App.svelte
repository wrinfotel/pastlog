<script lang="ts">
  // App shell: left navigation, view switch, theme bootstrap, and the global
  // link policy — external links open in the system browser, never here
  // (spec §4.4).
  import { api } from './lib/api';
  import { applyTheme, go, view, type ViewName } from './lib/stores.svelte';
  import { openProjects } from './lib/projects.svelte';
  import Home from './views/Home.svelte';
  import ProjectsView from './views/ProjectsView.svelte';
  import SearchView from './views/SearchView.svelte';
  import SessionsView from './views/SessionsView.svelte';
  import StatsView from './views/StatsView.svelte';
  import Viewer from './views/Viewer.svelte';
  import DiagnosticsView from './views/DiagnosticsView.svelte';
  import SettingsView from './views/SettingsView.svelte';

  // 16px stroke icons (feather-style geometry) — no icon dependency.
  const icons = {
    home: '<path d="M3 9.5 12 2l9 7.5"/><path d="M5 8.5V21a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8.5"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
    sessions: '<polygon points="12 2 2 7 12 12 22 7 12 2"/><polyline points="2 17 12 22 22 17"/><polyline points="2 12 12 17 22 12"/>',
    projects: '<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>',
    stats: '<line x1="6" y1="20" x2="6" y2="14"/><line x1="12" y1="20" x2="12" y2="8"/><line x1="18" y1="20" x2="18" y2="4"/>',
    diagnostics: '<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>',
    settings:
      '<line x1="4" y1="21" x2="4" y2="14"/><line x1="4" y1="10" x2="4" y2="3"/><line x1="12" y1="21" x2="12" y2="12"/><line x1="12" y1="8" x2="12" y2="3"/><line x1="20" y1="21" x2="20" y2="16"/><line x1="20" y1="12" x2="20" y2="3"/><line x1="1" y1="14" x2="7" y2="14"/><line x1="9" y1="8" x2="15" y2="8"/><line x1="17" y1="16" x2="23" y2="16"/>',
  } as const;

  const nav: { name: ViewName; label: string; hidden?: boolean }[] = [
    { name: 'home', label: 'Home' },
    { name: 'search', label: 'Search' },
    // kept out of the sidebar for now; the view itself stays reachable and
    // the viewer's back button can still return to it
    { name: 'sessions', label: 'Sessions', hidden: true },
    { name: 'projects', label: 'Projects' },
    { name: 'stats', label: 'Stats' },
    { name: 'diagnostics', label: 'Diagnostics' },
    { name: 'settings', label: 'Settings' },
  ];

  $effect(() => {
    api.getSettings().then((s) => applyTheme((s.theme || 'system') as never));
  });

  $effect(() => {
    // Untrusted-content policy: links found in log content are clicks into
    // the OS browser via the runtime bridge, nothing loads inside the app.
    function onClick(e: MouseEvent) {
      const a = (e.target as HTMLElement | null)?.closest?.('a');
      if (a instanceof HTMLAnchorElement && /^https?:/i.test(a.href)) {
        e.preventDefault();
        api.openExternal(a.href);
      }
    }
    document.addEventListener('click', onClick);
    return () => document.removeEventListener('click', onClick);
  });
</script>

<div class="app">
  <nav>
    <div class="brand">
      <span class="logo" aria-hidden="true">
        <svg viewBox="0 0 16 16" width="15" height="15">
          <path d="M3 4.5 6.5 8 3 11.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
          <line x1="8.5" y1="11.5" x2="13" y2="11.5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" />
        </svg>
      </span>
      <span class="word">pastlog<em>desktop</em></span>
    </div>
    {#each nav as item}
      {#if !item.hidden}
        <button
          class:active={view.current === item.name}
          onclick={() => {
            // the sidebar always lands on a section root: a drilled-in project
            // detail is restored only via the viewer's back button
            if (item.name === 'projects') openProjects();
            go(item.name);
          }}
        >
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
            {@html icons[item.name]}
          </svg>
          {item.label}
        </button>
      {/if}
    {/each}
    <div class="foot">
      <svg viewBox="0 0 24 24" width="11" height="11" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <rect x="3" y="11" width="18" height="11" rx="2" />
        <path d="M7 11V7a5 5 0 0 1 10 0v4" />
      </svg>
      local &middot; read-only
    </div>
  </nav>
  <main>
    {#if view.current === 'home'}
      <Home />
    {:else if view.current === 'search'}
      <SearchView />
    {:else if view.current === 'sessions'}
      <SessionsView />
    {:else if view.current === 'projects'}
      <ProjectsView />
    {:else if view.current === 'stats'}
      <StatsView />
    {:else if view.current === 'viewer'}
      <Viewer />
    {:else if view.current === 'diagnostics'}
      <DiagnosticsView />
    {:else}
      <SettingsView />
    {/if}
  </main>
</div>

<style>
  .app {
    display: flex;
    height: 100vh;
  }
  nav {
    width: 188px;
    flex-shrink: 0;
    background: var(--bg-deep);
    border-right: 1px solid var(--border);
    padding: 16px 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 2px 6px 14px;
    margin-bottom: 6px;
    border-bottom: 1px solid var(--border-subtle);
  }
  .logo {
    display: grid;
    place-items: center;
    width: 27px;
    height: 27px;
    border-radius: 8px;
    color: var(--accent-ink);
    background: linear-gradient(135deg, var(--accent) 0%, #0e7490 130%);
    box-shadow:
      0 0 14px var(--accent-glow),
      inset 0 1px 0 rgba(255, 255, 255, 0.25);
  }
  .word {
    font-size: 14.5px;
    font-weight: 700;
    letter-spacing: -0.01em;
    display: flex;
    flex-direction: column;
    line-height: 1.15;
  }
  .word em {
    font-style: normal;
    font-size: 9.5px;
    font-weight: 600;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--muted);
  }
  nav button {
    display: flex;
    align-items: center;
    gap: 10px;
    text-align: left;
    background: none;
    border: 0;
    color: var(--text-2);
    padding: 7px 10px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    transition:
      color var(--speed) ease,
      background var(--speed) ease;
  }
  nav button:hover {
    color: var(--text);
    background: var(--panel);
  }
  nav button.active {
    color: var(--accent);
    background: var(--accent-soft);
  }
  nav button svg {
    flex-shrink: 0;
    opacity: 0.85;
  }
  .foot {
    margin-top: auto;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 6px 2px;
    border-top: 1px solid var(--border-subtle);
    color: var(--muted);
    font-size: 10.5px;
    letter-spacing: 0.02em;
  }
  main {
    flex: 1;
    overflow-y: auto;
    padding: 22px 28px 34px;
  }
</style>
