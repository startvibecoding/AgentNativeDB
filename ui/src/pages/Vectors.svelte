<script>
  import { onMount } from 'svelte';
  import {
    listVectorIndexes,
    createVectorIndex,
    insertVector,
    deleteVector,
    searchVector,
  } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let indexes = $state([]);
  let selected = $state('');
  let loading = $state(false);
  let error = $state('');

  let showCreate = $state(false);
  let newIndex = $state({ name: '', dim: 128, metric: 'cosine' });

  // 插入
  let insertId = $state('');
  let insertVec = $state('');
  let insertPayload = $state('');
  let inserting = $state(false);

  // 搜索
  let searchVec = $state('');
  let searchTopK = $state(10);
  let searchResults = $state([]);
  let searching = $state(false);

  // payload 详情弹窗
  let payloadModal = $state({ show: false, data: null });

  onMount(loadIndexes);

  async function loadIndexes() {
    loading = true;
    error = '';
    try {
      indexes = (await listVectorIndexes()) || [];
      if (indexes.length > 0 && !indexes.find(i => i.name === selected)) {
        selected = indexes[0].name;
      } else if (indexes.length === 0) {
        selected = '';
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function currentIndex() {
    return indexes.find(i => i.name === selected);
  }

  async function doCreate() {
    error = '';
    if (!newIndex.name.trim() || !newIndex.dim || newIndex.dim <= 0) {
      error = '请填写索引名和维度';
      return;
    }
    try {
      await createVectorIndex(newIndex.name.trim(), Number(newIndex.dim), newIndex.metric);
      showCreate = false;
      selected = newIndex.name.trim();
      newIndex = { name: '', dim: 128, metric: 'cosine' };
      await loadIndexes();
    } catch (e) {
      error = e.message;
    }
  }

  // 解析用户输入的向量: 支持 [a,b,c] 或 a,b,c 或者换行分隔
  function parseVector(s) {
    if (!s) return null;
    let t = s.trim();
    if (t.startsWith('[')) t = t.slice(1);
    if (t.endsWith(']')) t = t.slice(0, -1);
    const parts = t.split(/[\s,]+/).filter(x => x.length > 0);
    const out = [];
    for (const p of parts) {
      const n = Number(p);
      if (Number.isNaN(n)) return null;
      out.push(n);
    }
    return out;
  }

  async function doInsert() {
    error = '';
    if (!selected) return;
    if (!insertId.trim()) { error = '请填写 ID'; return; }
    const vec = parseVector(insertVec);
    if (!vec) { error = '向量格式无效'; return; }
    const idx = currentIndex();
    if (idx && vec.length !== idx.dim) {
      error = `向量维度不匹配: 需要 ${idx.dim}, 实际 ${vec.length}`;
      return;
    }
    let payload = null;
    if (insertPayload.trim()) {
      try {
        payload = JSON.parse(insertPayload.trim());
      } catch {
        error = 'payload 不是合法 JSON';
        return;
      }
    }

    inserting = true;
    try {
      await insertVector(selected, insertId.trim(), vec, payload);
      insertId = '';
      insertVec = '';
      insertPayload = '';
      await loadIndexes();
    } catch (e) {
      error = e.message;
    } finally {
      inserting = false;
    }
  }

  async function doSearch() {
    error = '';
    searchResults = [];
    if (!selected) return;
    const vec = parseVector(searchVec);
    if (!vec) { error = '查询向量格式无效'; return; }
    const idx = currentIndex();
    if (idx && vec.length !== idx.dim) {
      error = `查询向量维度不匹配: 需要 ${idx.dim}, 实际 ${vec.length}`;
      return;
    }
    searching = true;
    try {
      searchResults = (await searchVector(selected, vec, Number(searchTopK) || 10)) || [];
    } catch (e) {
      error = e.message;
    } finally {
      searching = false;
    }
  }

  async function doDelete(id) {
    if (!confirm(`删除向量 ${id} ?`)) return;
    try {
      await deleteVector(selected, id);
      searchResults = searchResults.filter(r => r.id !== id);
      await loadIndexes();
    } catch (e) {
      error = e.message;
    }
  }

  function fillRandom(target) {
    const idx = currentIndex();
    if (!idx) return;
    const arr = Array.from({ length: idx.dim }, () => (Math.random() * 2 - 1).toFixed(4));
    if (target === 'insert') insertVec = '[' + arr.join(', ') + ']';
    else searchVec = '[' + arr.join(', ') + ']';
  }
</script>

{#if error}
  <div class="error-msg">{error}</div>
{/if}

<div class="flex justify-between items-center mb-4">
  <div class="flex items-center gap-2">
    <select class="input" style="width:260px" bind:value={selected}>
      <option value="">{indexes.length === 0 ? '暂无索引' : '选择索引'}</option>
      {#each indexes as idx}
        <option value={idx.name}>{idx.name} · dim={idx.dim} · {idx.size}条</option>
      {/each}
    </select>
    <button class="btn btn-sm" onclick={loadIndexes}>{i18n.t('common.refresh')}</button>
  </div>
  <button class="btn btn-primary" onclick={() => showCreate = true}>+ 新建索引</button>
</div>

{#if loading}
  <div class="loading">{i18n.t('status.loading')}</div>
{:else if indexes.length === 0}
  <div class="empty-state">
    <p>暂无向量索引</p>
    <p class="text-sm text-muted mt-2">点击右上角"新建索引"来创建一个 HNSW 索引</p>
  </div>
{:else if selected}
  {@const idx = currentIndex()}
  <div class="grid-cols-2 gap-4">
    <!-- 插入 -->
    <div class="card">
      <h4 class="card-title">插入向量</h4>
      <p class="text-sm text-muted mb-2">索引 <b>{idx?.name}</b> · 维度 <b>{idx?.dim}</b> · 当前 {idx?.size} 条</p>
      <div class="form-group">
        <label class="form-label">向量 ID</label>
        <input class="input" bind:value={insertId} placeholder="doc-001" />
      </div>
      <div class="form-group">
        <label class="form-label">向量 (支持 [a,b,c] 或 a,b,c)</label>
        <textarea class="input" bind:value={insertVec} rows="4" placeholder={`共 ${idx?.dim} 个浮点数`}></textarea>
      </div>
      <div class="form-group">
        <label class="form-label">Payload (JSON, 可选)</label>
        <textarea class="input" bind:value={insertPayload} rows="3" placeholder={`{"title":"...","url":"..."}`}></textarea>
      </div>
      <div class="flex gap-2">
        <button class="btn btn-sm" onclick={() => fillRandom('insert')}>随机填充</button>
        <button class="btn btn-primary btn-sm" onclick={doInsert} disabled={inserting}>
          {inserting ? '...' : '插入'}
        </button>
      </div>
    </div>

    <!-- 搜索 -->
    <div class="card">
      <h4 class="card-title">相似度搜索</h4>
      <div class="form-group">
        <label class="form-label">查询向量</label>
        <textarea class="input" bind:value={searchVec} rows="4" placeholder={`共 ${idx?.dim} 个浮点数`}></textarea>
      </div>
      <div class="form-group">
        <label class="form-label">Top K</label>
        <input class="input" type="number" min="1" max="1000" bind:value={searchTopK} />
      </div>
      <div class="flex gap-2">
        <button class="btn btn-sm" onclick={() => fillRandom('search')}>随机填充</button>
        <button class="btn btn-primary btn-sm" onclick={doSearch} disabled={searching}>
          {searching ? '...' : '搜索'}
        </button>
      </div>
    </div>
  </div>

  {#if searchResults.length > 0}
    <div class="card mt-4">
      <h4 class="card-title">搜索结果 ({searchResults.length})</h4>
      <table class="table">
        <thead>
          <tr>
            <th>#</th>
            <th>ID</th>
            <th>Distance</th>
            <th>Score</th>
            <th>Payload</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          {#each searchResults as r, i}
            <tr>
              <td>{i + 1}</td>
              <td><code>{r.id}</code></td>
              <td>{r.distance?.toFixed(6)}</td>
              <td>{r.score?.toFixed(6)}</td>
              <td>
                {#if r.payload}
                  <button class="btn btn-xs" onclick={() => payloadModal = { show: true, data: r.payload }}>
                    查看 payload
                  </button>
                {:else}
                  <span class="text-muted text-sm">-</span>
                {/if}
              </td>
              <td>
                <button class="btn btn-xs btn-danger" onclick={() => doDelete(r.id)}>删除</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
{/if}

{#if payloadModal.show}
  <div class="modal-overlay" onclick={() => payloadModal = { show: false, data: null }}>
    <div class="modal modal-wide" onclick={(e) => e.stopPropagation()}>
      <div class="flex justify-between items-center mb-3">
        <h3 class="modal-title">Payload 详情</h3>
        <button class="btn-icon" onclick={() => payloadModal = { show: false, data: null }}>✕</button>
      </div>
      <pre class="payload-json">{JSON.stringify(payloadModal.data, null, 2)}</pre>
    </div>
  </div>
{/if}

{#if showCreate}
  <div class="modal-overlay" onclick={() => showCreate = false}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3 class="modal-title">新建向量索引</h3>
      <div class="form-group">
        <label class="form-label">索引名</label>
        <input class="input" bind:value={newIndex.name} placeholder="my_index" />
      </div>
      <div class="form-group">
        <label class="form-label">维度</label>
        <input class="input" type="number" min="1" max="8192" bind:value={newIndex.dim} />
      </div>
      <div class="form-group">
        <label class="form-label">距离度量</label>
        <select class="input" bind:value={newIndex.metric}>
          <option value="cosine">cosine (余弦)</option>
          <option value="l2">l2 (欧式)</option>
          <option value="dot">dot (点积)</option>
        </select>
      </div>
      <div class="flex justify-between mt-4">
        <button class="btn" onclick={() => showCreate = false}>{i18n.t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={doCreate}>{i18n.t('common.create')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .grid-cols-2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
  }
  code {
    font-family: var(--font-mono, monospace);
    font-size: 12px;
    background: var(--bg-hover);
    padding: 2px 6px;
    border-radius: 4px;
  }
  .modal-wide {
    max-width: 640px;
    width: 90%;
  }
  .payload-json {
    margin: 0;
    padding: 16px;
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-family: var(--font-mono, monospace);
    font-size: 12.5px;
    line-height: 1.7;
    white-space: pre-wrap;
    word-break: break-all;
    max-height: 60vh;
    overflow-y: auto;
  }
</style>
