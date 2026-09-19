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

<h1>Settings</h1>

{#if settings}
  <section>
    <h2>Agent storage home</h2>
    <p class="meta">the directory holding the agent data (the CLI's --home). Empty = auto-discover.</p>
    <div class="row">
      <input type="text" bind:value={homeInput} placeholder="auto-discover" />
      <button class="btn" onclick={saveHome}>Save</button>
      <button class="btn" onclick={clearHome}>Clear</button>
    </div>
  </section>

  <section>
    <h2>Theme</h2>
    {#each ['system', 'dark', 'light'] as t}
      <label class="radio">
        <input
          type="radio"
          name="theme"
          checked={(settings.theme || 'system') === t}
          onclick={() => pickTheme(t as Theme)}
        />
        {t}
      </label>
    {/each}
  </section>

  <section>
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
    margin: 18px 0;
  }
  h2 {
    font-size: 15px;
    margin-bottom: 6px;
  }
  .row {
    display: flex;
    gap: 8px;
    max-width: 560px;
  }
  input[type='text'] {
    flex: 1;
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 7px 10px;
  }
  .btn {
    background: var(--panel);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 12px;
    cursor: pointer;
    font: inherit;
  }
  .btn:hover {
    border-color: var(--accent);
  }
  .radio {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 0;
    cursor: pointer;
  }
  .meta {
    color: var(--muted);
    font-size: 12px;
  }
  .ok {
    color: #4ade80;
  }
  .err {
    color: #f87171;
  }
</style>
