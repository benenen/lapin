<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'

import type { PersistedWhiteboardData, WhiteboardData } from '../types'
import { mountExcalidraw, type ExcalidrawBridge } from '../excalidrawBridge'
import { chapterContentRevision, isCompatibleWhiteboard, isLegacyTldrawWhiteboard, viewportScale, WHITEBOARD_MIN_HEIGHT, WHITEBOARD_WIDTH } from '../whiteboard'
import RichTextContent from './RichTextContent.vue'

const props = defineProps<{
  chapterId: string
  content: string
  modelValue?: PersistedWhiteboardData | null
  saving?: boolean
}>()

const emit = defineEmits<{
  save: [data: WhiteboardData]
}>()

const viewport = ref<HTMLElement | null>(null)
const contentLayer = ref<HTMLElement | null>(null)
const editorHost = ref<HTMLElement | null>(null)
const ready = ref(false)
const error = ref('')
const revision = ref('')
const referenceWidth = ref(WHITEBOARD_WIDTH)
const referenceHeight = ref(WHITEBOARD_MIN_HEIGHT)
const scale = ref(1)
const legacyResetAllowed = ref(false)
let bridge: ExcalidrawBridge | null = null
let resizeObserver: ResizeObserver | null = null
let rebuildGeneration = 0

const compatibleData = computed(() => isCompatibleWhiteboard(props.modelValue, props.chapterId) ? props.modelValue : null)
const legacyBlocked = computed(() => isLegacyTldrawWhiteboard(props.modelValue) && !legacyResetAllowed.value)
const stageStyle = computed(() => ({
  width: `${referenceWidth.value * scale.value}px`,
  height: `${referenceHeight.value * scale.value}px`,
}))
const contentStyle = computed(() => ({
  width: `${referenceWidth.value}px`,
  minHeight: `${referenceHeight.value}px`,
  transform: `scale(${scale.value})`,
}))
const revisionChanged = computed(() => Boolean(compatibleData.value && revision.value && compatibleData.value.anchor.content_revision !== revision.value))

onMounted(() => {
  resizeObserver = new ResizeObserver(syncScale)
  if (viewport.value) resizeObserver.observe(viewport.value)
  void rebuild()
})

onBeforeUnmount(() => {
  rebuildGeneration++
  resizeObserver?.disconnect()
  bridge?.destroy()
})

watch(() => props.chapterId, () => { legacyResetAllowed.value = false })
watch(() => [props.chapterId, props.content, props.modelValue] as const, () => void rebuild(), { deep: true })
watch(() => props.saving, (saving) => bridge?.setSaving(Boolean(saving)))

function syncScale() {
  scale.value = viewportScale(viewport.value?.clientWidth ?? referenceWidth.value, referenceWidth.value)
  requestAnimationFrame(() => bridge?.resize())
}

async function rebuild() {
  const generation = ++rebuildGeneration
  const chapterId = props.chapterId
  const content = props.content
  const saved = isCompatibleWhiteboard(props.modelValue, chapterId) ? props.modelValue : null
  const legacy = isLegacyTldrawWhiteboard(props.modelValue) ? props.modelValue : null
  bridge?.destroy()
  bridge = null
  ready.value = false
  error.value = ''
  const nextRevision = await chapterContentRevision(content)
  if (generation !== rebuildGeneration) return
  revision.value = nextRevision
  referenceWidth.value = saved?.space.width ?? legacy?.space.width ?? WHITEBOARD_WIDTH
  referenceHeight.value = saved?.space.height ?? legacy?.space.height ?? WHITEBOARD_MIN_HEIGHT
  await nextTick()
  if (generation !== rebuildGeneration) return
  if (!saved && !legacy && contentLayer.value) {
    referenceHeight.value = Math.max(WHITEBOARD_MIN_HEIGHT, contentLayer.value.scrollHeight + 80)
    await nextTick()
    if (generation !== rebuildGeneration) return
  }
  syncScale()
  if (legacy && !legacyResetAllowed.value) return
  if (!editorHost.value) return
  bridge = mountExcalidraw(editorHost.value, {
    data: saved,
    width: referenceWidth.value,
    height: referenceHeight.value,
    topInset: 0,
    onSave: save,
    onReady: () => {
      if (generation !== rebuildGeneration) return
      ready.value = true
      bridge?.setSaving(Boolean(props.saving))
    },
    onError: (caught: Error) => { if (generation === rebuildGeneration) error.value = caught.message },
  })
}

function startExcalidraw() {
  legacyResetAllowed.value = true
  void rebuild()
}

async function save() {
  if (!bridge?.isReady()) return
  try {
    emit('save', {
      version: 3,
      anchor: { type: 'chapter', id: props.chapterId, content_revision: revision.value },
      space: { width: referenceWidth.value, height: referenceHeight.value, fit: 'contain' },
      document: bridge.getDocument(),
    })
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : '保存白板失败'
  }
}
</script>

<template>
  <section class="anchored-whiteboard" data-testid="anchored-whiteboard">
    <Message v-if="legacyBlocked" severity="warn" :closable="false">
      <span>检测到旧版 tldraw 白板。两种格式无法安全自动转换；旧数据会继续保留，只有新白板保存后才会替换。</span>
      <Button label="使用 Excalidraw 新建" size="small" severity="secondary" @click="startExcalidraw" />
    </Message>
    <Message v-if="revisionChanged" severity="warn" :closable="false">章节正文已经变化，原白板仍按保存时版式显示，请确认位置后再保存。</Message>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div ref="viewport" class="whiteboard-viewport">
      <div class="whiteboard-stage" :style="stageStyle">
        <div ref="contentLayer" class="whiteboard-content-layer" :style="contentStyle">
          <RichTextContent v-if="content" :content="content" />
          <p v-else class="chapter-content">本章暂无正文，可以直接在空白区域勾画。</p>
        </div>
        <div ref="editorHost" class="excalidraw-host" aria-label="章节透明白板" />
      </div>
    </div>
  </section>
</template>
