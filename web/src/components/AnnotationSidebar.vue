<script setup lang="ts">
import { computed, nextTick, watch } from 'vue'
import Button from 'primevue/button'

import type { Annotation } from '../types'
import RichTextContent from './RichTextContent.vue'
import RichTextEditor from './RichTextEditor.vue'

interface AnnotationDraft {
  quote: string
  note: string
  color: string
}

const props = defineProps<{
  open: boolean
  annotations: Annotation[]
  draft: AnnotationDraft
  activeAnnotationId: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'update:draft': [draft: AnnotationDraft]
  'save-annotation': []
}>()

const colors = ['yellow', 'green', 'blue', 'pink']
const canSave = computed(() => props.draft.note.trim().length > 0)

// HashIDs are opaque strings, so they are never interpolated into a selector. The card is found
// by comparing the attribute, and the id is re-checked after the await in case the reader has
// already moved on to another annotation.
watch(() => props.activeAnnotationId, async (id) => {
  if (!id) return
  await nextTick()
  if (props.activeAnnotationId !== id) return
  const card = Array.from(document.querySelectorAll('[data-annotation-card]'))
    .find((element) => element.getAttribute('data-annotation-card') === id)
  card?.scrollIntoView({ block: 'nearest' })
})

function updateDraft(patch: Partial<AnnotationDraft>) {
  emit('update:draft', { ...props.draft, ...patch })
}
</script>

<template>
  <aside class="annotation-sidebar" :class="{ 'is-collapsed': !props.open }" aria-labelledby="annotation-sidebar-heading">
    <button
      type="button"
      class="annotation-sidebar-handle"
      :aria-expanded="props.open"
      :title="props.open ? '收起标注栏' : '展开标注栏'"
      :aria-label="props.open ? '收起标注栏' : '展开标注栏'"
      @click="emit('update:open', !props.open)"
    >
      <i class="pi" :class="props.open ? 'pi-chevron-right' : 'pi-chevron-left'" />
    </button>

    <div class="annotation-sidebar-body">
      <h2 id="annotation-sidebar-heading" class="annotation-sidebar-heading">标注 {{ props.annotations.length }}</h2>

      <section class="annotation-sidebar-panel">
        <div class="annotation-composer">
          <h3>新建标注</h3>
          <blockquote v-if="props.draft.quote">“{{ props.draft.quote }}”</blockquote>
          <p v-else class="annotation-empty-quote">先在正文里选中一段文字。</p>
          <RichTextEditor
            class="compact-rich-text-editor"
            :model-value="props.draft.note"
            @update:model-value="updateDraft({ note: $event })"
          />
          <div class="annotation-actions">
            <div class="annotation-colors">
              <button
                v-for="color in colors"
                :key="color"
                type="button"
                :data-color="color"
                :class="[color, { active: props.draft.color === color }]"
                :aria-label="`标注颜色 ${color}`"
                @click="updateDraft({ color })"
              />
            </div>
            <Button label="保存标注" size="small" :disabled="!canSave" @click="emit('save-annotation')" />
          </div>
        </div>
        <div class="annotation-list">
          <div
            v-for="item in props.annotations"
            :key="item.id"
            :data-annotation-card="item.id"
            class="annotation-card"
            :class="[item.color, { 'is-active': item.id === props.activeAnnotationId }]"
          >
            <small>{{ item.author_name }} · {{ new Date(item.created_at).toLocaleString() }}</small>
            <q v-if="item.quote">{{ item.quote }}</q>
            <RichTextContent :content="item.note" />
          </div>
        </div>
      </section>

    </div>
  </aside>
</template>
