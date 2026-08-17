<script setup lang="ts">
import { watch } from 'vue'
import { EditorContent, useEditor } from '@tiptap/vue-3'
import Button from 'primevue/button'

import { createEditorExtensions } from '../editor'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const editor = useEditor({
  content: props.modelValue,
  contentType: 'markdown',
  extensions: createEditorExtensions({ editInlineMath, editBlockMath }),
  onUpdate: ({ editor: current }) => emit('update:modelValue', current.getMarkdown()),
})

watch(
  () => props.modelValue,
  (value) => {
    const current = editor.value
    if (!current) return
    if (current.getMarkdown() !== value) {
      current.commands.setContent(value, { contentType: 'markdown', emitUpdate: false })
    }
  },
)

function insertInlineMath() {
  const latex = window.prompt('输入行内 LaTeX 公式，例如 E = mc^2')?.trim()
  if (latex) editor.value?.chain().focus().insertInlineMath({ latex }).run()
}

function insertBlockMath() {
  const latex = window.prompt('输入块级 LaTeX 公式，例如 \\frac{a}{b}')?.trim()
  if (latex) editor.value?.chain().focus().insertBlockMath({ latex }).run()
}

function editInlineMath(latex: string, position: number) {
  const next = window.prompt('编辑行内 LaTeX 公式', latex)?.trim()
  if (next) editor.value?.chain().updateInlineMath({ latex: next, pos: position }).focus().run()
}

function editBlockMath(latex: string, position: number) {
  const next = window.prompt('编辑块级 LaTeX 公式', latex)?.trim()
  if (next) editor.value?.chain().updateBlockMath({ latex: next, pos: position }).focus().run()
}
</script>

<template>
  <div class="rich-text-editor">
    <div v-if="editor" class="rich-text-toolbar" aria-label="正文格式">
      <Button type="button" label="B" aria-label="粗体" text size="small" :class="['rich-toolbar-bold', { active: editor.isActive('bold') }]" @click="editor.chain().focus().toggleBold().run()" />
      <Button type="button" label="I" aria-label="斜体" text size="small" :class="['rich-toolbar-italic', { active: editor.isActive('italic') }]" @click="editor.chain().focus().toggleItalic().run()" />
      <Button type="button" label="H2" aria-label="二级标题" text size="small" :class="{ active: editor.isActive('heading', { level: 2 }) }" @click="editor.chain().focus().toggleHeading({ level: 2 }).run()" />
      <Button type="button" icon="pi pi-list" aria-label="无序列表" text size="small" :class="{ active: editor.isActive('bulletList') }" @click="editor.chain().focus().toggleBulletList().run()" />
      <Button type="button" icon="pi pi-sort-numeric-down" aria-label="有序列表" text size="small" :class="{ active: editor.isActive('orderedList') }" @click="editor.chain().focus().toggleOrderedList().run()" />
      <Button type="button" icon="pi pi-align-left" aria-label="引用" text size="small" :class="{ active: editor.isActive('blockquote') }" @click="editor.chain().focus().toggleBlockquote().run()" />
      <Button type="button" label="ƒx" aria-label="插入行内公式" text size="small" @click="insertInlineMath" />
      <Button type="button" label="∑" aria-label="插入块级公式" text size="small" @click="insertBlockMath" />
      <span class="toolbar-spacer" />
      <Button type="button" icon="pi pi-undo" aria-label="撤销" text size="small" :disabled="!editor.can().undo()" @click="editor.chain().focus().undo().run()" />
      <Button type="button" icon="pi pi-redo" aria-label="重做" text size="small" :disabled="!editor.can().redo()" @click="editor.chain().focus().redo().run()" />
    </div>
    <EditorContent :editor="editor" />
  </div>
</template>
