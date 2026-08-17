<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, ref, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'

import { api } from '../api'
import lapinLogo from '../assets/lapin-logo.svg'
import { buildChapterTree } from '../chapterTree'
import type { AccessToken, Annotation, Comment, PersistedWhiteboardData, Subject, User, Whiteboard, WhiteboardData } from '../types'
import ChapterTree from './ChapterTree.vue'
import RichTextContent from './RichTextContent.vue'
import RichTextEditor from './RichTextEditor.vue'

const ExcalidrawWhiteboard = defineAsyncComponent(() => import('./ExcalidrawWhiteboard.vue'))

const props = defineProps<{ user: User; subjectId: string }>()
const emit = defineEmits<{ logout: [] }>()

const selectedSubject = ref<Subject | null>(null)
const activeChapterId = ref('')
const activeTab = ref<'notes' | 'comments'>('notes')
const whiteboardVisible = ref(false)
const loading = ref(true)
const error = ref('')

const editSubjectDialog = ref(false)
const editSubjectDraft = ref({ title: '', description: '' })
const editSubjectLoading = ref(false)

const chapterDialog = ref(false)
const newChapter = ref<{ parent_id?: string; title: string; content: string }>({ title: '', content: '' })
const editChapterDialog = ref(false)
const editChapterDraft = ref({ title: '', content: '' })
const editChapterLoading = ref(false)

const tokenDialog = ref(false)
const tokens = ref<AccessToken[]>([])
const tokenName = ref('OpenAPI')
const revealedToken = ref('')

const annotations = ref<Annotation[]>([])
const annotation = ref({ start_offset: 0, end_offset: 0, quote: '', note: '', color: 'yellow' })

const whiteboards = ref<Whiteboard[]>([])
const whiteboardsLoaded = ref(false)
const whiteboardLoading = ref(false)
const whiteboardLoadError = ref('')
const whiteboardSaving = ref(false)
const comments = ref<Comment[]>([])
const commentBody = ref('')
let interactionLoadGeneration = 0
let whiteboardLoadGeneration = 0

const activeChapter = computed(() => selectedSubject.value?.chapters?.find((chapter) => chapter.id === activeChapterId.value) ?? null)
const ownWhiteboard = computed<PersistedWhiteboardData | null>(() => whiteboards.value.find((board) => board.user_id === props.user.id)?.data ?? null)
const isOwner = computed(() => selectedSubject.value?.owner_id === props.user.id)
const chapterTreeNodes = computed(() => buildChapterTree(selectedSubject.value?.chapters ?? []))

onMounted(() => void openSubject(props.subjectId))

watch(() => props.subjectId, (id) => void openSubject(id))

watch(activeChapterId, (id) => {
  whiteboardVisible.value = false
  whiteboardsLoaded.value = false
  whiteboardLoading.value = false
  whiteboardLoadError.value = ''
  annotations.value = []
  whiteboards.value = []
  comments.value = []
  if (!id) return
  void loadChapterInteractions(id)
  void loadWhiteboards(id)
})

async function loadChapterInteractions(id: string) {
  const generation = ++interactionLoadGeneration
  const [annotationsResult, commentsResult] = await Promise.allSettled([
    api.listAnnotations(id),
    api.listComments(id),
  ])
  if (generation !== interactionLoadGeneration || activeChapterId.value !== id) return
  if (annotationsResult.status === 'fulfilled') {
    annotations.value = annotationsResult.value
  } else {
    showError(annotationsResult.reason)
  }
  if (commentsResult.status === 'fulfilled') {
    comments.value = commentsResult.value
  } else {
    showError(commentsResult.reason)
  }
}

async function loadWhiteboards(id: string) {
  const generation = ++whiteboardLoadGeneration
  whiteboardVisible.value = false
  whiteboardsLoaded.value = false
  whiteboardLoading.value = true
  whiteboardLoadError.value = ''
  try {
    const nextWhiteboards = await api.listWhiteboards(id)
    if (generation !== whiteboardLoadGeneration || activeChapterId.value !== id) return
    whiteboards.value = nextWhiteboards
    whiteboardsLoaded.value = true
  } catch (caught) {
    if (generation === whiteboardLoadGeneration && activeChapterId.value === id) {
      whiteboardLoadError.value = '白板加载失败，请重试'
      showError(caught)
    }
  } finally {
    if (generation === whiteboardLoadGeneration && activeChapterId.value === id) whiteboardLoading.value = false
  }
}

async function openSubject(id: string) {
  loading.value = true
  try {
    const subject = await api.getSubject(id)
    if (props.subjectId !== id) return
    selectedSubject.value = subject
    activeChapterId.value = buildChapterTree(subject.chapters ?? [])[0]?.chapter.id ?? ''
  } catch (caught) {
    selectedSubject.value = null
    activeChapterId.value = ''
    showError(caught)
  } finally {
    if (props.subjectId === id) loading.value = false
  }
}

