<script lang="ts">
  // Settings + About: the only knobs the app offers (spec §2.4) — the home
  // override (the CLI's --home equivalent) and the theme.
  import { api } from '../lib/api';
  import { applyTheme, type Theme } from '../lib/stores.svelte';

  type Settings = { home: string; theme: string; version: string; commit: string; date: string };

  let settings = $state<Settings | null>(null);
  let homeInput = $state('');
  let message = $state('');
  let error = $state('');

  $effect(() => {
    api.getSettings().then((s) => {
      settings = s as Settings;
      homeInput = (s as Settings).home;
    });
  });

  async function saveHome() {
    error = '';
    message = '';
    try {
      settings = (await api.setHome(homeInput)) as Settings;
      message = 'home override saved';
    } catch (e) {
      error = String(e);
    }
  }

  async function clearHome() {
    error = '';
    message = '';
    try {
      settings = (await api.setHome('')) as Settings;
      homeInput = settings.home ?? '';
      message = 'home override cleared — auto-discovery';
    } catch (e) {
      error = String(e);
    }
  }

  async function pickTheme(t: Theme) {
    try {
      settings = (await api.setTheme(t || 'system')) as Settings;
      applyTheme(t);
      error = '';
    } catch (e) {
      error = String(e);
    }
  }
</script>

<header class="page-head">
  <div>
    <div class="kicker">// 07 · SETTINGS</div>
    <h1>Settings</h1>
    <p class="lede">Control local storage discovery, appearance, and application details.</p>
  </div>
  <div class="page-mark">PREFERENCES<br /><strong>ON THIS DEVICE</strong></div>
</header>

{#if settings}
  <section class="panel">
    <div class="section-kicker">// 01 · STORAGE</div>
    <h2>Agent storage home</h2>
    <p class="meta">the directory holding the agent data (the CLI's --home). Empty = auto-discover.</p>
    <div class="row">
      <input type="text" bind:value={homeInput} placeholder="auto-discover" />
      <button class="btn primary" onclick={saveHome}>Save</button>
      <button class="btn" onclick={clearHome}>Clear</button>
    </div>
  </section>

  <section class="panel">
    <div class="section-kicker">// 02 · APPEARANCE</div>
    <h2>Theme</h2>
    <div class="seg">
      {#each ['system', 'dark', 'light'] as t}
        <label class="segopt">
          <input
            type="radio"
            name="theme"
            checked={(settings.theme || 'system') === t}
            onclick={() => pickTheme(t as Theme)}
          />
          {t}
        </label>
      {/each}
    </div>
  </section>

  <section class="panel">
    <div class="section-kicker">// 03 · ABOUT</div>
    <h2>About</h2>
    <p class="meta">
      pastlog Desktop {settings.version} (commit {settings.commit}, date {settings.date})<br />
      the windowed twin of the pastlog CLI — same engine, same guarantees:
      100% local, read-only, zero telemetry.
    </p>
  </section>

  {#if message}<p class="ok">{message}</p>{/if}
  {#if error}<p class="err">{error}</p>{/if}
{/if}

<style>
  section {
    margin: 0 0 14px;
    padding: 16px 18px;
    max-width: 640px;
  }
  .section-kicker { margin-bottom: 6px; color: var(--accent); font: 600 10px var(--mono); letter-spacing: 0.06em; }
  section .meta {
    margin: 0 0 12px;
  }
  .row {
    display: flex;
    gap: 8px;
  }
  .row input {
    flex: 1;
  }
  .seg {
    display: inline-flex;
    gap: 4px;
    padding: 4px;
    background: var(--inset);
    border: 1px solid var(--border);
    border-radius: 999px;
  }
  .segopt {
    display: inline-flex;
    align-items: center;
    padding: 5px 16px;
    border-radius: 999px;
    color: var(--muted);
    font-size: 12.5px;
    cursor: pointer;
    user-select: none;
    transition:
      color var(--speed) ease,
      background var(--speed) ease,
      box-shadow var(--speed) ease;
  }
  .segopt:hover {
    color: var(--text);
  }
  .segopt:has(input:checked) {
    color: var(--accent);
    background: var(--panel);
    font-weight: 600;
    box-shadow:
      var(--hairline),
      var(--shadow-1),
      0 0 0 1px var(--accent-soft);
  }
  .segopt input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .segopt:has(input:focus-visible) {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
</style>
