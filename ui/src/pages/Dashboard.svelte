<script>
  import { onMount } from 'svelte';
  import { executeQuery, healthCheck } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let stats = $state({ tables: 0, sessions: 0, memories: 0, decisions: 0 });
  let serverInfo = $state({ status: 'unknown', time: '' });
  let recentTables = $state([]);
  let error = $state('');
  let loading = $state(true);

  onMount(async () => {
    await refresh();
    const t = setInterval(refresh, 10000);
    return () => clearInterval(t);
  });

  async function refresh() {
    try {
      const h = await healthCheck();
      serverInfo = { status: h.data?.status || 'ok', time: h.data?.time || new Date().toISOString() };

      const tablesResult = await executeQuery('SHOW TABLES');
      recentTables = tablesResult?.Rows || [];
      stats.tables = recentTables.length;

      try {
        const r = await executeQuery('SELECT COUNT(*) as cnt FROM sessions');
        stats.sessions = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.sessions = 0; }

      try {
        const r = await executeQuery('SELECT COUNT(*) as cnt FROM memories');
        stats.memories = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.memories = 0; }

      try {
        const r = await executeQuery('SELECT COUNT(*) as cnt FROM decisions');
        stats.decisions = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.decisions = 0; }

      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
</script>

{#if error}
  <div class="error-msg">{error}</div>
{/if}

<div class="stat-grid">
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--blue-light); color: var(--blue);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.tables}</div>
      <div class="stat-label">{i18n.t('dashboard.tables')}</div>
    </div>
  </div>
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--green-light); color: var(--green);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.sessions}</div>
      <div class="stat-label">{i18n.t('dashboard.sessions')}</div>
    </div>
  </div>
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--purple-light); color: var(--purple);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8 8z"/><path d="M12 6v6l4 2"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.memories}</div>
      <div class="stat-label">{i18n.t('dashboard.memories')}</div>
    </div>
  </div>
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--orange-light); color: var(--orange);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><circle cx="12" cy="17" r="0.5"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.decisions}</div>
      <div class="stat-label">{i18n.t('dashboard.decisions')}</div>
    </div>
  </div>
</div>

<div class="grid-2">
  <div class="card">
    <h3 class="card-title">{i18n.t('dashboard.serverStatus')}</h3>
    <div class="info-grid">
      <div class="info-item">
        <span class="text-secondary">{i18n.t('dashboard.engine')}</span>
        <span class="badge badge-default">{i18n.t('dashboard.engineValue')}</span>
      </div>
      <div class="info-item">
        <span class="text-secondary">{i18n.t('dashboard.protocol')}</span>
        <span class="badge badge-default">{i18n.t('dashboard.protocolValue')}</span>
      </div>
      <div class="info-item">
        <span class="text-secondary">{i18n.t('sessions.state')}</span>
        <span class="badge badge-success">{serverInfo.status}</span>
      </div>
      <div class="info-item">
        <span class="text-secondary">{i18n.t('sessions.updatedAt')}</span>
        <span class="mono text-sm">{serverInfo.time ? new Date(serverInfo.time).toLocaleString() : '-'}</span>
      </div>
    </div>
  </div>

  <div class="card">
    <h3 class="card-title">{i18n.t('dashboard.tables')}</h3>
    {#if loading}
      <div class="loading">{i18n.t('status.loading')}</div>
    {:else if recentTables.length === 0}
      <div class="empty-state">
        <p>{i18n.t('dashboard.tablesEmpty')}</p>
        <p class="text-sm text-muted mt-2">{i18n.t('dashboard.tablesHint')}</p>
      </div>
    {:else}
      <div class="table-list">
        {#each recentTables as t}
          <div class="table-item">
            <span class="mono">{t.Values?.table_name || '-'}</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<div class="card mt-4">
  <h3 class="card-title">{i18n.t('dashboard.quickActions')}</h3>
  <div class="quick-actions">
    <button class="action-card" onclick={() => window.dispatchEvent(new CustomEvent('navigate', { detail: 'sql' }))}>
      <div class="action-icon" style="background: var(--blue-light); color: var(--blue);">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>
      </div>
      <span class="action-label">{i18n.t('dashboard.action.sql')}</span>
    </button>
    <button class="action-card" onclick={() => window.dispatchEvent(new CustomEvent('navigate', { detail: 'tables' }))}>
      <div class="action-icon" style="background: var(--teal-light); color: var(--teal);">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
      </div>
      <span class="action-label">{i18n.t('dashboard.action.tables')}</span>
    </button>
    <button class="action-card" onclick={() => window.dispatchEvent(new CustomEvent('navigate', { detail: 'sessions' }))}>
      <div class="action-icon" style="background: var(--green-light); color: var(--green);">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
      </div>
      <span class="action-label">{i18n.t('dashboard.action.sessions')}</span>
    </button>
    <button class="action-card" onclick={() => window.dispatchEvent(new CustomEvent('navigate', { detail: 'memories' }))}>
      <div class="action-icon" style="background: var(--purple-light); color: var(--purple);">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
      </div>
      <span class="action-label">{i18n.t('dashboard.action.memories')}</span>
    </button>
  </div>
</div>

<style>
  .stat-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 16px;
    margin-bottom: 20px;
  }
  .stat-card {
    background: var(--bg-surface);
    border-radius: var(--radius);
    padding: 20px;
    display: flex;
    align-items: center;
    gap: 14px;
    box-shadow: var(--shadow-sm);
    transition: box-shadow 0.2s;
  }
  .stat-card:hover {
    box-shadow: var(--shadow-md);
  }
  .stat-icon {
    width: 48px;
    height: 48px;
    border-radius: var(--radius);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .stat-value {
    font-size: 26px;
    font-weight: 700;
    line-height: 1.1;
    letter-spacing: -0.03em;
  }
  .stat-label {
    font-size: 12px;
    color: var(--text-tertiary);
    font-weight: 500;
    margin-top: 2px;
  }
  .table-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .table-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    background: var(--bg-base);
    font-size: 13px;
  }
  .quick-actions {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
  }
  .action-card {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 14px 20px;
    border: none;
    border-radius: var(--radius);
    background: var(--bg-base);
    color: var(--text-primary);
    cursor: pointer;
    transition: all 0.2s;
    font-family: var(--font-sans);
    outline: none;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
  }
  .action-card:hover {
    background: var(--bg-hover);
    box-shadow: var(--shadow-sm);
  }
  .action-card:active {
    transform: scale(0.97);
  }
  .action-icon {
    width: 36px;
    height: 36px;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  .action-label {
    font-size: 13px;
    font-weight: 500;
  }
  @media (max-width: 768px) {
    .stat-grid { grid-template-columns: repeat(2, 1fr); }
    .quick-actions { flex-direction: column; }
  }
</style>
