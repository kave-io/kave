import { createRouter, createWebHistory } from 'vue-router'
import ConsoleLayout from '@/layouts/ConsoleLayout.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: ConsoleLayout,
      children: [
        { path: '', name: 'overview', component: () => import('@/views/OverviewView.vue') },
        {
          path: 'analytics',
          name: 'analytics',
          component: () => import('@/views/AnalyticsView.vue'),
        },
        { path: 'tenants', name: 'tenants', component: () => import('@/views/TenantsView.vue') },
        {
          path: 'namespace',
          name: 'namespace',
          component: () => import('@/views/NamespaceView.vue'),
        },
        { path: 'audit', name: 'audit', component: () => import('@/views/AuditView.vue') },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.afterEach((route) => {
  const label = typeof route.name === 'string' ? route.name : 'overview'
  document.title = `${label.charAt(0).toUpperCase()}${label.slice(1)} · Kave`
})

export default router
