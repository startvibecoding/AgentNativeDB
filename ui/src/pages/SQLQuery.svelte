<script>
  import { executeQuery } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let sql = $state('SHOW TABLES');
  let result = $state(null);
  let error = $state('');
  let loading = $state(false);
  let history = $state([]);
  let execTime = $state(0);

  const presets = $derived([
    { label: i18n.t('sql.presets.tables'), sql: 'SHOW TABLES' },
    { label: i18n.t('sql.presets.indexes'), sql: 'SHOW INDEXES' },
    { label: i18n.t('sql.presets.sessions'), sql: 'SELECT * FROM sessions LIMIT 20' },
    { label: i18n.t('sql.presets.memories'), sql: 'SELECT * FROM memories LIMIT 20' },
    { label: i18n.t('sql.presets.decisions'), sql: 'SELECT * FROM decisions LIMIT 20' },
    { label: i18n.t('sql.presets.countSessions'), sql: 'SELECT COUNT(*) as total FROM sessions' },
    { label: i18n.t('sql.presets.countMemories'), sql: 'SELECT type, COUNT(*) as cnt FROM memories GROUP BY type' },
  ]);

  async function runQuery() {
    if (!sql.trim()) return;
    loading = true;
    error = '';
    result = null;
    const start = performance.now();
    try {
      result = await executeQuery(sql);
      execTime = Math.round(performance.now() - start);
      history = [{ sql: sql.trim(), time: new Date(), ok: true }, ...history.slice(0, 19)];
    } catch (e) {
      error = e.message;
      execTime = Math.round(performance.now() - start);
      history = [{ sql: sql.trim(), time: new Date(), ok: false, error: e.message }, ...history.slice(0, 19)];
    } finally {
      loading = false;
    }
  }

  function handleKeydown(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      runQuery();
    }
  }

  function loadHistory(h) {
    sql = h.sql;
  }

  function formatCell(v) {
    if (v === null || v === undefined) return 'NULL';
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
  }
</script>

<div class="sql-layout">
  <div class="sql-main">
    <div class="card mb-4">
      <div class="flex justify-between items-center mb-2">
        <h3 class="card-title">{i18n.t('sql.editor')}</h3>
        <span class="text-sm text-muted">{i18n.t('sql.shortcut')}</span>
      </div>
      <textarea
        class="sql-editor"
        bind:value={sql}
        onkeydown={handleKeydown}
        placeholder={i18n.t('sql.placeholder')}
        rows="5"
      ></textarea>
      <div class="flex justify-between items-center mt-3">
        <div class="flex gap-2 flex-wrap">
          {#each presets as p}
            <button class="btn btn-sm" onclick={() => sql = p.sql}>{p.label}</button>
          {/each}
        </div>
        <button class="btn btn-primary" onclick={runQuery} disabled={loading}>
          {loading ? i18n.t('common.executing') : '▶ ' + i18n.t('common.execute')}
        </button>
      </div>
    </div>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    {#if result}
      <div class="card">
        <div class="flex justify-between items-center mb-2">
          <h3 class="card-title">{i18n.t('sql.result')}</h3>
          <span class="text-sm text-muted">
            {#if result.Columns?.length}
              {result.Rows?.length || 0} {i18n.t('common.units.rows')} · {result.Columns.length} {i18n.t('common.units.cols')}
            {:else}
              {i18n.t('sql.affected')} {result.RowsAffected || 0} {i18n.t('common.units.rows')}
            {/if}
            · {execTime}{i18n.t('common.units.ms')}
          </span>
        </div>
        {#if result.Columns?.length}
          {#if !result.Rows?.length}
            <div class="empty-state"><p>{i18n.t('sql.emptyResult')}</p></div>
          {:else}
            <div class="table-container">
              <table>
                <thead>
                  <tr>
                    <th class="row-num">#</th>
                    {#each result.Columns as col}
                      <th>{col}</th>
                    {/each}
                  </tr>
                </thead>
                <tbody>
                  {#each result.Rows as row, i}
                    <tr>
                      <td class="row-num text-muted">{i + 1}</td>
                      {#each result.Columns as col}
                        <td class="mono text-sm">{formatCell(row.Values?.[col])}</td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        {:else}
          <div class="empty-state"><p>{i18n.t('sql.success')}</p></div>
        {/if}
      </div>
    {/if}
  </div>

  <div class="sql-sidebar">
    <div class="card">
      <h3 class="card-title">{i18n.t('sql.history')}</h3>
      {#if history.length === 0}
        <div class="empty-state"><p class="text-sm">{i18n.t('sql.historyEmpty')}</p></div>
      {:else}
        <div class="history-list">
          {#each history as h}
            <button class="history-item" class:error={!h.ok} onclick={() => loadHistory(h)}>
              <span class="history-sql mono text-sm truncate">{h.sql}</span>
              <span class="history-time text-muted text-sm">{h.time.toLocaleTimeString()}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .sql-layout {
    display: flex;
    gap: 16px;
    height: calc(100vh - var(--header-height) - 48px - 56px);
  }
  .sql-main {
    flex: 1;
    overflow-y: auto;
    min-width: 0;
  }
  .sql-sidebar {
    width: 260px;
    flex-shrink: 0;
    overflow-y: auto;
  }
  .sql-editor {
    width: 100%;
    padding: 14px 16px;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--bg-base);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 13px;
    line-height: 1.6;
    resize: vertical;
    min-height: 110px;
    outline: none;
    tab-size: 2;
    box-shadow: inset 0 0 0 0.5px rgba(0, 0, 0, 0.08);
    transition: box-shadow 0.2s;
  }
  .sql-editor:focus {
    box-shadow: inset 0 0 0 2px var(--accent);
    background: var(--bg-surface);
  }
  .row-num {
    width: 40px;
    text-align: center;
    font-size: 11px;
  }
  .history-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .history-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 10px;
    border: none;
    border-radius: var(--radius-sm);
    background: var(--bg-base);
    color: var(--text-primary);
    cursor: pointer;
    text-align: left;
    transition: background 0.15s;
    outline: none;
    font-family: var(--font-sans);
  }
  .history-item:hover {
    background: var(--bg-hover);
  }
  .history-item.error {
    border-left: 3px solid var(--red);
  }
  .history-sql {
    max-width: 100%;
  }
</style>
