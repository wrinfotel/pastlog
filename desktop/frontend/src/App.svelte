<script lang="ts">
  // App shell: left navigation, top bar, view switch, theme bootstrap, and the
  // global link policy — external links open in the system browser, never here
  // (spec §4.4).
  import { api } from './lib/api';
  import { appMeta, applyTheme, go, view, type ViewName } from './lib/stores.svelte';
  import Home from './views/Home.svelte';
  import ProjectsView from './views/ProjectsView.svelte';
  import SearchView from './views/SearchView.svelte';
  import SessionsView from './views/SessionsView.svelte';
  import StatsView from './views/StatsView.svelte';
  import Viewer from './views/Viewer.svelte';
  import DiagnosticsView from './views/DiagnosticsView.svelte';
  import SettingsView from './views/SettingsView.svelte';

  // Mono hints on the right of each item are cosmetic markers, not shortcuts.
  const navGroups: { label: string; items: { name: ViewName; label: string; hint: string; hidden?: boolean }[] }[] = [
    {
      label: 'Browse',
      items: [
        { name: 'home', label: 'Home', hint: '~' },
        { name: 'search', label: 'Search', hint: 'find' },
        // kept out of the sidebar for now; the view itself stays reachable and
        // the viewer's back button can still return to it
        { name: 'sessions', label: 'Sessions', hint: 'log', hidden: true },
      ],
    },
    {
      label: 'Analyze',
      items: [
        { name: 'stats', label: 'Stats', hint: 'calc' },
        { name: 'diagnostics', label: 'Diagnostics', hint: 'sys' },
        { name: 'settings', label: 'Settings', hint: 'conf' },
      ],
    },
  ];

  let home = $state('');

  $effect(() => {
    api.getSettings().then((s) => {
      applyTheme((s.theme || 'system') as never);
      home = s.home ?? '';
      appMeta.version = s.version ?? '';
    });
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
  <aside>
    <div class="brand">
      <span class="logo" aria-hidden="true">&gt;_</span>
      <span class="word">pastlog</span>
      <span class="edition">DESKTOP</span>
    </div>
    {#each navGroups as group}
      <div class="group">
        <div class="group-label">{group.label}</div>
        {#each group.items as item}
          {#if !item.hidden}
            <button
              class:active={view.current === item.name}
              onclick={() => go(item.name)}
            >
              <span class="lead"><span class="nav-dot" class:on={view.current === item.name}></span>{item.label}</span>
              <span class="hint">{item.hint}</span>
            </button>
          {/if}
        {/each}
      </div>
    {/each}
    <div class="ws">
      <div class="ws-row">
        <span class="ws-name"><span class="dot"></span>Local workspace</span>
        <span class="ro">
          <svg viewBox="0 0 24 24" width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="11" width="18" height="11" rx="2" />
            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
          </svg>
          READ-ONLY
        </span>
      </div>
      <div class="ws-path">
        <span>{home || 'auto-discovery'}</span>
        <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
        </svg>
      </div>
    </div>
  </aside>
  <div class="frame">
    <header class="topbar">
      <div class="crumb"><b>ISSUE 01</b><span>·</span><em>Local Archive</em></div>
      <div class="status"><span class="dot"></span>100% local · read-only · zero config</div>
      {#if appMeta.version}<span class="ver">pastlog v{appMeta.version}</span>{/if}
    </header>
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
</div>

<style>
  .app {
    display: flex;
    height: 100vh;
  }
  aside {
    width: 264px;
    flex-shrink: 0;
    background: var(--bg-deep);
    border-right: 1px solid var(--border-subtle);
    padding: 18px 14px 14px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 30px;
    padding: 0 4px;
  }
  .logo {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    border: 1px solid var(--border-subtle);
    color: var(--accent);
    font: 600 11px var(--mono);
    letter-spacing: -0.03em;
  }
  .word {
    font-size: 15px;
    font-weight: 650;
    letter-spacing: -0.01em;
  }
  .edition {
    margin-left: auto;
    padding: 2px 7px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--accent);
    font: 600 9px var(--mono);
    letter-spacing: 0.12em;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .group-label {
    padding: 0 10px 6px;
    color: var(--faint);
    font: 600 9.5px var(--mono);
    letter-spacing: 0.14em;
    text-transform: uppercase;
  }
  aside button {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    text-align: left;
    background: none;
    border: 0;
    color: var(--text-2);
    padding: 7px 10px;
    border-radius: var(--radius-s);
    font-size: 13px;
    font-weight: 500;
    transition:
      color var(--speed) ease,
      background var(--speed) ease;
  }
  aside button:hover {
    color: var(--text);
    background: var(--panel-hover);
  }
  aside button.active {
    color: var(--text);
    background: var(--panel-hover);
  }
  aside button .lead {
    display: flex;
    align-items: center;
    gap: 9px;
  }
  .nav-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: transparent;
    border: 1px solid var(--faint);
  }
  .nav-dot.on {
    background: var(--accent-vivid);
    border-color: var(--accent-vivid);
    box-shadow: 0 0 8px var(--accent-glow);
  }
  .hint {
    color: var(--faint);
    font: 500 10px var(--mono);
  }
  aside button:hover .hint {
    color: var(--muted);
  }
  aside button.active .hint {
    color: var(--accent);
  }
  .ws {
    margin-top: auto;
    display: grid;
    gap: 8px;
    padding: 12px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius);
    background: var(--panel);
  }
  .ws-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .ws-name {
    display: flex;
    align-items: center;
    gap: 7px;
    color: var(--text-2);
    font: 500 11px var(--mono);
    white-space: nowrap;
  }
  .ro {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 6px;
    border-radius: var(--radius-s);
    background: var(--panel-hover);
    color: var(--muted);
    font: 600 8.5px var(--mono);
    letter-spacing: 0.1em;
  }
  .ws-path {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 8px;
    border-radius: var(--radius-s);
    background: var(--inset);
    color: var(--muted);
  }
  .ws-path span {
    font: 10px var(--mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .ws-path svg {
    flex-shrink: 0;
    opacity: 0.7;
  }
  .frame {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .topbar {
    height: 50px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 18px;
    padding: 0 clamp(20px, 3vw, 40px);
    background: color-mix(in srgb, var(--bg) 88%, transparent);
    backdrop-filter: blur(10px);
    border-bottom: 1px solid var(--border-subtle);
    overflow: hidden;
  }
  .crumb {
    display: flex;
    align-items: center;
    gap: 8px;
    font: 500 11px var(--mono);
    white-space: nowrap;
  }
  .crumb b {
    color: var(--accent);
    font-weight: 600;
  }
  .crumb span {
    color: var(--faint);
  }
  .crumb em {
    font-style: normal;
    color: var(--text-2);
  }
  .status {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 3px 10px;
    border-radius: 999px;
    background: var(--bg-deep);
    color: var(--muted);
    font: 500 10.5px var(--mono);
    white-space: nowrap;
  }
  .ver {
    margin-left: auto;
    color: var(--faint);
    font: 500 10.5px var(--mono);
    white-space: nowrap;
  }
  main {
    flex: 1;
    overflow-y: auto;
    width: 100%;
    max-width: 1120px;
    margin: 0 auto;
    padding: 26px clamp(20px, 3.5vw, 44px) 56px;
  }
</style>
