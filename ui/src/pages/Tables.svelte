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

  // 子视图: 'list' | 'data' | 'schema'
  let currentView = $state('list');

  // 数据视图 (合并查看/编辑)
  let dataResult = $state(null);
  let dataLoading = $state(false);
  let dataError = $state('');
  let editingRow = $state(-1);
  let editValues = $state({});
  let editSaving = $state(false);
  let insertingNew = $state(false);
  let newRowValues = $state({});

  // 分页与查询条件
  let pageSize = $state(50);
  let pageIndex = $state(0);
  let totalRows = $state(0);
  let whereClause = $state('');
  let orderByClause = $state('');

  // 表结构编辑
  let schemaCols = $state([]);
  let schemaOriginal = $state([]);
  let schemaAddCol = $state({ name: '', type: 'STRING', primaryKey: false });
  let schemaSaving = $state(false);

  // 索引管理
  let tableIndexes = $state([]);
  let indexLoading = $state(false);
  let newIndex = $state({ name: '', column: '', type: 'BTREE' });
  const indexTypes = ['HASH', 'BTREE', 'INVERTED'];

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
    dataResult = null;
    editingRow = -1;
    tableIndexes = [];
    error = '';
    try {
      const schema = await executeQuery(`DESCRIBE ${name}`);
      tableSchema = schema;
      await loadIndexes();
    } catch (e) {
      error = `加载表结构失败: ${e.message}`;
      tableSchema = null;
    }
  }

  async function loadIndexes() {
    if (!selectedTable) return;
    indexLoading = true;
    try {
      const result = await executeQuery(`SHOW INDEXES FROM ${selectedTable}`);
      tableIndexes = result?.Rows || [];
    } catch (e) {
      // 某些表可能不支持索引查询,静默忽略
      tableIndexes = [];
    } finally {
      indexLoading = false;
    }
  }

  async function createIndex() {
    if (!selectedTable || !newIndex.name.trim() || !newIndex.column.trim()) return;
    const idxName = newIndex.name.trim();
    const colName = newIndex.column.trim();
    const idxType = newIndex.type || 'BTREE';
    const sql = `CREATE INDEX ${idxName} ON ${selectedTable}(${colName}) USING ${idxType}`;
    try {
      await executeQuery(sql);
      newIndex = { name: '', column: '', type: 'BTREE' };
      await loadIndexes();
    } catch (e) {
      error = e.message;
    }
  }

  async function dropIndex(idxName) {
    if (!confirm(`${i18n.t('tables.dropIndex')} ${idxName}?`)) return;
    try {
      await executeQuery(`DROP INDEX ${idxName}`);
      await loadIndexes();
    } catch (e) {
      error = e.message;
    }
  }

  async function openDataView() {
    currentView = 'data';
    pageIndex = 0;
    whereClause = '';
    orderByClause = '';
    dataError = '';
    if (!tableSchema) {
      try {
        tableSchema = await executeQuery(`DESCRIBE ${selectedTable}`);
      } catch { /* ignore */ }
    }
    await loadData();
  }

  async function openSchemaEdit() {
    currentView = 'schema';
    error = '';
    // 如果未加载 schema，先重新拉取一次
    if (!tableSchema) {
      try {
        tableSchema = await executeQuery(`DESCRIBE ${selectedTable}`);
      } catch (e) {
        error = `加载表结构失败: ${e.message}`;
      }
    }
    // 复制当前 schema 为可编辑对象
    schemaCols = (tableSchema?.Rows || []).map(r => ({
      name: r.Values?.field || '',
      type: parseTypeFromDescribe(r.Values?.type || 'STRING'),
      length: parseLenFromDescribe(r.Values?.type || ''),
      primaryKey: !!r.Values?.key,
      nullable: !!r.Values?.null,
      defaultVal: r.Values?.default ?? '',
      dropping: false,
      isNew: false,
    }));
    schemaOriginal = JSON.parse(JSON.stringify(schemaCols));
    schemaAddCol = { name: '', type: 'STRING', length: 0, primaryKey: false, nullable: true };
  }

  async function openIndexView() {
    currentView = 'indexes';
    error = '';
    if (!tableSchema) {
      try {
        tableSchema = await executeQuery(`DESCRIBE ${selectedTable}`);
      } catch (e) {
        error = `加载表结构失败: ${e.message}`;
      }
    }
    await loadIndexes();
  }

  function parseLenFromDescribe(t) {
    const m = String(t).match(/\((\d+)\)/);
    return m ? parseInt(m[1], 10) : 0;
  }

  function parseTypeFromDescribe(t) {
    const s = String(t).toUpperCase().split('(')[0];
    const map = {
      'INTEGER': 'INT',
      'BOOLEAN': 'BOOL',
      'VARCHAR': 'VARCHAR',
      'TEXT': 'TEXT',
      'STRING': 'STRING',
      'INT': 'INT',
      'FLOAT': 'FLOAT',
      'BOOL': 'BOOL'
    };
    return map[s] || 'STRING';
  }

  // ========= 新统一数据视图函数 =========
  async function loadData() {
    dataLoading = true;
    dataError = '';
    try {
      // 尝试 count
      let countSQL = `SELECT COUNT(*) AS cnt FROM ${selectedTable}`;
      if (whereClause.trim()) countSQL += ` WHERE ${whereClause.trim()}`;
      let total = null;
      try {
        const countResult = await executeQuery(countSQL);
        if (countResult?.Rows?.[0]?.Values?.cnt !== undefined) {
          total = Number(countResult.Rows[0].Values.cnt);
        }
      } catch (e) { /* ignore */ }
      totalRows = total ?? 0;

      // 数据查询
      let sql = `SELECT * FROM ${selectedTable}`;
      if (whereClause.trim()) sql += ` WHERE ${whereClause.trim()}`;
      if (orderByClause.trim()) sql += ` ORDER BY ${orderByClause.trim()}`;
      sql += ` LIMIT ${pageSize} OFFSET ${pageIndex * pageSize}`;

      const data = await executeQuery(sql);
      dataResult = data;
    } catch (e) {
      dataError = e.message;
    } finally {
      dataLoading = false;
    }
  }

  function startEditDataRow(idx) {
    if (!dataResult?.Columns || !dataResult?.Rows) return;
    const row = dataResult.Rows[idx];
    editValues = {};
    for (const col of dataResult.Columns) {
      editValues[col] = row.Values?.[col] ?? '';
    }
    editingRow = idx;
  }

  function cancelEditDataRow() {
    editingRow = -1;
    editValues = {};
  }

  async function saveEditDataRow(idx) {
    if (!dataResult?.Columns || !dataResult?.Rows) return;
    const row = dataResult.Rows[idx];
    const pkCol = dataResult.Columns[0]; // first column as PK
    const pkVal = row.Values?.[pkCol];

    if (pkVal === undefined || pkVal === null) {
      dataError = '无法确定主键值';
      return;
    }

    const setClauses = dataResult.Columns
      .filter(col => col !== pkCol)
      .map(col => {
        const v = editValues[col];
        if (v === '' || v === null || v === undefined) return `${col} = NULL`;
        const typeStr = getColumnType(col);
        if (isStringType(typeStr)) {
          return `${col} = '${String(v).replace(/'/g, "''")}'`;
        }
        if (typeStr.startsWith('BOOL')) {
          const s = String(v).toLowerCase();
          return `${col} = ${s === 'true' || s === '1' ? 'TRUE' : 'FALSE'}`;
        }
        if (typeof v === 'number' || !isNaN(Number(v))) return `${col} = ${v}`;
        return `${col} = '${String(v).replace(/'/g, "''")}'`;
      })
      .join(', ');

    if (!setClauses) {
      cancelEditDataRow();
      return;
    }

    const whereVal = typeof pkVal === 'string' ? `'${pkVal.replace(/'/g, "''")}'` : pkVal;
    const sql = `UPDATE ${selectedTable} SET ${setClauses} WHERE ${pkCol} = ${whereVal}`;

    editSaving = true;
    try {
      await executeQuery(sql);
      await loadData();
      editingRow = -1;
      editValues = {};
    } catch (e) {
      dataError = e.message;
    } finally {
      editSaving = false;
    }
  }

  async function deleteDataRow(idx) {
    if (!dataResult?.Columns || !dataResult?.Rows) return;
    const row = dataResult.Rows[idx];
    const pkCol = dataResult.Columns[0];
    const pkVal = row.Values?.[pkCol];

    if (pkVal === undefined || pkVal === null) {
      dataError = '无法确定主键值';
      return;
    }

    const whereVal = typeof pkVal === 'string' ? `'${pkVal.replace(/'/g, "''")}'` : pkVal;
    const sql = `DELETE FROM ${selectedTable} WHERE ${pkCol} = ${whereVal}`;

    try {
      await executeQuery(sql);
      await loadData();
    } catch (e) {
      dataError = e.message;
    }
  }

  function getInsertColumns() {
    if (dataResult?.Columns?.length > 0) return dataResult.Columns;
    if (tableSchema?.Rows?.length > 0) {
      return tableSchema.Rows.map(r => r.Values?.field).filter(Boolean);
    }
    return [];
  }

  function startInsertRow() {
    insertingNew = true;
    newRowValues = {};
    for (const c of getInsertColumns()) newRowValues[c] = '';
  }

  function cancelInsertRow() {
    insertingNew = false;
    newRowValues = {};
  }

  function getColumnType(col) {
    const row = tableSchema?.Rows?.find(r => r.Values?.field === col);
    return String(row?.Values?.type || '').toUpperCase();
  }

  function isStringType(typeStr) {
    return typeStr.startsWith('STRING') || typeStr.startsWith('VARCHAR') || typeStr.startsWith('TEXT');
  }

  async function saveInsertRow() {
    const cols = getInsertColumns();
    if (cols.length === 0) return;
    const vals = cols.map(col => {
      const v = newRowValues[col];
      if (v === '' || v === null || v === undefined) return 'NULL';
      const typeStr = getColumnType(col);
      if (isStringType(typeStr)) {
        return `'${String(v).replace(/'/g, "''")}'`;
      }
      if (typeStr.startsWith('BOOL')) {
        const s = String(v).toLowerCase();
        return s === 'true' || s === '1' ? 'TRUE' : 'FALSE';
      }
      if (typeof v === 'number' || !isNaN(Number(v))) return String(v);
      return `'${String(v).replace(/'/g, "''")}'`;
    }).join(', ');
    const sql = `INSERT INTO ${selectedTable} (${cols.join(', ')}) VALUES (${vals})`;

    try {
      await executeQuery(sql);
      await loadData();
      insertingNew = false;
      newRowValues = {};
    } catch (e) {
      dataError = e.message;
    }
  }

  async function firstPage() {
    pageIndex = 0;
    await loadData();
  }

  async function prevPage() {
    if (pageIndex > 0) {
      pageIndex -= 1;
      await loadData();
    }
  }

  async function nextPage() {
    if ((pageIndex + 1) * pageSize < (totalRows || Infinity)) {
      pageIndex += 1;
      await loadData();
    }
  }

  // ========= 表结构编辑函数 =========
  function markColDrop(idx, flag) {
    schemaCols[idx].dropping = flag;
  }

  function buildColDef(col) {
    let normType = col.type;
    if (normType === 'INTEGER') normType = 'INT';
    if (normType === 'BOOLEAN') normType = 'BOOL';
    let def = `${col.name} ${normType}`;
    if ((normType === 'VARCHAR') && col.length && col.length > 0) {
      def = `${col.name} ${normType}(${col.length})`;
    }
    if (col.primaryKey) def += ' PRIMARY KEY';
    else if (col.nullable === false) def += ' NOT NULL';
    return def;
  }

  async function saveSchema() {
    schemaSaving = true;
    error = '';
    try {
      // 1) 先处理删除（反向遵照索引）
      for (let i = schemaCols.length - 1; i >= 0; i--) {
        const col = schemaCols[i];
        const orig = schemaOriginal[i];
        if (col.dropping && orig) {
          await executeQuery(`ALTER TABLE ${selectedTable} DROP COLUMN ${orig.name}`);
        }
      }
      // 2) 处理修改
      for (let i = 0; i < schemaCols.length; i++) {
        const col = schemaCols[i];
        const orig = schemaOriginal[i];
        if (col.dropping) continue;
        if (!orig) continue; // 新增列在下一步处理
        if (col.name !== orig.name ||
            col.type !== orig.type ||
            col.length !== orig.length ||
            col.primaryKey !== orig.primaryKey ||
            col.nullable !== orig.nullable) {
          await executeQuery(`ALTER TABLE ${selectedTable} MODIFY COLUMN ${buildColDef(col)}`);
        }
      }
      // 3) 新增
      for (const col of schemaCols) {
        if (col.isNew && !col.dropping) {
          await executeQuery(`ALTER TABLE ${selectedTable} ADD COLUMN ${buildColDef(col)}`);
        }
      }
      // Refresh
      await loadTables();
      try {
        tableSchema = await executeQuery(`DESCRIBE ${selectedTable}`);
      } catch (e) {
        tableSchema = null;
      }
      currentView = 'list';
    } catch (e) {
      error = e.message;
    } finally {
      schemaSaving = false;
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
      let normType = c.type;
      if (normType === 'INTEGER') normType = 'INT';
      if (normType === 'BOOLEAN') normType = 'BOOL';
      let def = `${c.name} ${normType}`;
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
          <div class="detail-actions">
            <button class="btn btn-sm" onclick={openSchemaEdit}>{i18n.t('tables.editSchema')}</button>
            <button class="btn btn-sm" onclick={openIndexView}>{i18n.t('tables.indexes')}</button>
            <button class="btn btn-sm btn-primary" onclick={openDataView}>{i18n.t('tables.data')}</button>
            <button class="btn btn-sm btn-danger" onclick={() => showDropConfirm = true}>{i18n.t('common.delete')}</button>
          </div>
        </div>

        {#if tableSchema}
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
                  {#if tableSchema.Rows?.length > 0}
                    {#each tableSchema.Rows as row}
                      <tr>
                        {#each tableSchema.Columns || [] as col}
                          <td class="mono text-sm">{row.Values?.[col] ?? '-'}</td>
                        {/each}
                      </tr>
                    {/each}
                  {:else}
                    <tr>
                      <td class="text-muted text-center" colspan="{tableSchema.Columns?.length || 1}">暂无列定义</td>
                    </tr>
                  {/if}
                </tbody>
              </table>
            </div>
          </div>
        {/if}

      {/if}
    </div>
  </div>
{:else if currentView === 'indexes'}
  <!-- 索引管理视图 -->
  <div class="view-header">
    <button class="btn btn-sm" onclick={() => { currentView = 'list'; }}>
      ← {i18n.t('tables.backToList')}
    </button>
    <h3 class="mono" style="font-size:16px; font-weight:600;">
      {i18n.t('tables.indexes')} <span class="text-muted text-sm">— {selectedTable}</span>
    </h3>
  </div>

  {#if error}
    <div class="error-msg">{error}</div>
  {/if}

  <div class="card mb-4">
    <h4 class="card-title">{i18n.t('tables.createIndex')}</h4>
    <div class="index-form">
      <input class="input" bind:value={newIndex.name} placeholder={i18n.t('tables.indexName')} style="flex:2;" />
      <select class="input" bind:value={newIndex.column} style="flex:2;">
        <option value="">{i18n.t('tables.indexColumn')}</option>
        {#each tableSchema?.Rows || [] as row}
          <option value={row.Values?.field}>{row.Values?.field}</option>
        {/each}
      </select>
      <select class="input" bind:value={newIndex.type} style="flex:1;">
        {#each indexTypes as t}
          <option value={t}>{t}</option>
        {/each}
      </select>
      <button
        class="btn btn-sm btn-primary"
        onclick={createIndex}
        disabled={!newIndex.name.trim() || !newIndex.column}
      >
        + {i18n.t('tables.createIndex')}
      </button>
    </div>
  </div>

  <div class="card">
    <h4 class="card-title">{i18n.t('tables.indexes')}</h4>
    {#if indexLoading}
      <div class="loading">{i18n.t('status.loading')}</div>
    {:else if tableIndexes.length === 0}
      <p class="text-muted text-sm">{i18n.t('tables.noIndexes')}</p>
    {:else}
      <div class="table-container">
        <table>
          <thead>
            <tr>
              <th>{i18n.t('tables.indexName')}</th>
              <th>{i18n.t('tables.indexColumn')}</th>
              <th>{i18n.t('tables.indexType')}</th>
              <th style="width: 100px;">{i18n.t('tables.operation')}</th>
            </tr>
          </thead>
          <tbody>
            {#each tableIndexes as idx}
              <tr>
                <td class="mono text-sm">{idx.Values?.name}</td>
                <td class="mono text-sm">{idx.Values?.column}</td>
                <td><span class="badge badge-info">{idx.Values?.type}</span></td>
                <td>
                  <button class="btn btn-xs btn-danger" onclick={() => dropIndex(idx.Values?.name)}>
                    {i18n.t('tables.dropIndex')}
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
{:else if currentView === 'data'}
  <!-- 统一数据视图 -->
  <div class="view-header">
    <button class="btn btn-sm" onclick={() => { currentView = 'list'; dataResult = null; editingRow = -1; }}>
      ← {i18n.t('tables.backToList')}
    </button>
    <h3 class="mono" style="font-size:16px; font-weight:600;">
      {i18n.t('tables.data')} <span class="text-muted text-sm">— {selectedTable}</span>
    </h3>
    <div class="data-header-meta">
      {#if totalRows > 0}
        <span class="text-secondary text-sm">{i18n.t('common.total')} {totalRows} {i18n.t('common.units.rows')}</span>
      {/if}
      <span class="text-secondary text-sm">第 {pageIndex + 1} 页</span>
    </div>
  </div>

  {#if dataError}
    <div class="error-msg">{dataError}</div>
  {/if}

  <!-- 查询条件 -->
  <div class="data-filter card mb-4">
    <div class="filter-row">
      <label class="filter-label">{i18n.t('tables.where')}</label>
      <input class="input flex-1" bind:value={whereClause} placeholder="例如: id > 100" />
    </div>
    <div class="filter-row">
      <label class="filter-label">{i18n.t('tables.order')}</label>
      <input class="input flex-1" bind:value={orderByClause} placeholder="例如: created_at DESC" />
    </div>
    <div class="filter-row">
      <label class="filter-label">{i18n.t('tables.pageSize')}</label>
      <select class="input" style="width: 120px;" bind:value={pageSize}>
        <option value={20}>20</option>
        <option value={50}>50</option>
        <option value={100}>100</option>
      </select>
      <button class="btn btn-sm btn-primary ml-2" onclick={loadData}>{i18n.t('common.refresh')}</button>
    </div>
  </div>

  {#if dataLoading}
    <div class="loading">{i18n.t('status.loading')}</div>
  {:else}
    <div class="data-actions mb-2">
      <button class="btn btn-sm btn-primary" onclick={startInsertRow} disabled={insertingNew}>
        + {i18n.t('tables.insertRow')}
      </button>
    </div>

    {#if !dataResult?.Rows?.length && !insertingNew}
      <div class="empty-state"><p>{i18n.t('tables.dataEmpty')}</p></div>
    {:else}
      <div class="card">
        <div class="table-container">
          <table>
            <thead>
              <tr>
                {#each getInsertColumns() as col}
                  <th>{col}</th>
                {/each}
                <th style="width: 140px;">{i18n.t('tables.operation')}</th>
              </tr>
            </thead>
            <tbody>
              {#if insertingNew}
                <tr class="editing-row">
                  {#each getInsertColumns() as col}
                    <td class="mono text-sm">
                      <input class="cell-input" bind:value={newRowValues[col]} />
                    </td>
                  {/each}
                  <td class="row-actions">
                    <button class="btn btn-xs btn-primary" onclick={saveInsertRow}>
                      {i18n.t('tables.saveNewRow')}
                    </button>
                    <button class="btn btn-xs" onclick={cancelInsertRow}>
                      {i18n.t('tables.cancelInsert')}
                    </button>
                  </td>
                </tr>
              {/if}

              {#each dataResult?.Rows || [] as row, idx}
                {#if editingRow === idx}
                  <!-- 编辑行 -->
                  <tr class="editing-row">
                    {#each dataResult.Columns || [] as col}
                      <td class="mono text-sm">
                        <input class="cell-input" bind:value={editValues[col]} />
                      </td>
                    {/each}
                    <td class="row-actions">
                      <button class="btn btn-xs btn-primary" onclick={() => saveEditDataRow(idx)} disabled={editSaving}>
                        {editSaving ? '...' : i18n.t('common.confirm')}
                      </button>
                      <button class="btn btn-xs" onclick={cancelEditDataRow}>{i18n.t('common.cancel')}</button>
                    </td>
                  </tr>
                {:else}
                  <!-- 普通行 -->
                  <tr>
                    {#each dataResult.Columns || [] as col}
                      <td class="mono text-sm">{@render formatCell(row.Values?.[col])}</td>
                    {/each}
                    <td class="row-actions">
                      <button class="btn btn-xs" onclick={() => startEditDataRow(idx)}>{i18n.t('tables.edit')}</button>
                      <button class="btn btn-xs btn-danger" onclick={() => deleteDataRow(idx)}>{i18n.t('common.delete')}</button>
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <!-- 分页 -->
      <div class="pagination">
        <button class="btn btn-sm" onclick={prevPage} disabled={pageIndex <= 0}>{i18n.t('tables.prevPage')}</button>
        <span class="text-secondary text-sm">第 {pageIndex + 1} 页</span>
        <button class="btn btn-sm" onclick={nextPage} disabled={(pageIndex + 1) * pageSize >= (totalRows || Infinity)}>{i18n.t('tables.nextPage')}</button>
      </div>
    {/if}
  {/if}
{:else if currentView === 'schema'}
  <!-- 表结构编辑视图 -->
  <div class="view-header">
    <button class="btn btn-sm" onclick={() => { currentView = 'list'; schemaCols = []; }}>
      ← {i18n.t('tables.backToList')}
    </button>
    <h3 class="mono" style="font-size:16px; font-weight:600;">
      {i18n.t('tables.schemaEditTitle')} <span class="text-muted text-sm">— {selectedTable}</span>
    </h3>
    <button class="btn btn-sm btn-primary" onclick={saveSchema} disabled={schemaSaving}>
      {schemaSaving ? '...' : i18n.t('tables.saveSchema')}
    </button>
  </div>

  <div class="card">
    <h4 class="card-title">现有列</h4>
    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>列名</th>
            <th>类型</th>
            <th>主键</th>
            <th style="width: 100px;">{i18n.t('tables.operation')}</th>
          </tr>
        </thead>
        <tbody>
          {#each schemaCols as col, idx}
            {#if schemaOriginal[idx] || col.isNew}
              <tr class={col.isNew ? 'editing-row' : ''}>
                <td>
                  <input class="input" bind:value={col.name} style="width: 180px;" />
                </td>
                <td>
                  <select class="input" style="width: 140px;" bind:value={col.type}>
                    <option value="INT">INT</option>
                    <option value="INTEGER">INTEGER</option>
                    <option value="VARCHAR">VARCHAR</option>
                    <option value="TEXT">TEXT</option>
                    <option value="STRING">STRING</option>
                    <option value="FLOAT">FLOAT</option>
                    <option value="BOOL">BOOL</option>
                    <option value="BOOLEAN">BOOLEAN</option>
                  </select>
                </td>
                <td style="text-align:center;">
                  <input type="checkbox" bind:checked={col.primaryKey} />
                </td>
                <td>
                  {#if col.isNew}
                    <button class="btn btn-xs btn-danger" onclick={() => { schemaCols = schemaCols.filter((_, i) => i !== idx); }}>移除</button>
                  {:else if !col.primaryKey}
                    <button class="btn btn-xs btn-danger" onclick={() => markColDrop(idx, true)} disabled={col.dropping}>
                      {col.dropping ? '标记删除' : i18n.t('tables.dropCol')}
                    </button>
                    {#if col.dropping}
                      <button class="btn btn-xs" onclick={() => markColDrop(idx, false)}>撤销</button>
                    {/if}
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  </div>

  <div class="card mt-4">
    <h4 class="card-title">添加新列</h4>
    <div class="column-row">
      <input class="input" bind:value={schemaAddCol.name} placeholder="列名" style="flex:2;" />
      <select class="input" bind:value={schemaAddCol.type} style="flex:1;">
        <option value="STRING">STRING</option>
        <option value="INT">INT</option>
        <option value="INTEGER">INTEGER</option>
        <option value="VARCHAR">VARCHAR</option>
        <option value="TEXT">TEXT</option>
        <option value="FLOAT">FLOAT</option>
        <option value="BOOL">BOOL</option>
        <option value="BOOLEAN">BOOLEAN</option>
      </select>
      <label class="flex items-center gap-2 text-sm" style="white-space:nowrap;">
        <input type="checkbox" bind:checked={schemaAddCol.primaryKey} /> {i18n.t('tables.primaryKey')}
      </label>
      <button class="btn btn-sm btn-primary" onclick={() => {
        if (schemaAddCol.name.trim()) {
          // Normalize type
          let normType = schemaAddCol.type;
          if (normType === 'INTEGER') normType = 'INT';
          if (normType === 'BOOLEAN') normType = 'BOOL';
          schemaCols = [...schemaCols, { ...schemaAddCol, name: schemaAddCol.name.trim(), type: normType, dropping: false, isNew: true }];
          schemaAddCol = { name: '', type: 'STRING', primaryKey: false };
        }
      }} disabled={!schemaAddCol.name.trim()}>+ {i18n.t('tables.addCol')}</button>
    </div>
  </div>
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
              <option value="INTEGER">INTEGER</option>
              <option value="VARCHAR">VARCHAR</option>
              <option value="TEXT">TEXT</option>
              <option value="FLOAT">FLOAT</option>
              <option value="BOOL">BOOL</option>
              <option value="BOOLEAN">BOOLEAN</option>
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
  .index-form {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
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
  .detail-actions {
    display: flex;
    gap: 8px;
  }
  .data-filter {
    padding: 16px;
  }
  .filter-row {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-bottom: 8px;
  }
  .filter-row:last-child {
    margin-bottom: 0;
  }
  .filter-label {
    font-size: 13px;
    color: var(--text-secondary);
    min-width: 110px;
  }
  .data-header-meta {
    display: flex;
    gap: 16px;
  }
  .data-actions {
    display: flex;
    align-items: center;
  }
  .pagination {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 12px;
  }
</style>
