import { createRouter, createWebHashHistory } from 'vue-router'
import ChatView from './views/ChatView.vue'
import ModelsView from './views/ModelsView.vue'
import RulesView from './views/RulesView.vue'
import DashboardView from './views/DashboardView.vue'
import SettingsView from './views/SettingsView.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'chat', component: ChatView },
    { path: '/models', name: 'models', component: ModelsView },
    { path: '/rules', name: 'rules', component: RulesView },
    { path: '/dashboard', name: 'dashboard', component: DashboardView },
    { path: '/settings', name: 'settings', component: SettingsView },
  ],
})

export default router
