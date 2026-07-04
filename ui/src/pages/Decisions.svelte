<script>
  import { onMount } from 'svelte';
  import { listDecisions, getDecision, deleteDecision, getDecisionTree, listSessions } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let sessions = $state([]);
  let selectedSession = $state('');
  let decisions = $state([]);
  let selectedDecision = $state(null);
  let decisionTree = $state(null);
  let error = $state('');
  let loading = $state(false);
  let treeView = $state(false);

  onMount(async () => {
    try {
      sessions = await listSessions(undefined, 50) || [];
      if (sessions.length > 0) {
        selectedSession = sessions[0].id;
        await loadDecisions();
      }
    } catch (e) {
      error = e.message;
    }
  });

  async function loadDecisions() {
    if (!selectedSession) return;
    try {
      loading = true;
      decisions = await listDecisions(selectedSession) || [];
      selectedDecision = null;
      decisionTree = null;
      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function selectDecision(id) {
    try {
      selectedDecision = await getDecision(id);
    } catch (e) {
      error = e.message;
    }
  }

  async function loadTree(id) {
    try {
      decisionTree = await getDecisionTree(id);
      treeView = true;
    } catch (e) {
      error = e.message;
    }
  }

  async function deleteDec(id) {
    if (!confirm('Delete this decision?')) return;
    try {
      await deleteDecision(id);
      if (selectedDecision?.id === id) selectedDecision = null;
      await loadDecisions();
    } catch (e) {
      error = e.message;
    }
  }

  function typeBadge(t) {
    const m = { reasoning: 'badge-info', tool_call: 'badge-warning', planning: 'badge-success', reflection: 'badge-purple' };
    return m[t] || 'badge-default';
  }

  function typeLabel(t) {
    const m = {
      reasoning: i18n.t('decisions.type.reasoning'),
      tool_call: i18n.t('decisions.type.tool_call'),
      planning: i18n.t('decisions.type.planning'),
      reflection: i18n.t('decisions.type.reflection'),
    };
    return m[t] || t;
  }

  function formatDate(v) {
    if (!v) return '-';
    return new Date(v).toLocaleString();
  }

  function renderTree(node, depth = 0) {
    if (!node) return [];
    let items = [{ decision: node.decision, depth }];
    if (node.children) {
      for (const child of node.children) {
        items = items.concat(renderTree(child, depth + 1));
      }
    }
    return items;
  }
</script>

{#if error}
  <div class="error-msg">{error}</div>
{/if}

<div class="flex justify-between items-center mb-4">
  <div class="flex items-center gap-2">
    <select class="input" style="width:240px" bind:value={selectedSession} onchange={loadDecisions}>
      <option value="">{i18n.t('decisions.selectSession')}</option>
      {#each sessions as s}
        <option value={s.id}>{s.agent_id} — {s.id.slice(0, 8)}…</option>
      {/each}
    </select>
    <button class="btn btn-sm" onclick={loadDecisions} disabled={!selectedSession}>{i18n.t('common.refresh')}</button>
  </div>
  <div class="flex gap-2">
    {#if selectedDecision}
      <button class="btn btn-sm" onclick={() => loadTree(selectedDecision.id)}>🌳 {i18n.t('decisions.viewTree')}</button>
    {/if}
  </div>
</div>

<div class="decisions-layout">
  <div class="decisions-list">
    {#if loading}
      <div class="loading">{i18n.t('status.loading')}</div>
    {:else if decisions.length === 0}
      <div class="empty-state"><p>{selectedSession ? i18n.t('decisions.empty') : i18n.t('decisions.selectHint')}</p></div>
    {:else}
      <div class="table-container">
        <table>
          <thead>
            <tr>
              <th>{i18n.t('decisions.type')}</th>
              <th>{i18n.t('decisions.reasoning')}</th>
              <th>{i18n.t('decisions.duration')}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each decisions as d}
              <tr
                class:row-active={selectedDecision?.id === d.id}
                onclick={() => selectDecision(d.id)}
                style="cursor:pointer"
              >
                <td><span class="badge {typeBadge(d.type)}">{typeLabel(d.type)}</span></td>
                <td class="text-sm truncate" style="max-width:200px">{d.reasoning || '-'}</td>
                <td class="text-sm text-muted">{d.duration_ms ? d.duration_ms + i18n.t('common.units.ms') : '-'}</td>
                <td>
                  <button class="btn-icon" onclick={(e) => { e.stopPropagation(); deleteDec(d.id); }} title={i18n.t('common.delete')}>✕</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <div class="decision-detail">
    {#if treeView && decisionTree}
      <div class="card">
        <div class="flex justify-between items-center mb-4">
          <h3 class="card-title">{i18n.t('decisions.tree')}</h3>
          <button class="btn btn-sm" onclick={() => treeView = false}>{i18n.t('common.close')}</button>
        </div>
        {#each renderTree(decisionTree) as item}
          <div class="tree-item" style="padding-left: {item.depth * 24}px">
            <span class="badge {typeBadge(item.decision?.type)}">{typeLabel(item.decision?.type)}</span>
            <span class="text-sm truncate" style="margin-left:8px">{item.decision?.reasoning || '-'}</span>
          </div>
        {/each}
      </div>
    {:else if selectedDecision}
      <div class="card">
        <h3 class="card-title">{i18n.t('decisions.detail')}</h3>
        <div class="info-grid">
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('sessions.id')}</span>
            <span class="mono text-sm">{selectedDecision.id}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('decisions.type')}</span>
            <span class="badge {typeBadge(selectedDecision.type)}">{typeLabel(selectedDecision.type)}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('decisions.duration')}</span>
            <span class="text-sm">{selectedDecision.duration_ms ? selectedDecision.duration_ms + i18n.t('common.units.ms') : '-'}</span>
          </div>
          <div class="info-item">
            <span class="text-muted text-sm">{i18n.t('decisions.createdAt')}</span>
            <span class="text-sm">{formatDate(selectedDecision.created_at)}</span>
          </div>
        </div>

        {#if selectedDecision.reasoning}
          <div class="mt-4">
            <span class="text-muted text-sm">{i18n.t('decisions.reasoning')}</span>
            <div class="json-view mt-2">{selectedDecision.reasoning}</div>
          </div>
        {/if}

        {#if selectedDecision.tools_used?.length}
          <div class="mt-4">
            <span class="text-muted text-sm">{i18n.t('decisions.toolsUsed')}</span>
            <div class="flex gap-2 mt-2 flex-wrap">
              {#each selectedDecision.tools_used as tool}
                <span class="badge badge-default">{tool}</span>
              {/each}
            </div>
          </div>
        {/if}

        {#if selectedDecision.input}
          <div class="mt-4">
            <span class="text-muted text-sm">{i18n.t('decisions.input')}</span>
            <pre class="json-view mt-2">{typeof selectedDecision.input === 'string' ? selectedDecision.input : JSON.stringify(selectedDecision.input, null, 2)}</pre>
          </div>
        {/if}

        {#if selectedDecision.output}
          <div class="mt-4">
            <span class="text-muted text-sm">{i18n.t('decisions.output')}</span>
            <pre class="json-view mt-2">{typeof selectedDecision.output === 'string' ? selectedDecision.output : JSON.stringify(selectedDecision.output, null, 2)}</pre>
          </div>
        {/if}
      </div>
    {:else}
      <div class="empty-state"><p>{i18n.t('common.noSelection')}</p></div>
    {/if}
  </div>
</div>

<style>
  .decisions-layout {
    display: flex;
    gap: 16px;
    height: calc(100vh - var(--header-height) - 48px - 56px);
  }
  .decisions-list {
    flex: 1;
    overflow-y: auto;
    border-radius: var(--radius);
    background: var(--bg-surface);
    box-shadow: var(--shadow-sm);
    min-width: 0;
  }
  .decision-detail {
    width: 400px;
    flex-shrink: 0;
    overflow-y: auto;
  }
  .row-active td {
    background: var(--bg-active) !important;
  }
  .tree-item {
    display: flex;
    align-items: center;
    padding: 8px 0;
    border-bottom: 0.5px solid var(--border);
  }
  .tree-item:last-child { border-bottom: none; }
</style>
