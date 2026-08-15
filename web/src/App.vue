<script setup lang="ts">
import { onMounted, ref } from 'vue'
import ProgressSpinner from 'primevue/progressspinner'

import { api } from './api'
import AuthPanel from './components/AuthPanel.vue'
import DashboardView from './components/DashboardView.vue'
import type { User } from './types'

const user = ref<User | null>(null)
const loading = ref(true)

onMounted(async () => {
  try {
    user.value = await api.me()
  } catch {
    user.value = null
  } finally {
    loading.value = false
  }
})

async function logout() {
  try {
    await api.logout()
  } finally {
    user.value = null
  }
}
</script>

<template>
  <main v-if="loading" class="loading-screen" aria-label="正在加载">
    <ProgressSpinner />
  </main>
  <AuthPanel v-else-if="!user" @authenticated="user = $event" />
  <DashboardView v-else :user="user" @logout="logout" />
</template>
