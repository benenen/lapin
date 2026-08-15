<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'

import { api } from '../api'
import lapinLogo from '../assets/lapin-logo.svg'
import type { AccessToken, Annotation, Chapter, Comment, Subject, User, Whiteboard, WhiteboardData } from '../types'
import RichTextContent from './RichTextContent.vue'
import RichTextEditor from './RichTextEditor.vue'

const TldrawWhiteboard = defineAsyncComponent(() => import('./TldrawWhiteboard.vue'))

const props = defineProps<{ user: User }>()
const emit = defineEmits<{ logout: [] }>()

const subjects = ref<Subject[]>([])
const selectedSubject = ref<Subject | null>(null)
const activeChapterId = ref('')
const activeTab = ref<'notes' | 'whiteboard' | 'comments'>('notes')
const loading = ref(true)
const error = ref('')

const createDialog = ref(false)
const newSubject = ref({ title: '', description: '', tags: '', chapterTitle: '', chapterContent: '' })
const createLoading = ref(false)

const chapterDialog = ref(false)
const newChapter = ref<{ parent_id?: string; title: string; content: string }>({ title: '', content: '' })

const tokenDialog = ref(false)
const tokens = ref<AccessToken[]>([])
const tokenName = ref('OpenAPI')
const revealedToken = ref('')

const annotations = ref<Annotation[]>([])
const annotation = ref({ start_offset: 0, end_offset: 0, quote: '', note: '', color: 'yellow' })

const whiteboards = ref<Whiteboard[]>([])
const whiteboardSaving = ref(false)
const comments = ref<Comment[]>([])
const commentBody = ref('')

const activeChapter = computed(() => selectedSubject.value?.chapters?.find((chapter) => chapter.id === activeChapterId.value) ?? null)
const ownWhiteboard = computed<WhiteboardData | null>(() => whiteboards.value.find((board) => board.user_id === props.user.id)?.data ?? null)
const isOwner = computed(() => selectedSubject.value?.owner_id === props.user.id)
const chapterRows = computed<Array<Chapter & { depth: number }>>(() => {
  const chapters = selectedSubject.value?.chapters ?? []
  const knownIDs = new Set(chapters.map((chapter) => chapter.id))
  const children = new Map<string, Chapter[]>()
  for (const chapter of chapters) {
    const parent = chapter.parent_id && knownIDs.has(chapter.parent_id) ? chapter.parent_id : ''
    children.set(parent, [...(children.get(parent) ?? []), chapter])
  }
  const rows: Array<Chapter & { depth: number }> = []
  const stack = [...(children.get('') ?? [])].reverse().map((chapter) => ({ chapter, depth: 0 }))
  while (stack.length > 0) {
    const current = stack.pop()
    if (!current) break
    rows.push({ ...current.chapter, depth: current.depth })
    const nested = children.get(current.chapter.id) ?? []
    for (const child of [...nested].reverse()) {
      stack.push({ chapter: child, depth: current.depth + 1 })
    }
  }
  return rows
})

onMounted(loadSubjects)

watch(activeChapterId, async (id) => {
  annotations.value = []
  whiteboards.value = []
  comments.value = []
  if (!id) return
  try {
    const [nextAnnotations, nextWhiteboards, nextComments] = await Promise.all([
      api.listAnnotations(id),
      api.listWhiteboards(id),
      api.listComments(id),
    ])
    if (activeChapterId.value !== id) return
    annotations.value = nextAnnotations
    whiteboards.value = nextWhiteboards
    comments.value = nextComments
  } catch (caught) {
    showError(caught)
  }
})

async function loadSubjects() {
  loading.value = true
  try {
    subjects.value = await api.listSubjects()
    if (subjects.value.length > 0 && !selectedSubject.value) {
      await openSubject(subjects.value[0].id)
    }
  } catch (caught) {
    showError(caught)
  } finally {
    loading.value = false
  }
}

async function openSubject(id: string) {
  try {
    selectedSubject.value = await api.getSubject(id)
    activeChapterId.value = selectedSubject.value.chapters?.[0]?.id ?? ''
  } catch (caught) {
    showError(caught)
  }
}

