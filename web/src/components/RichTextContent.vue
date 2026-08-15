<script setup lang="ts">
import { watch } from 'vue'
import type { Editor } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/vue-3'

import { createEditorExtensions } from '../editor'

const props = defineProps<{ content: string }>()
const emit = defineEmits<{
  selection: [selection: { start_offset: number; end_offset: number; quote: string }]
}>()

const editor = useEditor({
  content: props.content,
  contentType: 'markdown',
  editable: false,
  extensions: createEditorExtensions(),
  onSelectionUpdate: ({ editor: current }) => emitSelection(current),
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

function emitSelection(current: Editor) {
  const { from, to } = current.state.selection
  if (from === to) return
  const leafText = (node: { type: { name: string }; attrs: Record<string, unknown> }) => {
    if (node.type.name === 'inlineMath' || node.type.name === 'blockMath') {
      return String(node.attrs.latex ?? '')
    }
    return ''
  }
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
  <EditorContent class="chapter-content" :editor="editor" />
</template>
