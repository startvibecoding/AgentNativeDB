<script>
  import { onMount } from 'svelte';
  import { listMemories, storeMemory, deleteMemory, listSessions } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let sessions = $state([]);
  let selectedSession = $state('');
  let memories = $state([]);
  let error = $state('');
  let loading = $state(false);
  let showStoreModal = $state(false);
  let newMemory = $state({ type: 'short_term', content: '', importance: 0.5 });

  onMount(async () => {
    try {
      sessions = await listSessions(undefined, 50) || [];
      if (sessions.length > 0) {
        selectedSession = sessions[0].id;
        await loadMemories();
      }
    } catch (e) {
      error = e.message;
    }
  });

  async function loadMemories() {
    if (!selectedSession) return;
    try {
      loading = true;
      memories = await listMemories(selectedSession) || [];
      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function storeNewMemory() {
    if (!newMemory.content.trim() || !selectedSession) return;
    try {
      await storeMemory(selectedSession, newMemory.type, newMemory.content, newMemory.importance);
      showStoreModal = false;
      newMemory = { type: 'short_term', content: '', importance: 0.5 };
      await loadMemories();
    } catch (e) {
      error = e.message;
    }
  }

  async function deleteMem(id) {
    if (!confirm(i18n.t('memories.deleteConfirm') || 'Delete this memory?')) return;
    try {
      await deleteMemory(id);
      await loadMemories();
    } catch (e) {
      error = e.message;
    }
  }

  function typeBadge(t) {
    const m = { short_term: 'badge-info', long_term: 'badge-success', working: 'badge-warning' };
    return m[t] || 'badge-default';
  }

  function typeLabel(t) {
    const m = {
      short_term: i18n.t('memories.type.short'),
      long_term: i18n.t('memories.type.long'),
      working: i18n.t('memories.type.working'),
    };
    return m[t] || t;
  }

  function formatDate(v) {
    if (!v) return '-';
    return new Date(v).toLocaleString();
  }
</script>

{#if error}
  <div class="error-msg">{error}</div>
{/if}

<div class="flex justify-between items-center mb-4">
  <div class="flex items-center gap-2">
    <select class="input" style="width:240px" bind:value={selectedSession} onchange={loadMemories}>
      <option value="">{i18n.t('memories.selectSession')}</option>
      {#each sessions as s}
        <option value={s.id}>{s.agent_id} — {s.id.slice(0, 8)}…</option>
      {/each}
    </select>
    <button class="btn btn-sm" onclick={loadMemories} disabled={!selectedSession}>{i18n.t('common.refresh')}</button>
  </div>
  <button class="btn btn-primary" onclick={() => showStoreModal = true} disabled={!selectedSession}>+ {i18n.t('memories.add')}</button>
</div>

{#if loading}
  <div class="loading">{i18n.t('status.loading')}</div>
{:else if memories.length === 0}
  <div class="empty-state">
    <p>{selectedSession ? i18n.t('memories.empty') : i18n.t('memories.selectHint')}</p>
  </div>
{:else}
  <div class="memories-grid">
    {#each memories as m}
      <div class="memory-card card">
        <div class="flex justify-between items-center mb-2">
          <span class="badge {typeBadge(m.type)}">{typeLabel(m.type)}</span>
          <button class="btn-icon" onclick={() => deleteMem(m.id)} title={i18n.t('common.delete')}>✕</button>
        </div>
        <p class="memory-content">{m.content}</p>
        <div class="memory-meta mt-2">
          <span class="text-sm text-muted">{i18n.t('memories.importance')}: {(m.importance * 100).toFixed(0)}%</span>
          <span class="text-sm text-muted">{i18n.t('memories.accessCount')}: {m.access_count} {i18n.t('common.units.times')}</span>
        </div>
        <div class="text-sm text-muted mt-2">{formatDate(m.created_at)}</div>
      </div>
    {/each}
  </div>
{/if}

{#if showStoreModal}
  <div class="modal-overlay" onclick={() => showStoreModal = false}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3 class="modal-title">{i18n.t('memories.addTitle')}</h3>
      <div class="form-group">
        <label class="form-label">{i18n.t('memories.type')}</label>
        <select class="input" bind:value={newMemory.type}>
          <option value="short_term">{i18n.t('memories.type.short')}</option>
          <option value="long_term">{i18n.t('memories.type.long')}</option>
          <option value="working">{i18n.t('memories.type.working')}</option>
        </select>
      </div>
      <div class="form-group">
        <label class="form-label">{i18n.t('memories.content')}</label>
        <textarea class="input" bind:value={newMemory.content} rows="4" placeholder={i18n.t('memories.contentPlaceholder')}></textarea>
      </div>
      <div class="form-group">
        <label class="form-label">{i18n.t('memories.importance')}: {(newMemory.importance * 100).toFixed(0)}%</label>
        <input type="range" min="0" max="1" step="0.05" bind:value={newMemory.importance} style="width:100%" />
      </div>
      <div class="flex justify-between mt-4">
        <button class="btn" onclick={() => showStoreModal = false}>{i18n.t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={storeNewMemory}>{i18n.t('common.add')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .memories-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 16px;
  }
  .memory-card {
    display: flex;
    flex-direction: column;
  }
  .memory-content {
    font-size: 14px;
    line-height: 1.6;
    color: var(--text-primary);
    flex: 1;
  }
  .memory-meta {
    display: flex;
    gap: 16px;
  }
</style>