async function createSubject() {
  createLoading.value = true
  try {
    const created = await api.createSubject({
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
    await openSubject(created.id)
  } catch (caught) {
    showError(caught)
  } finally {
    createLoading.value = false
  }
}

async function createChapter() {
  if (!selectedSubject.value) return
  try {
    await api.createChapter(selectedSubject.value.id, newChapter.value)
    chapterDialog.value = false
    newChapter.value = { title: '', content: '' }
    await openSubject(selectedSubject.value.id)
    await loadSubjects()
  } catch (caught) {
    showError(caught)
  }
}

function captureSelection(selection: { start_offset: number; end_offset: number; quote: string }) {
  annotation.value = {
    ...annotation.value,
    ...selection,
  }
}

async function saveAnnotation() {
  if (!activeChapter.value) return
  try {
    const created = await api.createAnnotation(activeChapter.value.id, annotation.value)
    annotations.value = [created, ...annotations.value]
    annotation.value = { start_offset: 0, end_offset: 0, quote: '', note: '', color: 'yellow' }
  } catch (caught) {
    showError(caught)
  }
}

async function saveWhiteboard(data: WhiteboardData) {
  if (!activeChapter.value) return
  whiteboardSaving.value = true
  try {
    const saved = await api.saveWhiteboard(activeChapter.value.id, data)
    whiteboards.value = [saved, ...whiteboards.value.filter((board) => board.user_id !== props.user.id)]
  } catch (caught) {
    showError(caught)
  } finally {
    whiteboardSaving.value = false
  }
}

async function postComment() {
  if (!activeChapter.value || !commentBody.value.trim()) return
  try {
    const created = await api.createComment(activeChapter.value.id, commentBody.value)
    comments.value = [...comments.value, created]
    commentBody.value = ''
  } catch (caught) {
    showError(caught)
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

function setActiveTab(tab: string) {
  if (tab === 'notes' || tab === 'whiteboard' || tab === 'comments') {
    activeTab.value = tab
  }
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

    <div class="workspace-grid">
      <aside class="subject-sidebar">
        <div class="sidebar-heading">
          <div><span class="eyebrow">LIBRARY</span><h2>我的科目</h2></div>
          <Button icon="pi pi-plus" aria-label="新建科目" rounded @click="createDialog = true" />
        </div>
        <div v-if="loading" class="muted">正在读取科目…</div>
        <button
          v-for="subject in subjects"
          :key="subject.id"
          type="button"
          class="subject-item"
          :class="{ active: selectedSubject?.id === subject.id }"
          @click="openSubject(subject.id)"
        >
          <span class="subject-icon">{{ subject.title.slice(0, 1) }}</span>
          <span><strong>{{ subject.title }}</strong><small>{{ subject.owner_name }} · {{ subject.tags.join(' / ') || '未分类' }}</small></span>
        </button>
        <div v-if="!loading && subjects.length === 0" class="empty-sidebar">
          <i class="pi pi-book" />
          <p>还没有科目</p>
          <Button label="创建第一个科目" size="small" @click="createDialog = true" />
        </div>
      </aside>

      <section v-if="selectedSubject" class="subject-main">
        <header class="subject-header">
          <div>
            <div class="tag-row"><Tag v-for="tag in selectedSubject.tags" :key="tag" :value="tag" severity="secondary" /></div>
            <h1>{{ selectedSubject.title }}</h1>
            <p>{{ selectedSubject.description || '这个科目还没有简介。' }}</p>
          </div>
          <Button v-if="isOwner" label="添加章节" icon="pi pi-plus" severity="secondary" outlined @click="chapterDialog = true" />
        </header>

        <div class="study-layout">
          <nav class="chapter-nav" aria-label="章节">
            <span class="eyebrow">CHAPTERS</span>
            <button
              v-for="chapter in chapterRows"
              :key="chapter.id"
              type="button"
              :class="{ active: chapter.id === activeChapterId }"
              :style="{ paddingLeft: `${0.65 + chapter.depth * 1.05}rem` }"
              @click="activeChapterId = chapter.id"
            >
              <span>{{ String(chapter.position + 1).padStart(2, '0') }}</span>{{ chapter.title }}
            </button>
          </nav>

          <article v-if="activeChapter" class="study-area">
            <div class="chapter-title"><span>第 {{ activeChapter.position + 1 }} 章</span><h2>{{ activeChapter.title }}</h2></div>
            <div class="study-tabs" role="tablist">
              <button v-for="tab in [
                { key: 'notes', label: '正文与标注', icon: 'pi-bookmark' },
                { key: 'whiteboard', label: '白板', icon: 'pi-pencil' },
                { key: 'comments', label: `讨论 ${comments.length}`, icon: 'pi-comments' },
              ]" :key="tab.key" type="button" :class="{ active: activeTab === tab.key }" @click="setActiveTab(tab.key)">
                <i class="pi" :class="tab.icon" /> {{ tab.label }}
              </button>
            </div>

            <section v-if="activeTab === 'notes'" class="notes-grid">
              <div>
                <p v-if="!activeChapter.content" class="chapter-content">本章暂无正文。</p>
                <RichTextContent v-else :content="activeChapter.content" @selection="captureSelection" />
                <p class="selection-tip"><i class="pi pi-info-circle" /> 选中正文即可定位标注位置</p>
              </div>
              <aside class="annotation-panel">
                <h3>新建标注</h3>
                <blockquote v-if="annotation.quote">“{{ annotation.quote }}”</blockquote>
                <span v-else class="muted">先在左侧正文中选中一段文字，也可以直接写章节笔记。</span>
                <Textarea v-model="annotation.note" rows="4" maxlength="2000" placeholder="写下你的理解…" fluid />
                <div class="annotation-actions">
                  <div class="annotation-colors">
                    <button v-for="color in ['yellow', 'green', 'blue', 'pink']" :key="color" type="button" :class="[color, { active: annotation.color === color }]" @click="annotation.color = color" />
                  </div>
                  <Button label="保存标注" size="small" :disabled="!annotation.note.trim()" @click="saveAnnotation" />
                </div>
                <div class="annotation-list">
                  <div v-for="item in annotations" :key="item.id" class="annotation-card" :class="item.color">
                    <small>{{ item.author_name }} · {{ new Date(item.created_at).toLocaleString() }}</small>
                    <q v-if="item.quote">{{ item.quote }}</q>
                    <p>{{ item.note }}</p>
                  </div>
                </div>
              </aside>
            </section>

            <section v-else-if="activeTab === 'whiteboard'">
              <div class="collaborator-line">
                <span>我的白板</span>
                <small>白板内容仅你自己可见</small>
              </div>
              <TldrawWhiteboard
                :chapter-id="activeChapter.id"
                :content="activeChapter.content"
                :model-value="ownWhiteboard"
                :saving="whiteboardSaving"
                @save="saveWhiteboard"
              />
            </section>

            <section v-else class="comments-panel">
              <div class="comment-compose">
                <Avatar :label="user.name.slice(0, 1)" shape="circle" />
                <Textarea v-model="commentBody" rows="3" maxlength="2000" placeholder="分享问题或想法…" fluid />
                <Button label="发送" icon="pi pi-send" :disabled="!commentBody.trim()" @click="postComment" />
              </div>
              <div v-if="comments.length === 0" class="empty-comments">还没有讨论，来提出第一个问题吧。</div>
              <div v-for="comment in comments" :key="comment.id" class="comment-item">
                <Avatar :label="comment.author_name.slice(0, 1)" shape="circle" />
                <div><strong>{{ comment.author_name }}</strong><small>{{ new Date(comment.created_at).toLocaleString() }}</small><p>{{ comment.body }}</p></div>
              </div>
            </section>
          </article>
          <div v-else class="empty-study"><i class="pi pi-file-edit" /><h3>还没有章节</h3><p>科目所有者可以添加第一章。</p></div>
        </div>
      </section>

      <section v-else class="welcome-empty">
        <div><span class="eyebrow">READY WHEN YOU ARE</span><h1>从一门科目开始</h1><p>创建科目，或者生成 Access Token 后通过 OpenAPI 导入。</p></div>
      </section>
    </div>

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

    <Dialog v-model:visible="chapterDialog" modal header="添加章节" :style="{ width: 'min(40rem, 94vw)' }">
      <form class="dialog-form" @submit.prevent="createChapter">
        <label>
          <span>父章节（可选）</span>
          <Select
            v-model="newChapter.parent_id"
            :options="chapterRows"
            option-label="title"
            option-value="id"
            placeholder="作为顶层章节"
            show-clear
            fluid
          />
        </label>
        <label><span>章节标题</span><InputText v-model="newChapter.title" required maxlength="200" fluid /></label>
        <label><span>正文（Markdown 存储）</span><RichTextEditor v-model="newChapter.content" /></label>
        <Button type="submit" label="保存章节" />
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
