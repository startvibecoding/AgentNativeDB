<script>
  import { currentPage, i18n } from '../lib/stores.js';

  let page = $state('dashboard');
  currentPage.subscribe(v => page = v);

  // 展开的分组 (默认展开)
  let expanded = $state({ 'ai-native': true });

  const navItems = $derived([
    { id: 'dashboard', label: i18n.t('nav.dashboard'), icon: '◆' },
    { id: 'cluster', label: i18n.t('nav.cluster'), icon: '◉' },
    { id: 'tables', label: i18n.t('nav.tables'), icon: '☰' },
    { id: 'sql', label: i18n.t('nav.sql'), icon: '▸' },
    { id: 'vectors', label: i18n.t('nav.vectors'), icon: '↯' },
    {
      id: 'ai-native',
      label: i18n.t('nav.aiNative'),
      icon: '✦',
      children: [
        { id: 'ai-dashboard', label: i18n.t('nav.aiDashboard'), icon: '▣' },
        { id: 'sessions', label: i18n.t('nav.sessions'), icon: '◎' },
        { id: 'memories', label: i18n.t('nav.memories'), icon: '◉' },
        { id: 'decisions', label: i18n.t('nav.decisions'), icon: '◈' },
      ],
    },
  ]);

  function navigate(id) {
    currentPage.set(id);
  }

  function toggleGroup(id) {
    expanded[id] = !expanded[id];
  }
</script>

<aside class="sidebar">
  <div class="sidebar-header">
    <div class="logo">
      <div class="logo-mark">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <rect width="24" height="24" rx="6" fill="url(#logoGrad)"/>
          <path d="M8 16V8l4 4 4-4v8" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          <defs>
            <linearGradient id="logoGrad" x1="0" y1="0" x2="24" y2="24">
              <stop stop-color="#007aff"/>
              <stop offset="1" stop-color="#5856d6"/>
            </linearGradient>
          </defs>
        </svg>
      </div>
      <span class="logo-text">AgentNativeDB</span>
    </div>
  </div>

  <nav class="sidebar-nav">
    {#each navItems as item}
      {#if item.children}
        <button
          class="nav-item nav-group"
          onclick={() => toggleGroup(item.id)}
        >
          <span class="nav-icon">{item.icon}</span>
          <span class="nav-label">{item.label}</span>
          <span class="nav-caret" class:open={expanded[item.id]}>›</span>
        </button>
        {#if expanded[item.id]}
          <div class="nav-children">
            {#each item.children as child}
              <button
                class="nav-item nav-child"
                class:active={page === child.id}
                onclick={() => navigate(child.id)}
              >
                <span class="nav-icon">{child.icon}</span>
                <span class="nav-label">{child.label}</span>
              </button>
            {/each}
          </div>
        {/if}
      {:else}
        <button
          class="nav-item"
          class:active={page === item.id}
          onclick={() => navigate(item.id)}
        >
          <span class="nav-icon">{item.icon}</span>
          <span class="nav-label">{item.label}</span>
        </button>
      {/if}
    {/each}
  </nav>

  <div class="sidebar-footer">
    <div class="version">v0.1.0</div>
  </div>
</aside>

<style>
  .sidebar {
    width: var(--sidebar-width);
    height: 100vh;
    background: var(--bg-sidebar);
    backdrop-filter: blur(24px);
    -webkit-backdrop-filter: blur(24px);
    border-right: 0.5px solid var(--border);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    overflow: hidden;
  }
  .sidebar-header {
    padding: 16px 16px 12px;
  }
  .logo {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .logo-mark {
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .logo-text {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-primary);
    white-space: nowrap;
    letter-spacing: -0.02em;
  }
  .sidebar-nav {
    flex: 1;
    padding: 4px 8px;
    overflow-y: auto;
  }
  .nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px 12px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font-family: var(--font-sans);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
    text-align: left;
    margin-bottom: 2px;
    outline: none;
  }
  .nav-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .nav-item.active {
    background: var(--bg-active);
    color: var(--accent);
  }
  .nav-icon {
    font-size: 14px;
    width: 20px;
    text-align: center;
    flex-shrink: 0;
    opacity: 0.7;
  }
  .nav-item.active .nav-icon {
    opacity: 1;
  }
  .nav-label {
    white-space: nowrap;
    flex: 1;
  }
  .nav-caret {
    font-size: 12px;
    color: var(--text-tertiary);
    transition: transform 0.15s ease;
    display: inline-block;
  }
  .nav-caret.open {
    transform: rotate(90deg);
  }
  .nav-children {
    margin-left: 12px;
    padding-left: 8px;
    border-left: 1px solid var(--border);
    margin-bottom: 4px;
  }
  .nav-child {
    padding-left: 10px;
    font-size: 12.5px;
  }
  .sidebar-footer {
    padding: 12px 16px;
    border-top: 0.5px solid var(--border);
  }
  .version {
    font-size: 11px;
    color: var(--text-tertiary);
    text-align: center;
    font-weight: 500;
  }
</style>
