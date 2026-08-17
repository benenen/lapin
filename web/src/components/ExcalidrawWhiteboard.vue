<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'

import type { PersistedWhiteboardData, WhiteboardData } from '../types'
import { mountExcalidraw, type ExcalidrawBridge } from '../excalidrawBridge'
import { chapterContentRevision, isCompatibleWhiteboard, isLegacyTldrawWhiteboard, viewportScale, WHITEBOARD_MIN_HEIGHT, WHITEBOARD_WIDTH, whiteboardReferenceHeight } from '../whiteboard'
import RichTextContent from './RichTextContent.vue'

const props = defineProps<{
  chapterId: string
  content: string
  active: boolean
  modelValue?: PersistedWhiteboardData | null
  saving?: boolean
}>()

const emit = defineEmits<{
  save: [data: WhiteboardData]
  selection: [selection: { start_offset: number; end_offset: number; quote: string }]
}>()

const viewport = ref<HTMLElement | null>(null)
const contentLayer = ref<HTMLElement | null>(null)
const editorHost = ref<HTMLElement | null>(null)
const ready = ref(false)
const error = ref('')
const revision = ref('')
const referenceWidth = ref(WHITEBOARD_WIDTH)
const referenceHeight = ref(WHITEBOARD_MIN_HEIGHT)
const contentReferenceHeight = ref(0)
const scale = ref(1)
const interactionReady = ref(false)
const legacyResetAllowed = ref(false)
let bridge: ExcalidrawBridge | null = null
let resizeObserver: ResizeObserver | null = null
let rebuildGeneration = 0
let interactionGeneration = 0
let layoutChapterId = ''

const compatibleData = computed(() => isCompatibleWhiteboard(props.modelValue, props.chapterId) ? props.modelValue : null)
const legacyBlocked = computed(() => isLegacyTldrawWhiteboard(props.modelValue) && !legacyResetAllowed.value)
const stageStyle = computed(() => ({
  width: `${referenceWidth.value * scale.value}px`,
  height: `${(props.active ? referenceHeight.value : contentReferenceHeight.value) * scale.value}px`,
}))
const contentStyle = computed(() => ({
  width: `${referenceWidth.value}px`,
  minHeight: `${contentReferenceHeight.value}px`,
  transform: `scale(${scale.value})`,
}))
const revisionChanged = computed(() => Boolean(compatibleData.value && revision.value && compatibleData.value.anchor.content_revision !== revision.value))

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    syncScale()
    if (props.active) void prepareInteraction()
  })
  if (viewport.value) resizeObserver.observe(viewport.value)
  void rebuild()
})

onBeforeUnmount(() => {
  rebuildGeneration++
  interactionGeneration++
  resizeObserver?.disconnect()
  bridge?.destroy()
})

watch(() => props.chapterId, () => { legacyResetAllowed.value = false })
watch(() => [props.chapterId, props.content, props.modelValue] as const, () => void rebuild(), { deep: true })
watch(() => props.active, () => { void prepareInteraction() })
watch(() => props.saving, (saving) => bridge?.setSaving(Boolean(saving)))

function syncScale() {
  scale.value = viewportScale(viewport.value?.getBoundingClientRect().width ?? referenceWidth.value, referenceWidth.value)
  requestAnimationFrame(() => bridge?.resize())
}

function nextAnimationFrame(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()))
}

async function prepareInteraction() {
  const generation = ++interactionGeneration
  interactionReady.value = false
  if (!props.active) return
  await nextTick()
  await nextAnimationFrame()
  if (generation !== interactionGeneration || !props.active) return
  bridge?.resize()
  await nextAnimationFrame()
  if (generation !== interactionGeneration || !props.active || !bridge?.isReady()) return
  interactionReady.value = true
}

async function rebuild() {
  const generation = ++rebuildGeneration
  interactionGeneration++
  interactionReady.value = false
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
  if (saved || legacy) {
    referenceWidth.value = saved?.space.width ?? legacy?.space.width ?? WHITEBOARD_WIDTH
  } else if (layoutChapterId !== chapterId) {
    referenceWidth.value = Math.max(100, viewport.value?.getBoundingClientRect().width ?? WHITEBOARD_WIDTH)
  }
  layoutChapterId = chapterId
  referenceHeight.value = saved?.space.height ?? legacy?.space.height ?? WHITEBOARD_MIN_HEIGHT
  await nextTick()
  if (generation !== rebuildGeneration) return
  const renderedContent = contentLayer.value?.firstElementChild as HTMLElement | null
  contentReferenceHeight.value = renderedContent?.scrollHeight ?? 0
  referenceHeight.value = whiteboardReferenceHeight(contentReferenceHeight.value, referenceHeight.value)
  syncScale()
  if (generation !== rebuildGeneration) return
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
      void prepareInteraction()
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
    <Message v-if="props.active && legacyBlocked" severity="warn" :closable="false">
      <span>检测到旧版 tldraw 白板。两种格式无法安全自动转换；旧数据会继续保留，只有新白板保存后才会替换。</span>
      <Button label="使用 Excalidraw 新建" size="small" severity="secondary" @click="startExcalidraw" />
    </Message>
    <Message v-if="props.active && revisionChanged" severity="warn" :closable="false">章节正文已经变化，原白板仍按保存时版式显示，请确认位置后再保存。</Message>
    <Message v-if="props.active && error" severity="error" :closable="false">{{ error }}</Message>
    <div ref="viewport" class="whiteboard-viewport">
      <div class="whiteboard-stage" :class="{ 'is-active': props.active, 'is-interactive': interactionReady }" :style="stageStyle">
        <div ref="contentLayer" class="whiteboard-content-layer" :style="contentStyle">
          <RichTextContent v-if="content" :content="content" @selection="emit('selection', $event)" />
          <p v-else class="chapter-content">本章暂无正文，可以直接在空白区域勾画。</p>
        </div>
        <div ref="editorHost" class="excalidraw-host" aria-label="章节透明白板" :aria-hidden="!props.active || !interactionReady" />
      </div>
    </div>
  </section>
</template>
