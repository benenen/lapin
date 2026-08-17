import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'subjects', component: () => import('./components/SubjectListView.vue') },
    {
      path: '/subjects/:subjectId',
      name: 'subject-detail',
      component: () => import('./components/DashboardView.vue'),
      props: (route) => ({ subjectId: String(route.params.subjectId) }),
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})
