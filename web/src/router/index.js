import { createRouter, createWebHistory } from 'vue-router'
import { useConfigStore } from '@/stores/config'
import HomeView from '../views/HomeView.vue'
import ConfigView from "@/views/ConfigView.vue";
import WorkerStatusView from "@/views/admin/WorkerStatusView.vue";
import MessageListView from "@/views/admin/MessageListView.vue";
import MessageDetailView from "@/views/admin/MessageDetailView.vue";
import SendMessageView from "@/views/public/SendMessageView.vue";
import TemplateListView from "@/views/admin/TemplateListView.vue";
import StopListView from "@/views/admin/StopListView.vue";

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/admin/subscriptions',
      alias: '/admin/stop-list',
      name: 'AdminSubscriptions',
      component: StopListView,
    },
    {
      path: '/',
      name: 'Home',
      component: HomeView,
    },
    {
      path: '/config',
      name: 'Config',
      component: ConfigView,
    },
    {
      path: '/send',
      name: 'Send',
      component: SendMessageView,
    },
    {
      path: '/admin/messages',
      name: 'AdminMessageList',
      component: MessageListView,
    },
    {
      path: '/admin/message/:id',
      name: 'AdminMessageDetail',
      component: MessageDetailView
    },
    {
      path: '/admin/templates',
      name: 'AdminTemplates',
      component: TemplateListView,
    },
    {
      path: '/admin/worker/statuses',
      name: 'WorkerStatuses',
      component: WorkerStatusView,
    },
  ],
})

router.beforeEach((to) => {
  const configStore = useConfigStore()
  const hasConfig = configStore.host && configStore.apiKey && configStore.apiHeader

  if (!hasConfig && to.name !== 'Config') {
    return { name: 'Config' }
  }

  return true
})


export default router
