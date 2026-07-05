<script>
  import { onMount } from 'svelte';
  import { executeQuery } from '../lib/api.js';
  import { i18n } from '../lib/stores.js';

  let loading = $state(true);
  let error = $state('');

  let stats = $state({
    sessions: 0,
    activeSessions: 0,
    memories: 0,
    decisions: 0,
    avgDecisionMs: 0,
  });

  let topAgents = $state([]);
  let recentDecisions = $state([]);
  let memoryByType = $state({ short_term: 0, long_term: 0, working: 0 });

  onMount(refresh);

  async function refresh() {
    loading = true;
    try {
      // 会话统计
      try {
        const r = await executeQuery('SELECT COUNT(*) as cnt FROM sessions');
        stats.sessions = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.sessions = 0; }

      try {
        const r = await executeQuery("SELECT COUNT(*) as cnt FROM sessions WHERE state = 'active'");
        stats.activeSessions = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.activeSessions = 0; }

      // 记忆统计
      try {
        const r = await executeQuery('SELECT COUNT(*) as cnt FROM memories');
        stats.memories = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.memories = 0; }

      try {
        const r = await executeQuery('SELECT type, COUNT(*) as cnt FROM memories GROUP BY type');
        memoryByType = { short_term: 0, long_term: 0, working: 0 };
        for (const row of r?.Rows || []) {
          memoryByType[row.Values?.type || 'short_term'] = row.Values?.cnt || 0;
        }
      } catch { /* ignore */ }

      // 决策统计
      try {
        const r = await executeQuery('SELECT COUNT(*) as cnt FROM decisions');
        stats.decisions = r?.Rows?.[0]?.Values?.cnt || 0;
      } catch { stats.decisions = 0; }

      try {
        const r = await executeQuery('SELECT AVG(duration_ms) as avg_ms FROM decisions WHERE duration_ms > 0');
        stats.avgDecisionMs = Math.round(r?.Rows?.[0]?.Values?.avg_ms || 0);
      } catch { stats.avgDecisionMs = 0; }

      // Top Agents (按会话数)
      try {
        const r = await executeQuery('SELECT agent_id, COUNT(*) as cnt FROM sessions GROUP BY agent_id ORDER BY cnt DESC LIMIT 5');
        topAgents = (r?.Rows || []).map(row => ({
          agentId: row.Values?.agent_id,
          count: row.Values?.cnt,
        }));
      } catch { topAgents = []; }

      // 最近决策
      try {
        const r = await executeQuery('SELECT id, type, session_id, duration_ms, created_at FROM decisions ORDER BY created_at DESC LIMIT 8');
        recentDecisions = (r?.Rows || []).map(row => ({
          id: row.Values?.id,
          type: row.Values?.type,
          sessionId: row.Values?.session_id,
          durationMs: row.Values?.duration_ms,
          createdAt: row.Values?.created_at,
        }));
      } catch { recentDecisions = []; }

      error = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  let copied = $state(false);

  // 快速开始 tab: 'cli' | 'http'
  let tutorialTab = $state('cli');

  const tutorialTabs = [
    { id: 'cli', label: i18n.t('ai.tab.cli') },
    { id: 'http', label: i18n.t('ai.tab.http') },
  ];

  function currentTutorialMarkdown() {
    return tutorialTab === 'http' ? i18n.t('ai.tutorial.http') : i18n.t('ai.tutorial.cli');
  }

  async function copyTutorial() {
    try {
      await navigator.clipboard.writeText(currentTutorialMarkdown());
      copied = true;
      setTimeout(() => copied = false, 2000);
    } catch (e) {
      // 兜底:选中文本
      const el = document.getElementById('ai-tutorial-md');
      if (el) {
        const selection = window.getSelection();
        selection.selectAllChildren(el);
      }
    }
  }
</script>

{#if error}
  <div class="error-msg">{error}</div>
{/if}

<h1 class="page-title-lg">{i18n.t('ai.title')}</h1>

<!-- 统计卡片 -->
<div class="stat-grid">
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--green-light); color: var(--green);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.activeSessions}</div>
      <div class="stat-label">{i18n.t('ai.stats.activeSessions')}</div>
    </div>
  </div>
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--purple-light); color: var(--purple);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.memories}</div>
      <div class="stat-label">{i18n.t('ai.stats.totalMemories')}</div>
    </div>
  </div>
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--orange-light); color: var(--orange);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><circle cx="12" cy="17" r="0.5"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.decisions}</div>
      <div class="stat-label">{i18n.t('ai.stats.totalDecisions')}</div>
    </div>
  </div>
  <div class="stat-card">
    <div class="stat-icon" style="background: var(--blue-light); color: var(--blue);">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
    </div>
    <div class="stat-info">
      <div class="stat-value">{stats.avgDecisionMs}ms</div>
      <div class="stat-label">{i18n.t('ai.stats.avgDecisionTime')}</div>
    </div>
  </div>
</div>