function openEditSubject() {
  if (!selectedSubject.value || !isOwner.value) return
  editSubjectDraft.value = {
    title: selectedSubject.value.title,
    description: selectedSubject.value.description,
  }
  editSubjectDialog.value = true
}

async function updateSubject() {
  if (!selectedSubject.value || !isOwner.value) return
  editSubjectLoading.value = true
  try {
    const updated = await api.updateSubject(selectedSubject.value.id, { ...editSubjectDraft.value })
    selectedSubject.value = updated
    editSubjectDialog.value = false
  } catch (caught) {
    showError(caught)
  } finally {
    editSubjectLoading.value = false
  }
}

async function createChapter() {
  if (!selectedSubject.value) return
  try {
    await api.createChapter(selectedSubject.value.id, newChapter.value)
    chapterDialog.value = false
    newChapter.value = { title: '', content: '' }
    await openSubject(selectedSubject.value.id)
  } catch (caught) {
    showError(caught)
  }
}

function openEditChapter() {
  if (!activeChapter.value || !isOwner.value) return
  editChapterDraft.value = {
    title: activeChapter.value.title,
    content: activeChapter.value.content,
  }
  editChapterDialog.value = true
}

async function updateChapter() {
  if (!activeChapter.value || !selectedSubject.value || !isOwner.value) return
  editChapterLoading.value = true
  try {
    const updated = await api.updateChapter(activeChapter.value.id, { ...editChapterDraft.value })
    selectedSubject.value = {
      ...selectedSubject.value,
      chapters: (selectedSubject.value.chapters ?? []).map((chapter) => (
        chapter.id === updated.id ? updated : chapter
      )),
    }
    editChapterDialog.value = false
  } catch (caught) {
    showError(caught)
  } finally {
    editChapterLoading.value = false
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
  if (tab === 'notes' || tab === 'comments') {
    activeTab.value = tab
    if (tab === 'comments') whiteboardVisible.value = false
  }
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <a class="wordmark" href="/" aria-label="返回科目首页"><img :src="lapinLogo" alt="Lapin" /></a>
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

    <div class="workspace-grid detail-only">
      <section v-if="selectedSubject" class="subject-main">
        <header class="subject-header">
          <div>
            <div class="tag-row"><Tag v-for="tag in selectedSubject.tags" :key="tag" :value="tag" severity="secondary" /></div>
            <h1>{{ selectedSubject.title }}</h1>
            <p>{{ selectedSubject.description || '这个科目还没有简介。' }}</p>
          </div>
          <div v-if="isOwner" class="owner-actions">
            <Button label="编辑科目" icon="pi pi-pencil" severity="secondary" outlined @click="openEditSubject" />
            <Button label="添加章节" icon="pi pi-plus" severity="secondary" outlined @click="chapterDialog = true" />
          </div>
        </header>

        <div class="study-layout">
          <nav class="chapter-nav" aria-label="章节">
            <span class="eyebrow">CHAPTERS</span>
            <ChapterTree :nodes="chapterTreeNodes" :active-chapter-id="activeChapterId" @select="activeChapterId = $event" />
          </nav>

          <article v-if="activeChapter" class="study-area">
            <div class="chapter-heading">
              <div class="chapter-title"><span>第 {{ activeChapter.position + 1 }} 章</span><h2>{{ activeChapter.title }}</h2></div>
              <Button v-if="isOwner" label="编辑章节" icon="pi pi-pencil" severity="secondary" text @click="openEditChapter" />
            </div>
            <div class="study-tabs" role="tablist">
              <button v-for="tab in [
                { key: 'notes', label: '正文与标注', icon: 'pi-bookmark' },
                { key: 'comments', label: `讨论 ${comments.length}`, icon: 'pi-comments' },
              ]" :key="tab.key" type="button" :class="{ active: activeTab === tab.key }" @click="setActiveTab(tab.key)">
                <i class="pi" :class="tab.icon" /> {{ tab.label }}
              </button>
            </div>

            <section v-show="activeTab === 'notes'" class="notes-grid">
              <div class="chapter-document">
                <div class="chapter-document-actions">
                  <Button
                    v-if="whiteboardLoadError"
                    label="重试加载白板"
                    icon="pi pi-refresh"
                    severity="secondary"
                    outlined
                    size="small"
                    :loading="whiteboardLoading"
                    @click="loadWhiteboards(activeChapter.id)"
                  />
                  <Button
                    v-else
                    :label="whiteboardVisible ? '隐藏白板' : '显示白板'"
                    :icon="whiteboardVisible ? 'pi pi-eye-slash' : 'pi pi-eye'"
                    severity="secondary"
                    outlined
                    size="small"
                    :loading="whiteboardLoading"
                    :disabled="!whiteboardsLoaded"
                    @click="whiteboardVisible = !whiteboardVisible"
                  />
                </div>
                <ExcalidrawWhiteboard
                  :chapter-id="activeChapter.id"
                  :content="activeChapter.content"
                  :active="whiteboardVisible && whiteboardsLoaded"
                  :model-value="ownWhiteboard"
                  :saving="whiteboardSaving"
                  @selection="captureSelection"
                  @save="saveWhiteboard"
                />
              </div>
              <aside class="annotation-panel">
                <h3>新建标注</h3>
                <blockquote v-if="annotation.quote">“{{ annotation.quote }}”</blockquote>
                <RichTextEditor v-model="annotation.note" class="compact-rich-text-editor" />
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
                    <RichTextContent :content="item.note" />
                  </div>
                </div>
              </aside>
            </section>

            <section v-show="activeTab === 'comments'" class="comments-panel">
              <div class="comment-compose">
                <Avatar :label="user.name.slice(0, 1)" shape="circle" />
                <RichTextEditor v-model="commentBody" class="compact-rich-text-editor" />
                <Button label="发送" icon="pi pi-send" :disabled="!commentBody.trim()" @click="postComment" />
              </div>
              <div v-if="comments.length === 0" class="empty-comments">还没有讨论，来提出第一个问题吧。</div>
              <div v-for="comment in comments" :key="comment.id" class="comment-item">
                <Avatar :label="comment.author_name.slice(0, 1)" shape="circle" />
                <div><strong>{{ comment.author_name }}</strong><small>{{ new Date(comment.created_at).toLocaleString() }}</small><RichTextContent :content="comment.body" /></div>
              </div>
            </section>
          </article>
          <div v-else class="empty-study"><i class="pi pi-file-edit" /><h3>还没有章节</h3><p>科目所有者可以添加第一章。</p></div>
        </div>
      </section>

      <section v-else class="welcome-empty detail-empty">
        <div v-if="loading"><span class="eyebrow">LOADING</span><h1>正在读取科目</h1><p>请稍候，学习内容马上就好。</p></div>
        <div v-else><span class="eyebrow">NOT FOUND</span><h1>无法打开这个科目</h1><p>科目可能不存在，或者你没有查看权限。</p><a class="text-link" href="/">返回科目首页</a></div>
      </section>
    </div>

    <Dialog v-model:visible="editSubjectDialog" modal header="编辑科目" :style="{ width: 'min(36rem, 94vw)' }">
      <form class="dialog-form" @submit.prevent="updateSubject">
        <Message v-if="selectedSubject?.external_id" severity="warn" :closable="false">
          这个科目来自 OpenAPI，再次通过 OpenAPI 导入时可能被覆盖。
        </Message>
        <label><span>科目名称</span><InputText v-model="editSubjectDraft.title" required maxlength="200" fluid /></label>
        <label><span>简介</span><Textarea v-model="editSubjectDraft.description" rows="3" maxlength="4000" fluid /></label>
        <Button type="submit" label="保存修改" :loading="editSubjectLoading" />
      </form>
    </Dialog>

    <Dialog v-model:visible="chapterDialog" modal header="添加章节" :style="{ width: 'min(40rem, 94vw)' }">
      <form class="dialog-form" @submit.prevent="createChapter">
        <label>
          <span>父章节（可选）</span>
          <Select
            v-model="newChapter.parent_id"
            :options="selectedSubject?.chapters ?? []"
            option-label="title"
            option-value="id"
            placeholder="作为顶层章节"
            show-clear
            fluid
          />
        </label>
        <label><span>章节标题</span><InputText v-model="newChapter.title" required maxlength="200" fluid /></label>
        <label><span>正文（Markdown 存储）</span><RichTextEditor v-model="newChapter.content" allow-images /></label>
        <Button type="submit" label="保存章节" />
      </form>
    </Dialog>

    <Dialog v-model:visible="editChapterDialog" modal header="编辑章节" :style="{ width: 'min(44rem, 94vw)' }">
      <form class="dialog-form" @submit.prevent="updateChapter">
        <Message severity="warn" :closable="false">
          正文位置变化可能使既有标注和白板提示需要重新校对；章节 ID 和现有数据会保留。
          <span v-if="activeChapter?.external_id">再次通过 OpenAPI 导入时，本章修改也可能被覆盖。</span>
        </Message>
        <label><span>章节标题</span><InputText v-model="editChapterDraft.title" required maxlength="200" fluid /></label>
        <label><span>正文（Markdown 存储）</span><RichTextEditor v-model="editChapterDraft.content" allow-images /></label>
        <Button type="submit" label="保存修改" :loading="editChapterLoading" />
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
