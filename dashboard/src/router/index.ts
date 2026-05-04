import { createRouter, createWebHistory } from 'vue-router'
import DashboardLayout from '../layouts/DashboardLayout.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: DashboardLayout,
      children: [
        { path: '',           name: 'overview',   component: () => import('../views/OverviewView.vue')   },
        { path: 'monitor',    name: 'monitor',    component: () => import('../views/MonitorView.vue')    },
        { path: 'runs',       name: 'runs',       component: () => import('../views/RunsView.vue')       },
        { path: 'traces',     name: 'traces',     component: () => import('../views/TracesView.vue')     },
        { path: 'audit',      name: 'audit',      component: () => import('../views/AuditView.vue')      },
        { path: 'agents',     name: 'agents',     component: () => import('../views/AgentsView.vue')     },
        { path: 'policies',   name: 'policies',   component: () => import('../views/PoliciesView.vue')   },
        { path: 'connectors', name: 'connectors', component: () => import('../views/ConnectorsView.vue') },
        { path: 'budgets',    name: 'budgets',    component: () => import('../views/BudgetsView.vue')    },
        { path: 'settings',   name: 'settings',   component: () => import('../views/SettingsView.vue')   },
      ],
    },
  ],
})

export default router
