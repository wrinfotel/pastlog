<script lang="ts">
  // App shell: left navigation, view switch, theme bootstrap, and the global
  // link policy — external links open in the system browser, never here
  // (spec §4.4).
  import { api } from './lib/api';
  import { applyTheme, go, view, type ViewName } from './lib/stores.svelte';
  import Home from './views/Home.svelte';
  import ProjectsView from './views/ProjectsView.svelte';
  import SearchView from './views/SearchView.svelte';
  import SessionsView from './views/SessionsView.svelte';
  import StatsView from './views/StatsView.svelte';
  import Viewer from './views/Viewer.svelte';
  import DiagnosticsView from './views/DiagnosticsView.svelte';
  import SettingsView from './views/SettingsView.svelte';

  const nav: { name: ViewName; label: string }[] = [
    { name: 'home', label: 'Home' },
    { name: 'search', label: 'Search' },
    { name: 'sessions', label: 'Sessions' },
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
    <div class="brand">pastlog</div>
    {#each nav as item}
      <button class:active={view.current === item.name} onclick={() => go(item.name)}>
        {item.label}
      </button>
    {/each}
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
    width: 150px;
    background: var(--panel);
    border-right: 1px solid var(--border);
    padding: 14px 10px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .brand {
    font-weight: 700;
    color: var(--accent);
    padding: 2px 8px 12px;
  }
  nav button {
    text-align: left;
    background: none;
    border: 0;
    color: var(--text);
    padding: 7px 8px;
    border-radius: 6px;
    cursor: pointer;
    font: inherit;
  }
  nav button:hover {
    background: var(--bg);
  }
  nav button.active {
    background: var(--bg);
    color: var(--accent);
    font-weight: 600;
  }
  main {
    flex: 1;
    overflow-y: auto;
    padding: 18px 22px;
  }
</style>
