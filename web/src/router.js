import { createRouter, createWebHashHistory } from 'vue-router'
import { token } from './api'

const routes = [
  { path: '/login', component: () => import('./pages/Login.vue'), meta: { public: true } },
  // 改密页：需要 token（不是 public），但走独立布局，不显示侧边栏
  { path: '/change-password', component: () => import('./pages/ChangePassword.vue'), meta: { bare: true } },
  { path: '/', component: () => import('./pages/Overview.vue'), meta: { title: '总览', group: '总览' } },
  // 家庭工作台（2026-11-07）：扁平路由，AppLayout 检测路径前缀后包进 FamilyLayout（选择器+页签）
  { path: '/workspace', redirect: '/workspace/info' },
  { path: '/workspace/info', component: () => import('./pages/FamilySettings.vue'), meta: { title: '家庭 · 信息', tab: 'info' } },
  { path: '/workspace/members', component: () => import('./pages/Members.vue'), meta: { title: '家庭 · 成员', tab: 'members' } },
  { path: '/workspace/rules', component: () => import('./pages/Rules.vue'), meta: { title: '家庭 · 睡觉规则', tab: 'rules' } },
  { path: '/workspace/items', component: () => import('./pages/Items.vue'), meta: { title: '家庭 · 兑换商城', tab: 'items' } },
  { path: '/workspace/tasks', component: () => import('./pages/Tasks.vue'), meta: { title: '家庭 · 任务', tab: 'tasks' } },
  { path: '/workspace/transfers', component: () => import('./pages/Transfers.vue'), meta: { title: '家庭 · 赠送', tab: 'transfers' } },
  { path: '/workspace/screen', component: () => import('./pages/Screen.vue'), meta: { title: '家庭 · 屏幕时间', tab: 'screen' } },
  { path: '/workspace/records', component: () => import('./pages/Records.vue'), meta: { title: '家庭 · 积分流水', tab: 'records' } },
  { path: '/workspace/stats', component: () => import('./pages/Stats.vue'), meta: { title: '家庭 · 统计', tab: 'stats' } },
  { path: '/members', redirect: '/workspace/members' },
  { path: '/rules', redirect: '/workspace/rules' },
  { path: '/items', redirect: '/workspace/items' },
  { path: '/tasks', redirect: '/workspace/tasks' },
  { path: '/transfers', redirect: '/workspace/transfers' },
  { path: '/screen', redirect: '/workspace/screen' },
  { path: '/settings', redirect: '/workspace/info' },
  { path: '/records', redirect: '/workspace/records' },
  { path: '/stats', redirect: '/workspace/stats' },
  { path: '/audit', component: () => import('./pages/Audit.vue'), meta: { title: '审计日志', group: '数据查询' } },
  { path: '/feedback', component: () => import('./pages/Feedback.vue'), meta: { title: '用户反馈', group: '数据查询' } },
  { path: '/accounts', component: () => import('./pages/Accounts.vue'), meta: { title: '管理员账号', group: '系统' } },
  { path: '/site-config', component: () => import('./pages/SiteConfig.vue'), meta: { title: '站点设置', group: '系统' } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

router.beforeEach((to) => {
  if (to.meta.public) return true
  if (!token.get()) return { path: '/login', query: { next: to.fullPath } }
  return true
})
