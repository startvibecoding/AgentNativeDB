<script>
  import { onMount } from 'svelte';
  import { executeQuery } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let tables = $state([]);
  let selectedTable = $state('');
  let tableSchema = $state(null);
  let error = $state('');
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showDropConfirm = $state(false);
  let createTableName = $state('');
  let createColumns = $state([{ name: 'id', type: 'STRING', primaryKey: true }]);

  // 子视图: 'list' | 'view' | 'edit'
  let currentView = $state('list');

  // 查看数据
  let viewData = $state(null);
  let viewLoading = $state(false);

  // 编辑数据
  let editData = $state(null);
  let editLoading = $state(false);
  let editingRow = $state(-1);
  let editValues = $state({});
  let editSaving = $state(false);

  onMount(loadTables);

  async function loadTables() {
    try {
      const result = await executeQuery('SHOW TABLES');
      tables = result?.Rows || [];
      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function selectTable(name) {
    selectedTable = name;
    currentView = 'list';
    tableSchema = null;
    viewData = null;
    editData = null;
    editingRow = -1;
    try {
      const schema = await executeQuery(`DESCRIBE ${name}`).catch(() => null);
      tableSchema = schema;
    } catch (e) {
      error = e.message;
    }
  }

  async function viewTableData() {
    viewLoading = true;
    try {
      const data = await executeQuery(`SELECT * FROM ${selectedTable} LIMIT 200`);
      viewData = data;
      currentView = 'view';
    } catch (e) {
      error = e.message;
    } finally {
      viewLoading = false;
    }
  }

  async function editTableData() {
    editLoading = true;
    try {
      const data = await executeQuery(`SELECT * FROM ${selectedTable} LIMIT 200`);
      editData = data;
      editingRow = -1;
      editValues = {};
      currentView = 'edit';
    } catch (e) {
      error = e.message;
    } finally {
      editLoading = false;
    }
  }

  function startEditRow(idx) {
    if (!editData?.Columns || !editData?.Rows) return;
    const row = editData.Rows[idx];
    editValues = {};
    for (const col of editData.Columns) {
      editValues[col] = row.Values?.[col] ?? '';
    }
    editingRow = idx;
  }

  function cancelEdit() {
    editingRow = -1;
    editValues = {};
  }

  async function saveRow(idx) {
    if (!editData?.Columns || !editData?.Rows) return;
    const row = editData.Rows[idx];
    const pkCol = editData.Columns[0]; // 假设第一列为主键
    const pkVal = row.Values?.[pkCol];

    if (pkVal === undefined || pkVal === null) {
      error = '无法确定主键值';
      return;
    }

    const setClauses = editData.Columns
      .filter(col => col !== pkCol)
      .map(col => {
        const v = editValues[col];
        if (v === '' || v === null || v === undefined) return `${col} = NULL`;
        if (typeof v === 'number' || !isNaN(Number(v))) return `${col} = ${v}`;
        return `${col} = '${String(v).replace(/'/g, "''")}'`;
      })
      .join(', ');

    if (!setClauses) {
      cancelEdit();
      return;
    }

    const whereVal = typeof pkVal === 'string' ? `'${pkVal.replace(/'/g, "''")}'` : pkVal;
    const sql = `UPDATE ${selectedTable} SET ${setClauses} WHERE ${pkCol} = ${whereVal}`;

    editSaving = true;
    try {
      await executeQuery(sql);
      // 刷新数据
      const data = await executeQuery(`SELECT * FROM ${selectedTable} LIMIT 200`);
      editData = data;
      editingRow = -1;
      editValues = {};
    } catch (e) {
      error = e.message;
    } finally {
      editSaving = false;
    }
  }

  async function deleteRow(idx) {
    if (!editData?.Columns || !editData?.Rows) return;
    const row = editData.Rows[idx];
    const pkCol = editData.Columns[0];
    const pkVal = row.Values?.[pkCol];

    if (pkVal === undefined || pkVal === null) {
      error = '无法确定主键值';
      return;
    }

    const whereVal = typeof pkVal === 'string' ? `'${pkVal.replace(/'/g, "''")}'` : pkVal;
    const sql = `DELETE FROM ${selectedTable} WHERE ${pkCol} = ${whereVal}`;

    try {
      await executeQuery(sql);
      const data = await executeQuery(`SELECT * FROM ${selectedTable} LIMIT 200`);
      editData = data;
      editingRow = -1;
      editValues = {};
    } catch (e) {
      error = e.message;
    }
  }

  async function dropTable(name) {
    try {
      await executeQuery(`DROP TABLE ${name}`);
      selectedTable = '';
      tableSchema = null;
      currentView = 'list';
      showDropConfirm = false;
      await loadTables();
    } catch (e) {
      error = e.message;
    }
  }

  function addColumn() {
    createColumns = [...createColumns, { name: '', type: 'STRING', primaryKey: false }];
  }

  function removeColumn(i) {
    createColumns = createColumns.filter((_, idx) => idx !== i);
  }

  async function createTable() {
    if (!createTableName.trim()) return;
    const cols = createColumns.map(c => {
      let def = `${c.name} ${c.type}`;
      if (c.primaryKey) def += ' PRIMARY KEY';
      return def;
    }).join(', ');
    try {
      await executeQuery(`CREATE TABLE ${createTableName} (${cols})`);
      showCreateModal = false;
      createTableName = '';
      createColumns = [{ name: 'id', type: 'STRING', primaryKey: true }];
      await loadTables();
    } catch (e) {
      error = e.message;
    }
  }
</script>

{#if error}
  <div class="error-msg">{error}</div>
{/if}

{#if currentView === 'list'}
  <!-- 列表视图 -->
  <div class="flex justify-between items-center mb-4">
    <span class="text-secondary text-sm">{i18n.t('common.total')} {tables.length} {i18n.t('common.units.items')}</span>
    <button class="btn btn-primary" onclick={() => showCreateModal = true}>+ {i18n.t('tables.create')}</button>
  </div>

  <div class="tables-layout">
    <div class="table-sidebar">
      {#if loading}
        <div class="loading">{i18n.t('status.loading')}</div>
      {:else if tables.length === 0}
        <div class="empty-state"><p>{i18n.t('dashboard.tablesEmpty')}</p></div>
      {:else}
        {#each tables as t}
          <button
            class="table-nav-item"
            class:active={selectedTable === t.Values?.table_name}
            onclick={() => selectTable(t.Values?.table_name)}
          >
            <span class="mono">{t.Values?.table_name}</span>
          </button>
        {/each}
      {/if}
    </div>

    <div class="table-detail">
      {#if !selectedTable}
        <div class="empty-state"><p>{i18n.t('tables.selectHint')}</p></div>
      {:else}
        <div class="flex justify-between items-center mb-4">
          <h3 class="mono" style="font-size:16px; font-weight:600;">{selectedTable}</h3>
          <button class="btn btn-sm btn-danger" onclick={() => showDropConfirm = true}>{i18n.t('common.delete')}</button>
        </div>

        {#if tableSchema?.Rows?.length}
          <div class="card mb-4">
            <h4 class="card-title">{i18n.t('tables.structure')}</h4>
            <div class="table-container">
              <table>
                <thead>
                  <tr>
                    {#each tableSchema.Columns || [] as col}
                      <th>{col}</th>
                    {/each}
                  </tr>
                </thead>
                <tbody>
                  {#each tableSchema.Rows as row}
                    <tr>
                      {#each tableSchema.Columns || [] as col}
                        <td class="mono text-sm">{row.Values?.[col] ?? '-'}</td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </div>
        {/if}

        <div class="card">
          <div class="card-title-row">
            <h4 class="card-title">{i18n.t('tables.actions')}</h4>
          </div>
          <div class="action-buttons">
            <button class="btn btn-action" onclick={viewTableData} disabled={viewLoading}>
              <span class="btn-icon-text">👁</span>
              {viewLoading ? i18n.t('status.loading') : i18n.t('tables.viewData')}
            </button>
            <button class="btn btn-action" onclick={editTableData} disabled={editLoading}>
              <span class="btn-icon-text">✏️</span>
              {editLoading ? i18n.t('status.loading') : i18n.t('tables.editData')}
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{:else if currentView === 'view'}
  <!-- 查看数据视图 -->
  <div class="view-header">
    <button class="btn btn-sm" onclick={() => { currentView = 'list'; viewData = null; }}>
      ← {i18n.t('tables.backToList')}
    </button>
    <h3 class="mono" style="font-size:16px; font-weight:600;">
      {i18n.t('tables.viewDataTitle')} <span class="text-muted text-sm">— {selectedTable}</span>
    </h3>
    <span class="text-secondary text-sm">{i18n.t('common.total')} {viewData?.Rows?.length || 0} {i18n.t('common.units.rows')}</span>
  </div>

  {#if viewLoading}
    <div class="loading">{i18n.t('status.loading')}</div>
  {:else if !viewData?.Rows?.length}
    <div class="empty-state"><p>{i18n.t('tables.dataEmpty')}</p></div>
  {:else}
    <div class="card">
      <div class="table-container">
        <table>
          <thead>
            <tr>
              {#each viewData.Columns || [] as col}
                <th>{col}</th>
              {/each}
            </tr>
          </thead>
          <tbody>
            {#each viewData.Rows as row}
              <tr>
                {#each viewData.Columns || [] as col}
                  <td class="mono text-sm">{formatCell(row.Values?.[col])}</td>
                {/each}
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
{:else if currentView === 'edit'}
  <!-- 编辑数据视图 -->
  <div class="view-header">
    <button class="btn btn-sm" onclick={() => { currentView = 'list'; editData = null; editingRow = -1; }}>
      ← {i18n.t('tables.backToList')}
    </button>
    <h3 class="mono" style="font-size:16px; font-weight:600;">
      {i18n.t('tables.editDataTitle')} <span class="text-muted text-sm">— {selectedTable}</span>
    </h3>
    <span class="text-secondary text-sm">{i18n.t('common.total')} {editData?.Rows?.length || 0} {i18n.t('common.units.rows')}</span>
  </div>

  {#if editLoading}
    <div class="loading">{i18n.t('status.loading')}</div>
  {:else if !editData?.Rows?.length}
    <div class="empty-state"><p>{i18n.t('tables.dataEmpty')}</p></div>
  {:else}
    <div class="card">
      <div class="table-container">
        <table>
          <thead>
            <tr>
              {#each editData.Columns || [] as col}
                <th>{col}</th>
              {/each}
              <th style="width:120px;">{i18n.t('tables.operation')}</th>
            </tr>
          </thead>
          <tbody>
            {#each editData.Rows as row, idx}
              {#if editingRow === idx}
                <!-- 编辑行 -->
                <tr class="editing-row">
                  {#each editData.Columns || [] as col}
                    <td class="mono text-sm">
                      <input
                        class="cell-input"
                        bind:value={editValues[col]}
                        onkeydown={(e) => { if (e.key === 'Enter') saveRow(idx); if (e.key === 'Escape') cancelEdit(); }}
                      />
                    </td>
                  {/each}
                  <td class="row-actions">
                    <button class="btn btn-xs btn-primary" onclick={() => saveRow(idx)} disabled={editSaving}>
                      {editSaving ? '...' : i18n.t('common.confirm')}
                    </button>
                    <button class="btn btn-xs" onclick={cancelEdit}>{i18n.t('common.cancel')}</button>
                  </td>
                </tr>
              {:else}
                <!-- 普通行 -->
                <tr>
                  {#each editData.Columns || [] as col}
                    <td class="mono text-sm">{formatCell(row.Values?.[col])}</td>
                  {/each}
                  <td class="row-actions">
                    <button class="btn btn-xs" onclick={() => startEditRow(idx)}>{i18n.t('tables.edit')}</button>
                    <button class="btn btn-xs btn-danger" onclick={() => deleteRow(idx)}>{i18n.t('common.delete')}</button>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
{/if}

{#if showCreateModal}
  <div class="modal-overlay" onclick={() => showCreateModal = false}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3 class="modal-title">{i18n.t('tables.createTitle')}</h3>
      <div class="form-group">
        <label class="form-label">{i18n.t('tables.tableName')}</label>
        <input class="input" bind:value={createTableName} placeholder="my_table" />
      </div>
      <div class="form-group">
        <label class="form-label">{i18n.t('tables.columns')}</label>
        {#each createColumns as col, i}
          <div class="column-row">
            <input class="input" bind:value={col.name} placeholder={i18n.t('tables.column')} style="flex:2" />
            <select class="input" bind:value={col.type} style="flex:1">
              <option value="STRING">STRING</option>
              <option value="INT">INT</option>
              <option value="FLOAT">FLOAT</option>
              <option value="BOOL">BOOL</option>
              <option value="TEXT">TEXT</option>
            </select>
            <label class="flex items-center gap-2 text-sm" style="white-space:nowrap">
              <input type="checkbox" bind:checked={col.primaryKey} /> {i18n.t('tables.primaryKey')}
            </label>
            {#if createColumns.length > 1}
              <button class="btn-icon" onclick={() => removeColumn(i)}>✕</button>
            {/if}
          </div>
        {/each}
        <button class="btn btn-sm mt-2" onclick={addColumn}>+ {i18n.t('tables.addColumn')}</button>
      </div>
      <div class="flex justify-between mt-4">
        <button class="btn" onclick={() => showCreateModal = false}>{i18n.t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={createTable}>{i18n.t('common.create')}</button>
      </div>
    </div>
  </div>
{/if}

{#if showDropConfirm}
  <div class="modal-overlay" onclick={() => showDropConfirm = false}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3 class="modal-title">{i18n.t('tables.dropConfirm')}</h3>
      <p class="mb-4">{i18n.t('tables.dropMessage')} <strong class="mono">{selectedTable}</strong> {i18n.t('tables.dropHint')}</p>
      <div class="flex justify-between">
        <button class="btn" onclick={() => showDropConfirm = false}>{i18n.t('common.cancel')}</button>
        <button class="btn btn-danger" onclick={() => dropTable(selectedTable)}>{i18n.t('common.confirm')}</button>
      </div>
    </div>
  </div>
{/if}

{#snippet formatCell(v)}
  {#if v === null || v === undefined}
    <span class="text-muted">NULL</span>
  {:else if typeof v === 'object'}
    <span class="text-muted">{JSON.stringify(v)}</span>
  {:else}
    {String(v)}
  {/if}
{/snippet}

<style>
  .tables-layout {
    display: flex;
    gap: 16px;
    height: calc(100vh - var(--header-height) - 48px - 56px);
  }
  .table-sidebar {
    width: 200px;
    border-radius: var(--radius);
    background: var(--bg-surface);
    overflow-y: auto;
    flex-shrink: 0;
    box-shadow: var(--shadow-sm);
  }
  .table-nav-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 10px 14px;
    border: none;
    border-bottom: 0.5px solid var(--border);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    text-align: left;
    font-family: var(--font-mono);
    font-size: 12px;
    transition: all 0.15s;
    outline: none;
  }
  .table-nav-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .table-nav-item.active {
    background: var(--bg-active);
    color: var(--accent);
  }
  .table-detail {
    flex: 1;
    overflow-y: auto;
    min-width: 0;
  }
  .column-row {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 8px;
  }
  .card-title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .action-buttons {
    display: flex;
    gap: 12px;
    padding: 16px;
  }
  .btn-action {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 20px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s;
  }
  .btn-action:hover:not(:disabled) {
    background: var(--bg-hover);
    border-color: var(--accent);
    color: var(--accent);
  }
  .btn-action:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .btn-icon-text {
    font-size: 16px;
  }
  .view-header {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 16px;
  }
  .editing-row {
    background: var(--bg-active);
  }
  .cell-input {
    width: 100%;
    padding: 4px 8px;
    border: 1px solid var(--accent);
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 12px;
    outline: none;
  }
  .cell-input:focus {
    box-shadow: 0 0 0 2px rgba(0, 122, 255, 0.2);
  }
  .row-actions {
    display: flex;
    gap: 4px;
    white-space: nowrap;
  }
  .btn-xs {
    padding: 2px 8px;
    font-size: 11px;
  }
</style>
