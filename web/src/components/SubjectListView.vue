<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'

import { api } from '../api'
import lapinLogo from '../assets/lapin-logo.svg'
import type { AccessToken, Subject, User } from '../types'
import RichTextEditor from './RichTextEditor.vue'

defineProps<{ user: User }>()
const emit = defineEmits<{ logout: [] }>()

const subjects = ref<Subject[]>([])
const loading = ref(true)
const error = ref('')
const createDialog = ref(false)
const newSubject = ref({ title: '', description: '', tags: '', chapterTitle: '', chapterContent: '' })
const createLoading = ref(false)
const tokenDialog = ref(false)
const tokens = ref<AccessToken[]>([])
const tokenName = ref('OpenAPI')
const revealedToken = ref('')

onMounted(loadSubjects)

async function loadSubjects() {
  loading.value = true
  try {
    subjects.value = await api.listSubjects()
  } catch (caught) {
    showError(caught)
  } finally {
    loading.value = false
  }
}

async function createSubject() {
  createLoading.value = true
  try {
    await api.createSubject({
      title: newSubject.value.title,
      description: newSubject.value.description,
      tags: newSubject.value.tags.split(',').map((tag) => tag.trim()).filter(Boolean),
      chapters: newSubject.value.chapterTitle.trim()
        ? [{ title: newSubject.value.chapterTitle, content: newSubject.value.chapterContent }]
        : [],
    })
    createDialog.value = false
    newSubject.value = { title: '', description: '', tags: '', chapterTitle: '', chapterContent: '' }
    await loadSubjects()
  } catch (caught) {
    showError(caught)
  } finally {
    createLoading.value = false
  }
}

async function openTokens() {
  tokenDialog.value = true
  revealedToken.value = ''
  try {
    tokens.value = await api.listTokens()
  } catch (caught) {
    showError(caught)
  }
}

async function createToken() {
  try {
    const result = await api.createToken(tokenName.value)
    revealedToken.value = result.access_token
    tokens.value = [result.token, ...tokens.value]
  } catch (caught) {
    showError(caught)
  }
}

async function revokeToken(id: string) {
  try {
    await api.revokeToken(id)
    tokens.value = tokens.value.filter((token) => token.id !== id)
  } catch (caught) {
    showError(caught)
  }
}

async function copyToken() {
  await navigator.clipboard.writeText(revealedToken.value)
}

function showError(caught: unknown) {
  error.value = caught instanceof Error ? caught.message : '操作失败'
  window.setTimeout(() => { error.value = '' }, 5000)
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <div class="wordmark"><img :src="lapinLogo" alt="Lapin" /></div>
      <div class="topbar-actions">
        <Button label="Access Token" icon="pi pi-key" severity="secondary" outlined @click="openTokens" />
        <div class="user-chip">
          <Avatar :image="user.avatar_url || undefined" :label="user.avatar_url ? undefined : user.name.slice(0, 1).toUpperCase()" shape="circle" />
          <div><strong>{{ user.name }}</strong><small>{{ user.email }}</small></div>
        </div>
        <Button icon="pi pi-sign-out" aria-label="退出登录" severity="secondary" text @click="emit('logout')" />
      </div>
    </header>

    <Message v-if="error" class="global-message" severity="error" :closable="false">{{ error }}</Message>

    <main class="home-dashboard">
      <aside class="home-course-sidebar">
        <header class="home-course-heading">
          <div><span class="eyebrow">COURSES</span><h1>课程</h1></div>
          <Button label="新建" icon="pi pi-plus" size="small" @click="createDialog = true" />
        </header>

        <div v-if="loading" class="home-course-loading muted">正在读取课程…</div>
        <nav v-else-if="subjects.length > 0" class="home-course-list" aria-label="课程列表">
          <a
            v-for="subject in subjects"
            :key="subject.id"
            class="home-course-item"
            :href="`/subjects/${encodeURIComponent(subject.id)}`"
            target="_blank"
            rel="noopener noreferrer"
          >
            <span class="subject-icon">{{ subject.title.slice(0, 1) }}</span>
            <span class="home-course-copy"><strong>{{ subject.title }}</strong><small>{{ subject.owner_name }}</small></span>
            <i class="pi pi-external-link" aria-hidden="true" />
          </a>
        </nav>
        <div v-else class="home-course-empty">
          <strong>还没有课程</strong>
          <small>新建课程，或通过 OpenAPI 导入。</small>
          <Button label="新建课程" size="small" @click="createDialog = true" />
        </div>
      </aside>
      <section class="home-dashboard-main" aria-label="首页内容" />
    </main>

    <Dialog v-model:visible="createDialog" modal header="新建科目" :style="{ width: 'min(36rem, 94vw)' }">
      <form class="dialog-form" @submit.prevent="createSubject">
        <label><span>科目名称</span><InputText v-model="newSubject.title" required maxlength="200" fluid /></label>
        <label><span>简介</span><Textarea v-model="newSubject.description" rows="3" maxlength="4000" fluid /></label>
        <label><span>标签（英文逗号分隔）</span><InputText v-model="newSubject.tags" placeholder="Go, 数据库, 后端" fluid /></label>
        <div class="dialog-section">第一章（可选）</div>
        <label><span>章节标题</span><InputText v-model="newSubject.chapterTitle" maxlength="200" fluid /></label>
        <label><span>章节正文（Markdown 存储）</span><RichTextEditor v-model="newSubject.chapterContent" /></label>
        <Button type="submit" label="创建科目" :loading="createLoading" />
      </form>
    </Dialog>

    <Dialog v-model:visible="tokenDialog" modal header="OpenAPI Access Token" :style="{ width: 'min(42rem, 94vw)' }">
      <div class="token-create">
        <InputText v-model="tokenName" maxlength="80" placeholder="Token 名称" />
        <Button label="生成 Token" icon="pi pi-plus" @click="createToken" />
      </div>
      <Message v-if="revealedToken" severity="warn" :closable="false">
        <strong>请立即复制，关闭后无法再次查看：</strong>
        <code>{{ revealedToken }}</code>
        <Button label="复制" icon="pi pi-copy" size="small" @click="copyToken" />
      </Message>
      <div class="token-list">
        <div v-for="token in tokens" :key="token.id">
          <span><strong>{{ token.name }}</strong><small>{{ token.prefix }}… · 有效至 {{ new Date(token.expires_at).toLocaleDateString() }}</small></span>
          <Button icon="pi pi-trash" severity="danger" text aria-label="撤销 Token" @click="revokeToken(token.id)" />
        </div>
        <p v-if="tokens.length === 0" class="muted">还没有 Access Token。</p>
      </div>
    </Dialog>
  </div>
</template>
