<script>
  import { onMount } from 'svelte';
  import { getClusterStatus, addPeer, removePeer } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let cluster = $state(null);
  let error = $state('');
  let loading = $state(true);
  let showAddPeer = $state(false);
  let newPeerId = $state('');
  let newPeerAddr = $state('');
  let actionError = $state('');

  onMount(async () => {
    await refresh();
    const t = setInterval(refresh, 5000);
    return () => clearInterval(t);
  });

  async function refresh() {
    try {
      cluster = await getClusterStatus();
      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function handleAddPeer() {
    actionError = '';
    if (!newPeerId || !newPeerAddr) {
      actionError = i18n.t('cluster.fillAllFields');
      return;
    }
    try {
      await addPeer(newPeerId, newPeerAddr);
      newPeerId = '';
      newPeerAddr = '';
      showAddPeer = false;
      await refresh();
    } catch (e) {
      actionError = e.message;
    }
  }

  async function handleRemovePeer(nodeId) {
    if (!confirm(i18n.t('cluster.confirmRemove', { id: nodeId }))) return;
    try {
      await removePeer(nodeId);
      await refresh();
    } catch (e) {
      error = e.message;
    }
  }

  function stateLabel(state) {
    const labels = {
      leader: i18n.t('cluster.stateLeader'),
      follower: i18n.t('cluster.stateFollower'),
      candidate: i18n.t('cluster.stateCandidate'),
    };
    return labels[state] || state;
  }

  function stateColor(state) {
    if (state === 'leader') return 'var(--green)';
    if (state === 'candidate') return 'var(--orange)';
    return 'var(--blue)';
  }
</script>

{#if loading}
  <div class="loading">{i18n.t('common.loading')}</div>
{:else if error && !cluster}
  <div class="error-msg">{error}</div>
{:else if !cluster || !cluster.node_id}
  <div class="card">
    <div class="empty-state">
      <div class="empty-icon">◯</div>
      <h3>{i18n.t('cluster.notEnabled')}</h3>
      <p>{i18n.t('cluster.notEnabledDesc')}</p>
    </div>
  </div>
{:else}
  <div class="stat-grid">
    <div class="stat-card">
      <div class="stat-icon" style="background: var(--blue-light); color: var(--blue);">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <circle cx="12" cy="12" r="3"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="stat-value">{Object.keys(cluster.peers || {}).length}</div>
        <div class="stat-label">{i18n.t('cluster.nodes')}</div>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon" style="background: var(--green-light); color: var(--green);">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2L2 7l10 5 10-5-10-5z"/>
          <path d="M2 17l10 5 10-5"/>
          <path d="M2 12l10 5 10-5"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="stat-value">{cluster.term}</div>
        <div class="stat-label">{i18n.t('cluster.currentTerm')}</div>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon" style="background: var(--purple-light); color: var(--purple);">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="20 6 9 17 4 12"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="stat-value">{cluster.commit_index}</div>
        <div class="stat-label">{i18n.t('cluster.commitIndex')}</div>
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-icon" style="background: var(--orange-light); color: var(--orange);">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/>
        </svg>
      </div>
      <div class="stat-info">
        <div class="stat-value">
          <span class="status-badge" style="background: {stateColor(cluster.state)}20; color: {stateColor(cluster.state)}">
            {stateLabel(cluster.state)}
          </span>
        </div>
        <div class="stat-label">{i18n.t('cluster.nodeState')}</div>
      </div>
    </div>
  </div>

  <div class="grid-2">
    <div class="card">
      <div class="card-header">
        <h3 class="card-title">{i18n.t('cluster.nodeInfo')}</h3>
      </div>
      <div class="info-grid">
        <div class="info-item">
          <span class="text-secondary">{i18n.t('cluster.nodeId')}</span>
          <span class="font-mono">{cluster.node_id}</span>
        </div>
        <div class="info-item">
          <span class="text-secondary">{i18n.t('cluster.leaderId')}</span>
          <span class="font-mono">{cluster.leader_id || '-'}</span>
        </div>
        <div class="info-item">
          <span class="text-secondary">{i18n.t('cluster.leaderAddr')}</span>
          <span class="font-mono">{cluster.leader_addr || '-'}</span>
        </div>
        <div class="info-item">
          <span class="text-secondary">{i18n.t('cluster.lastApplied')}</span>
          <span class="font-mono">{cluster.last_applied}</span>
        </div>
        <div class="info-item">
          <span class="text-secondary">{i18n.t('cluster.state')}</span>
          <span class="badge" style="background: {stateColor(cluster.state)}20; color: {stateColor(cluster.state)}">
            {stateLabel(cluster.state)}
          </span>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <h3 class="card-title">{i18n.t('cluster.peers')}</h3>
        {#if cluster.state === 'leader'}
          <button class="btn-primary-small" onclick={() => showAddPeer = true}>
            + {i18n.t('cluster.addPeer')}
          </button>
        {/if}
      </div>

      {#if showAddPeer}
        <div class="add-peer-form">
          <input type="text" bind:value={newPeerId} placeholder={i18n.t('cluster.peerId')} class="input" />
          <input type="text" bind:value={newPeerAddr} placeholder="host:port" class="input" />
          {#if actionError}<div class="form-error">{actionError}</div>{/if}
          <div class="form-actions">
            <button class="btn-secondary-small" onclick={() => showAddPeer = false}>{i18n.t('common.cancel')}</button>
            <button class="btn-primary-small" onclick={handleAddPeer}>{i18n.t('common.add')}</button>
          </div>
        </div>
      {/if}

      <div class="peer-list">
        {#each Object.entries(cluster.peers || {}) as [id, addr]}
          <div class="peer-item">
            <div class="peer-info">
              <div class="peer-name">
                {#if id === cluster.leader_id}
                  <span class="leader-badge">♛</span>
                {/if}
                <span class="font-mono">{id}</span>
                {#if id === cluster.node_id}
                  <span class="badge badge-self">{i18n.t('cluster.self')}</span>
                {/if}
              </div>
              <div class="peer-addr font-mono">{addr}</div>
            </div>
            {#if cluster.state === 'leader' && id !== cluster.node_id}
              <button class="btn-danger-small" onclick={() => handleRemovePeer(id)}>
                {i18n.t('common.remove')}
              </button>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .loading {
    text-align: center;
    padding: 40px;
    color: var(--text-secondary);
  }
  .empty-state {
    text-align: center;
    padding: 48px 24px;
  }
  .empty-icon {
    font-size: 48px;
    margin-bottom: 16px;
    opacity: 0.3;
  }
  .empty-state h3 {
    margin: 0 0 8px 0;
    color: var(--text-primary);
  }
  .empty-state p {
    margin: 0;
    color: var(--text-secondary);
    font-size: 14px;
  }
  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;
  }
  .card-title {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  .status-badge {
    padding: 2px 10px;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 600;
  }
  .add-peer-form {
    padding: 16px;
    background: var(--bg-secondary);
    border-radius: var(--radius-sm);
    margin-bottom: 16px;
  }
  .input {
    width: 100%;
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--bg-primary);
    color: var(--text-primary);
    font-family: var(--font-sans);
    font-size: 13px;
    margin-bottom: 8px;
    box-sizing: border-box;
  }
  .input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .form-error {
    color: var(--red);
    font-size: 12px;
    margin-bottom: 8px;
  }
  .form-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .peer-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .peer-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px;
    background: var(--bg-secondary);
    border-radius: var(--radius-sm);
  }
  .peer-name {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 500;
    margin-bottom: 4px;
  }
  .peer-addr {
    font-size: 12px;
    color: var(--text-tertiary);
  }
  .leader-badge {
    color: var(--orange);
    font-size: 14px;
  }
  .badge-self {
    background: var(--blue-light);
    color: var(--blue);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 10px;
    font-weight: 600;
  }
  .btn-primary-small, .btn-secondary-small, .btn-danger-small {
    padding: 5px 12px;
    border: none;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    font-family: var(--font-sans);
    cursor: pointer;
    transition: all 0.15s;
  }
  .btn-primary-small {
    background: var(--accent);
    color: white;
  }
  .btn-primary-small:hover {
    opacity: 0.9;
  }
  .btn-secondary-small {
    background: var(--bg-hover);
    color: var(--text-secondary);
  }
  .btn-secondary-small:hover {
    background: var(--border);
    color: var(--text-primary);
  }
  .btn-danger-small {
    background: var(--red-light);
    color: var(--red);
  }
  .btn-danger-small:hover {
    background: var(--red);
    color: white;
  }
  .font-mono {
    font-family: var(--font-mono);
    font-size: 12px;
  }
  .text-secondary {
    color: var(--text-secondary);
    font-size: 13px;
  }
  .info-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
  }
  .info-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .badge {
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    display: inline-block;
    width: fit-content;
  }
</style>
