<script>
  import { onMount } from 'svelte';
  import { currentPage, connected, i18n } from './lib/stores.js';
  import { healthCheck } from './lib/api.js';
  import Sidebar from './components/Sidebar.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import Tables from './pages/Tables.svelte';
  import SQLQuery from './pages/SQLQuery.svelte';
  import Sessions from './pages/Sessions.svelte';
  import Memories from './pages/Memories.svelte';
  import Decisions from './pages/Decisions.svelte';

  let page = $state('dashboard');
  let ok = $state(false);

  currentPage.subscribe(v => page = v);

  const pageTitle = $derived(() => {
    const map = {
      dashboard: i18n.t('nav.dashboard'),
      tables: i18n.t('nav.tables'),
      sql: i18n.t('nav.sql'),
      sessions: i18n.t('nav.sessions'),
      memories: i18n.t('nav.memories'),
      decisions: i18n.t('nav.decisions'),
    };
    return map[page] || page;
  });

  onMount(async () => {
    await checkHealth();
    const t = setInterval(checkHealth, 5000);
    return () => clearInterval(t);
  });

  async function checkHealth() {
    try {
      await healthCheck();
      ok = true;
      connected.set(true);
    } catch {
      ok = false;
      connected.set(false);
    }
  }
</script>

<div class="app-layout">
  <Sidebar />
  <main class="main-content">
    <header class="top-bar">
      <div class="top-bar-left">
        <h1 class="page-title">{pageTitle()}</h1>
      </div>
      <div class="top-bar-right">
        <button class="lang-toggle" onclick={() => i18n.toggle()}>
          {i18n.locale === 'zh' ? 'EN' : '中'}
        </button>
        <div class="status-indicator" class:online={ok}>
          <span class="status-dot"></span>
          <span class="status-text">{ok ? i18n.t('status.connected') : i18n.t('status.disconnected')}</span>
        </div>
      </div>
    </header>
    <div class="page-content">
      {#if page === 'dashboard'}
        <Dashboard />
      {:else if page === 'tables'}
        <Tables />
      {:else if page === 'sql'}
        <SQLQuery />
      {:else if page === 'sessions'}
        <Sessions />
      {:else if page === 'memories'}
        <Memories />
      {:else if page === 'decisions'}
        <Decisions />
      {/if}
    </div>
  </main>
</div>

<style>
  .app-layout {
    display: flex;
    height: 100vh;
    overflow: hidden;
  }
  .main-content {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-width: 0;
  }
  .top-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 24px;
    height: var(--header-height);
    background: rgba(245, 245, 247, 0.8);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    border-bottom: 0.5px solid var(--border);
    flex-shrink: 0;
    z-index: 10;
  }
  .top-bar-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .top-bar-right {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .page-title {
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.02em;
    color: var(--text-primary);
  }
  .lang-toggle {
    padding: 4px 10px;
    border: none;
    border-radius: 6px;
    background: var(--bg-hover);
    color: var(--text-secondary);
    font-size: 12px;
    font-weight: 600;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.2s;
    letter-spacing: 0.02em;
  }
  .lang-toggle:hover {
    background: var(--border-strong);
    color: var(--text-primary);
  }
  .status-indicator {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--red);
    transition: all 0.3s;
  }
  .status-indicator.online .status-dot {
    background: var(--green);
    box-shadow: 0 0 6px rgba(52, 199, 89, 0.4);
  }
  .status-text {
    font-size: 12px;
    color: var(--text-tertiary);
    font-weight: 500;
  }
  .page-content {
    flex: 1;
    overflow-y: auto;
    padding: 24px;
  }
</style>
