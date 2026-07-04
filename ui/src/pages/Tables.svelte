<script>
  import { onMount } from 'svelte';
  import { executeQuery } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let tables = $state([]);
  let selectedTable = $state('');
  let tableData = $state(null);
  let tableSchema = $state(null);
  let error = $state('');
  let loading = $state(true);
  let showCreateModal = $state(false);
  let showDropConfirm = $state(false);
  let createTableName = $state('');
  let createColumns = $state([{ name: 'id', type: 'STRING', primaryKey: true }]);

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
    tableData = null;
    tableSchema = null;
    try {
      const [data, schema] = await Promise.all([
        executeQuery(`SELECT * FROM ${name} LIMIT 100`),
        executeQuery(`DESCRIBE ${name}`).catch(() => null),
      ]);
      tableData = data;
      tableSchema = schema;
    } catch (e) {
      error = e.message;
    }
  }

  async function dropTable(name) {
    try {
      await executeQuery(`DROP TABLE ${name}`);
      selectedTable = '';
      tableData = null;
      tableSchema = null;
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

      {#if tableData}
        <div class="card">
          <h4 class="card-title">{i18n.t('tables.data')} <span class="text-muted text-sm" style="font-weight:400; text-transform:none; letter-spacing:0;">({i18n.t('tables.dataHint')})</span></h4>
          {#if !tableData.Rows?.length}
            <div class="empty-state"><p>{i18n.t('tables.dataEmpty')}</p></div>
          {:else}
            <div class="table-container">
              <table>
                <thead>
                  <tr>
                    {#each tableData.Columns || [] as col}
                      <th>{col}</th>
                    {/each}
                  </tr>
                </thead>
                <tbody>
                  {#each tableData.Rows as row}
                    <tr>
                      {#each tableData.Columns || [] as col}
                        <td class="mono text-sm">{formatCell(row.Values?.[col])}</td>
                      {/each}
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      {/if}
    {/if}
  </div>
</div>

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
</style>
