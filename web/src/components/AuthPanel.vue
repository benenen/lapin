<script setup lang="ts">
import { ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Password from 'primevue/password'

import { api } from '../api'
import lapinLogo from '../assets/lapin-logo.svg'
import type { User } from '../types'

const emit = defineEmits<{ authenticated: [user: User] }>()

const mode = ref<'login' | 'register'>('login')
const email = ref('')
const name = ref('')
const avatarURL = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  error.value = ''
  loading.value = true
  try {
    const result = mode.value === 'register'
      ? await api.register({ email: email.value, name: name.value, avatar_url: avatarURL.value, password: password.value })
      : await api.login({ email: email.value, password: password.value })
    emit('authenticated', result.user)
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '操作失败'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="auth-shell">
    <section class="brand-panel">
      <span class="eyebrow">LEARN LIKE A RABBIT</span>
      <h1>把知识变成<br /><em>可以探索的地图</em></h1>
      <p>导入科目，拆解章节，在文本与白板上留下思路，并和伙伴一起讨论。</p>
      <img class="brand-logo" :src="lapinLogo" alt="Lapin" />
    </section>

    <Card class="auth-card">
      <template #title>{{ mode === 'login' ? '欢迎回来' : '创建学习空间' }}</template>
      <template #subtitle>{{ mode === 'login' ? '继续你的学习旅程' : '一分钟即可开始' }}</template>
      <template #content>
        <form class="auth-form" @submit.prevent="submit">
          <label v-if="mode === 'register'">
            <span>昵称</span>
            <InputText v-model="name" autocomplete="name" required maxlength="80" fluid />
          </label>
          <label v-if="mode === 'register'">
            <span>头像地址（可选，仅 HTTPS）</span>
            <InputText v-model="avatarURL" type="url" autocomplete="photo" maxlength="2048" placeholder="https://…" fluid />
          </label>
          <label>
            <span>邮箱</span>
            <InputText v-model="email" type="email" autocomplete="email" required fluid />
          </label>
          <label>
            <span>密码</span>
            <Password
              v-model="password"
              :feedback="false"
              :autocomplete="mode === 'login' ? 'current-password' : 'new-password'"
              :minlength="8"
              toggle-mask
              fluid
              required
            />
          </label>
          <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
          <Button type="submit" :label="mode === 'login' ? '登录' : '注册并进入'" :loading="loading" fluid />
        </form>
      </template>
      <template #footer>
        <button class="text-button" type="button" @click="mode = mode === 'login' ? 'register' : 'login'; error = ''">
          {{ mode === 'login' ? '没有账号？现在注册' : '已经有账号？返回登录' }}
        </button>
      </template>
    </Card>
  </main>
</template>