<div class="grid-2 mt-4">
  <!-- 记忆类型分布 -->
  <div class="card">
    <h3 class="card-title">{i18n.t('ai.stats.memoryByType')}</h3>
    {#if loading}
      <div class="loading">{i18n.t('status.loading')}</div>
    {:else}
      <div class="bar-chart">
        <div class="bar-item">
          <span class="bar-label">short_term</span>
          <div class="bar-track">
            <div class="bar-fill" style="width: {stats.memories > 0 ? (memoryByType.short_term / stats.memories * 100) : 0}%; background: var(--blue);"></div>
          </div>
          <span class="bar-value">{memoryByType.short_term}</span>
        </div>
        <div class="bar-item">
          <span class="bar-label">long_term</span>
          <div class="bar-track">
            <div class="bar-fill" style="width: {stats.memories > 0 ? (memoryByType.long_term / stats.memories * 100) : 0}%; background: var(--green);"></div>
          </div>
          <span class="bar-value">{memoryByType.long_term}</span>
        </div>
        <div class="bar-item">
          <span class="bar-label">working</span>
          <div class="bar-track">
            <div class="bar-fill" style="width: {stats.memories > 0 ? (memoryByType.working / stats.memories * 100) : 0}%; background: var(--orange);"></div>
          </div>
          <span class="bar-value">{memoryByType.working}</span>
        </div>
      </div>
    {/if}
  </div>

  <!-- Top Agents -->
  <div class="card">
    <h3 class="card-title">{i18n.t('ai.stats.topAgents')}</h3>
    {#if loading}
      <div class="loading">{i18n.t('status.loading')}</div>
    {:else if topAgents.length === 0}
      <div class="empty-state-sm">暂无</div>
    {:else}
      <div class="agent-list">
        {#each topAgents as agent}
          <div class="agent-row">
            <code class="mono-sm">{agent.agentId}</code>
            <span class="text-secondary">{agent.count} 会话</span>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</div>

<!-- 最近决策 -->
<div class="card mt-4">
  <h3 class="card-title">{i18n.t('ai.stats.recentDecisions')}</h3>
  {#if loading}
    <div class="loading">{i18n.t('status.loading')}</div>
  {:else if recentDecisions.length === 0}
    <div class="empty-state-sm">暂无</div>
  {:else}
    <table class="table">
      <thead>
        <tr>
          <th>类型</th>
          <th>会话 ID</th>
          <th>耗时</th>
          <th>时间</th>
        </tr>
      </thead>
      <tbody>
        {#each recentDecisions as d}
          <tr>
            <td><span class="badge badge-info">{d.type}</span></td>
            <td><code class="mono-sm">{d.sessionId?.slice(0, 16)}...</code></td>
            <td>{d.durationMs}ms</td>
            <td class="text-sm text-secondary">{d.createdAt ? new Date(d.createdAt).toLocaleString() : '-'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

<!-- 快速开始教程 -->
<div class="card mt-4">
  <div class="flex justify-between items-center mb-3">
    <h3 class="card-title">{i18n.t('ai.tutorial')}</h3>
    <button class="btn btn-sm btn-primary" onclick={copyTutorial}>
      {copied ? '已复制' : i18n.t('ai.copy')}
    </button>
  </div>

  <div class="tabs mb-3">
    {#each tutorialTabs as tab}
      <button
        class="tab"
        class:active={tutorialTab === tab.id}
        onclick={() => tutorialTab = tab.id}
      >
        {tab.label}
      </button>
    {/each}
  </div>

  <pre id="ai-tutorial-md" class="tutorial-md">{currentTutorialMarkdown()}</pre>
</div>

<style>
  .page-title-lg {
    font-size: 24px;
    font-weight: 700;
    margin: 0 0 24px;
    color: var(--text-primary);
  }
  .stat-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 16px;
  }
  .stat-card {
    background: var(--bg-surface);
    border-radius: var(--radius);
    padding: 20px;
    display: flex;
    align-items: center;
    gap: 14px;
    box-shadow: var(--shadow-sm);
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
  }
  .stat-label {
    font-size: 12px;
    color: var(--text-tertiary);
    font-weight: 500;
    margin-top: 2px;
  }
  .grid-2 {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  .mt-4 {
    margin-top: 24px;
  }
  .bar-chart {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .bar-item {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .bar-label {
    width: 80px;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary);
  }
  .bar-track {
    flex: 1;
    height: 8px;
    background: var(--bg-hover);
    border-radius: 4px;
    overflow: hidden;
  }
  .bar-fill {
    height: 100%;
    border-radius: 4px;
    transition: width 0.3s;
  }
  .bar-value {
    width: 40px;
    text-align: right;
    font-size: 12px;
    font-weight: 600;
  }
  .agent-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .agent-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: var(--bg-base);
    border-radius: var(--radius-sm);
    font-size: 13px;
  }
  .empty-state-sm {
    padding: 20px;
    text-align: center;
    color: var(--text-tertiary);
    font-size: 13px;
  }
  .tabs {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid var(--border);
    padding-bottom: 8px;
  }
  .tab {
    padding: 6px 14px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
  }
  .tab:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .tab.active {
    background: var(--bg-active);
    color: var(--accent);
  }
  .tutorial-md {
    padding: 16px;
    background: var(--bg-base);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-family: var(--font-mono, monospace);
    font-size: 12.5px;
    line-height: 1.7;
    white-space: pre-wrap;
    overflow-x: auto;
    color: var(--text-primary);
    max-height: 640px;
    overflow-y: auto;
  }
  .mono-sm {
    font-family: var(--font-mono, monospace);
    font-size: 11px;
    background: var(--bg-hover);
    padding: 2px 6px;
    border-radius: 4px;
  }
  @media (max-width: 768px) {
    .stat-grid { grid-template-columns: repeat(2, 1fr); }
    .grid-2 { grid-template-columns: 1fr; }
  }
</style>
