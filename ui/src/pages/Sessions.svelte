<script>
  import { onMount } from 'svelte';
  import { listSessions, getSession, createSession, updateSession, deleteSession } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let sessions = $state([]);
  let selectedSession = $state(null);
  let error = $state('');
  let loading = $state(true);
  let showCreateModal = $state(false);
  let newAgentId = $state('');
  let filterAgent = $state('');

  onMount(loadSessions);

  async function loadSessions() {
    try {
      loading = true;
      sessions = await listSessions(filterAgent || undefined, 100) || [];
      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function selectSession(id) {
    try {
      selectedSession = await getSession(id);
    } catch (e) {
      error = e.message;
    }
  }

  async function createNewSession() {
    if (!newAgentId.trim()) return;
    try {
      await createSession(newAgentId);
      showCreateModal = false;
      newAgentId = '';
      await loadSessions();
    } catch (e) {
      error = e.message;
    }
  }

  async function deleteSess(id) {
    if (!confirm(i18n.t('sessions.deleteConfirm') || 'Delete this session?')) return;
    try {
      await deleteSession(id);
      if (selectedSession?.id === id) selectedSession = null;
      await loadSessions();
    } catch (e) {
      error = e.message;
    }
  }

  async function updateState(id, state) {
    try {
      await updateSession(id, state);
      await loadSessions();
      if (selectedSession?.id === id) {
        selectedSession = await getSession(id);
      }
    } catch (e) {
      error = e.message;
    }
  }

  function stateBadge(s) {
    const m = { active: 'badge-success', paused: 'badge-warning', completed: 'badge-info', failed: 'badge-danger' };
    return m[s] || 'badge-default';
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
    <input class="input" style="width:200px" bind:value={filterAgent} placeholder={i18n.t('sessions.filterHint')} onkeydown={(e) => e.key === 'Enter' && loadSessions()} />
    <button class="btn btn-sm" onclick={loadSessions}>{i18n.t('common.refresh')}</button>
  </div>
  <button class="btn btn-primary" onclick={() => showCreateModal = true}>+ {i18n.t('sessions.create')}</button>
</div>

<div class="sessions-layout">
  <div class="sessions-list">
    {#if loading}
      <div class="loading">{i18n.t('status.loading')}</div>
    {:else if sessions.length === 0}
      <div class="empty-state"><p>{i18n.t('sessions.empty')}</p></div>
    {:else}
      <div class="table-container">
        <table>
          <thead>
            <tr>
              <th>{i18n.t('sessions.agent')}</th>
              <th>{i18n.t('sessions.state')}</th>
              <th>{i18n.t('sessions.createdAt')}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each sessions as s}
              <tr
                class:row-active={selectedSession?.id === s.id}
                onclick={() => selectSession(s.id)}
                style="cursor:pointer"
              >
                <td class="mono text-sm">{s.agent_id}</td>
                <td><span class="badge {stateBadge(s.state)}">{s.state}</span></td>
                <td class="text-sm text-muted">{formatDate(s.created_at)}</td>
                <td>
                  <button class="btn-icon" onclick={(e) => { e.stopPropagation(); deleteSess(s.id); }} title={i18n.t('common.delete')}>✕</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <div class="session-detail">
    {#if !selectedSession}
      <div class="empty-state"><p>{i18n.t('sessions.selectHint')}</p></div>
    {:else}
      <div class="card">
        <h3 class="card-title">{i18n.t('sessions.detail')}</h3>
        <div class="info-grid">
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('sessions.id')}</span>
            <span class="mono text-sm">{selectedSession.id}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('sessions.agent')}</span>
            <span class="mono text-sm">{selectedSession.agent_id}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('sessions.state')}</span>
            <span class="badge {stateBadge(selectedSession.state)}">{selectedSession.state}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('sessions.createdAt')}</span>
            <span class="text-sm">{formatDate(selectedSession.created_at)}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('sessions.updatedAt')}</span>
            <span class="text-sm">{formatDate(selectedSession.updated_at)}</span>
          </div>
        </div>

        <div class="mt-4">
          <span class="text-muted text-sm">{i18n.t('sessions.actions')}</span>
          <div class="flex gap-2 mt-2">
            {#if selectedSession.state !== 'active'}
              <button class="btn btn-sm" onclick={() => updateState(selectedSession.id, 'active')}>{i18n.t('sessions.activate')}</button>
            {/if}
            {#if selectedSession.state !== 'paused'}
              <button class="btn btn-sm" onclick={() => updateState(selectedSession.id, 'paused')}>{i18n.t('sessions.pause')}</button>
            {/if}
            {#if selectedSession.state !== 'completed'}
              <button class="btn btn-sm" onclick={() => updateState(selectedSession.id, 'completed')}>{i18n.t('sessions.complete')}</button>
            {/if}
          </div>
        </div>

        {#if selectedSession.context && Object.keys(selectedSession.context).length > 0}
          <div class="mt-4">
            <span class="text-muted text-sm">{i18n.t('sessions.context')}</span>
            <pre class="json-view mt-2">{JSON.stringify(selectedSession.context, null, 2)}</pre>
          </div>
        {/if}

        {#if selectedSession.metadata && Object.keys(selectedSession.metadata).length > 0}
          <div class="mt-4">
            <span class="text-muted text-sm">{i18n.t('sessions.metadata')}</span>
            <pre class="json-view mt-2">{JSON.stringify(selectedSession.metadata, null, 2)}</pre>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

{#if showCreateModal}
  <div class="modal-overlay" onclick={() => showCreateModal = false}>
    <div class="modal" onclick={(e) => e.stopPropagation()}>
      <h3 class="modal-title">{i18n.t('sessions.createTitle')}</h3>
      <div class="form-group">
        <label class="form-label">{i18n.t('sessions.agentId')}</label>
        <input class="input" bind:value={newAgentId} placeholder={i18n.t('sessions.agentIdHint')} />
      </div>
      <div class="flex justify-between mt-4">
        <button class="btn" onclick={() => showCreateModal = false}>{i18n.t('common.cancel')}</button>
        <button class="btn btn-primary" onclick={createNewSession}>{i18n.t('common.create')}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .sessions-layout {
    display: flex;
    gap: 16px;
    height: calc(100vh - var(--header-height) - 48px - 56px);
  }
  .sessions-list {
    flex: 1;
    overflow-y: auto;
    border-radius: var(--radius);
    background: var(--bg-surface);
    box-shadow: var(--shadow-sm);
    min-width: 0;
  }
  .session-detail {
    width: 380px;
    flex-shrink: 0;
    overflow-y: auto;
  }
  .row-active td {
    background: var(--bg-active) !important;
  }
</style>
