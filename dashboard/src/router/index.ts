import { createRouter, createWebHistory } from 'vue-router'
import DashboardLayout from '../layouts/DashboardLayout.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: DashboardLayout,
      children: [
        { path: '', name: 'overview', component: () => import('../views/IndexView.vue') },
        { path: 'agents', name: 'agents', component: () => import('../views/AgentsView.vue') },
        { path: 'traces', name: 'traces', component: () => import('../views/TracesView.vue') },
        { path: 'policies', name: 'policies', component: () => import('../views/PoliciesView.vue') },
        { path: 'settings', name: 'settings', component: () => import('../views/SettingsView.vue') },
      ],
    },
  ],
})

export default router
