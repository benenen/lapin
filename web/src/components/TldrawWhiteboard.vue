<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'

import type { WhiteboardData } from '../types'
import { mountTldraw, type TldrawBridge, type WhiteboardTool } from '../tldrawBridge'
import { chapterContentRevision, isCompatibleWhiteboard, viewportScale, WHITEBOARD_MIN_HEIGHT, WHITEBOARD_WIDTH } from '../whiteboard'
import RichTextContent from './RichTextContent.vue'

const props = defineProps<{
  chapterId: string
  content: string
  modelValue?: WhiteboardData | null
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
const activeTool = ref<WhiteboardTool>('draw')
const referenceWidth = ref(WHITEBOARD_WIDTH)
const referenceHeight = ref(WHITEBOARD_MIN_HEIGHT)
const scale = ref(1)
let bridge: TldrawBridge | null = null
let resizeObserver: ResizeObserver | null = null
let rebuildGeneration = 0

const compatibleData = computed(() => isCompatibleWhiteboard(props.modelValue, props.chapterId) ? props.modelValue : null)
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

watch(() => [props.chapterId, props.content, props.modelValue] as const, () => void rebuild(), { deep: true })

function syncScale() {
  scale.value = viewportScale(viewport.value?.clientWidth ?? referenceWidth.value, referenceWidth.value)
  requestAnimationFrame(() => bridge?.resize())
}

async function rebuild() {
  const generation = ++rebuildGeneration
  const chapterId = props.chapterId
  const content = props.content
  const saved = isCompatibleWhiteboard(props.modelValue, chapterId) ? props.modelValue : null
  bridge?.destroy()
  bridge = null
  ready.value = false
  error.value = ''
  const nextRevision = await chapterContentRevision(content)
  if (generation !== rebuildGeneration) return
  revision.value = nextRevision
  referenceWidth.value = saved?.space.width ?? WHITEBOARD_WIDTH
  referenceHeight.value = saved?.space.height ?? WHITEBOARD_MIN_HEIGHT
  await nextTick()
  if (generation !== rebuildGeneration) return
  if (!saved && contentLayer.value) {
    referenceHeight.value = Math.max(WHITEBOARD_MIN_HEIGHT, contentLayer.value.scrollHeight + 80)
    await nextTick()
    if (generation !== rebuildGeneration) return
  }
  syncScale()
  if (!editorHost.value) return
  bridge = mountTldraw(editorHost.value, {
    data: saved,
    width: referenceWidth.value,
    height: referenceHeight.value,
    onReady: () => { if (generation === rebuildGeneration) ready.value = true },
    onError: (caught) => { if (generation === rebuildGeneration) error.value = caught.message },
  })
  activeTool.value = 'draw'
}

function chooseTool(tool: WhiteboardTool) {
  activeTool.value = tool
  bridge?.setTool(tool)
}

function undo() {
  bridge?.undo()
}

function redo() {
  bridge?.redo()
}

function clear() {
  bridge?.clear()
}

async function save() {
  if (!bridge?.isReady()) return
  try {
    emit('save', {
      version: 2,
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
  <section class="whiteboard-panel anchored-whiteboard" data-testid="anchored-whiteboard">
    <div class="whiteboard-toolbar">
      <div class="toolbar-actions" role="toolbar" aria-label="白板工具">
        <Button label="选择" icon="pi pi-arrows-alt" size="small" :outlined="activeTool !== 'select'" @click="chooseTool('select')" />
        <Button label="画笔" icon="pi pi-pencil" size="small" :outlined="activeTool !== 'draw'" @click="chooseTool('draw')" />
        <Button label="橡皮" icon="pi pi-eraser" size="small" :outlined="activeTool !== 'eraser'" @click="chooseTool('eraser')" />
      </div>
      <div class="toolbar-actions">
        <Button icon="pi pi-undo" aria-label="撤销" severity="secondary" text @click="undo" />
        <Button icon="pi pi-refresh" aria-label="重做" severity="secondary" text @click="redo" />
        <Button label="清空" icon="pi pi-trash" severity="secondary" text @click="clear" />
        <Button label="保存白板" icon="pi pi-save" :loading="saving" :disabled="!ready" @click="save" />
      </div>
    </div>
    <Message v-if="revisionChanged" severity="warn" :closable="false">章节正文已经变化，原白板仍按保存时版式显示，请确认位置后再保存。</Message>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div ref="viewport" class="whiteboard-viewport">
      <div class="whiteboard-stage" :style="stageStyle">
        <div ref="contentLayer" class="whiteboard-content-layer" :style="contentStyle">
          <RichTextContent v-if="content" :content="content" />
          <p v-else class="chapter-content">本章暂无正文，可以直接在空白区域勾画。</p>
        </div>
        <div ref="editorHost" class="tldraw-host" aria-label="章节透明白板" />
      </div>
    </div>
  </section>
</template>
