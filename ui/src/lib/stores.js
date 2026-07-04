import { writable } from 'svelte/store';
import { createI18n } from './i18n.svelte.js';

// 国际化实例
export const i18n = createI18n('zh');

// 当前页面路由
export const currentPage = writable('dashboard');

// 连接状态
export const connected = writable(false);

// 侧边栏折叠状态
export const sidebarCollapsed = writable(false);
