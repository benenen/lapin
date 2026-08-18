<script setup lang="ts">
import { onMounted, watch } from 'vue'
import type { Editor } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/vue-3'

import { createEditorExtensions } from '../editor'
import { annotationDecorationPlugin, annotationDecorationPluginKey, leafText, type AnnotationOffsets } from '../annotationMarks'

const props = withDefaults(defineProps<{ content: string; annotations?: AnnotationOffsets[] }>(), {
  annotations: () => [],
})
const emit = defineEmits<{
  selection: [selection: { start_offset: number; end_offset: number; quote: string }]
  'annotation-click': [id: string]
}>()

const editor = useEditor({
  content: props.content,
  contentType: 'markdown',
  editable: false,
  extensions: createEditorExtensions(),
  onSelectionUpdate: ({ editor: current }) => emitSelection(current),
})

onMounted(() => {
  editor.value?.registerPlugin(annotationDecorationPlugin(() => props.annotations))
})

watch(
  () => props.content,
  (value) => {
    const current = editor.value
    if (!current) return
    if (current.getMarkdown() !== value) {
      current.commands.setContent(value, { contentType: 'markdown', emitUpdate: false })
    }
  },
)

watch(
  () => props.annotations,
  () => {
    const current = editor.value
    if (!current) return
    current.view.dispatch(current.state.tr.setMeta(annotationDecorationPluginKey, true))
  },
  { deep: true },
)

function handleClick(event: MouseEvent) {
  const target = (event.target as HTMLElement | null)?.closest('[data-annotation-id]')
  const id = target?.getAttribute('data-annotation-id')
  if (id) emit('annotation-click', id)
}

function emitSelection(current: Editor) {
  const { from, to } = current.state.selection
  if (from === to) return
  const quote = current.state.doc.textBetween(from, to, '', leafText)
  if (!quote) return
  const prefix = current.state.doc.textBetween(0, from, '', leafText)
  emit('selection', {
    start_offset: prefix.length,
    end_offset: prefix.length + quote.length,
    quote,
  })
}
</script>

<template>
  <EditorContent class="chapter-content autosize-rich-text" :editor="editor" @click="handleClick" />
</template>
