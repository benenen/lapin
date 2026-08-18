<script setup lang="ts">
import { computed, nextTick, watch } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'

import type { Annotation, Comment } from '../types'
import RichTextContent from './RichTextContent.vue'
import RichTextEditor from './RichTextEditor.vue'

interface AnnotationDraft {
  quote: string
  note: string
  color: string
}

const props = defineProps<{
  open: boolean
  tab: 'annotations' | 'comments'
  annotations: Annotation[]
  comments: Comment[]
  draft: AnnotationDraft
  activeAnnotationId: string
  commentBody: string
  userName: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'update:tab': [tab: 'annotations' | 'comments']
  'update:draft': [draft: AnnotationDraft]
  'update:commentBody': [body: string]
  'save-annotation': []
  'post-comment': []
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
  <aside class="annotation-sidebar" :class="{ 'is-collapsed': !props.open }">
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
      <div class="annotation-sidebar-tabs" role="tablist" aria-label="标注与讨论">
        <button
          id="annotation-sidebar-tab-annotations"
          type="button"
          role="tab"
          data-tab="annotations"
          :aria-selected="props.tab === 'annotations'"
          aria-controls="annotation-sidebar-panel-annotations"
          :tabindex="props.tab === 'annotations' ? 0 : -1"
          :class="{ active: props.tab === 'annotations' }"
          @click="emit('update:tab', 'annotations')"
        >
          标注 {{ props.annotations.length }}
        </button>
        <button
          id="annotation-sidebar-tab-comments"
          type="button"
          role="tab"
          data-tab="comments"
          :aria-selected="props.tab === 'comments'"
          aria-controls="annotation-sidebar-panel-comments"
          :tabindex="props.tab === 'comments' ? 0 : -1"
          :class="{ active: props.tab === 'comments' }"
          @click="emit('update:tab', 'comments')"
        >
          讨论 {{ props.comments.length }}
        </button>
      </div>

      <section
        v-if="props.tab === 'annotations'"
        id="annotation-sidebar-panel-annotations"
        class="annotation-sidebar-panel"
        role="tabpanel"
        tabindex="0"
        aria-labelledby="annotation-sidebar-tab-annotations"
      >
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

      <section
        v-else
        id="annotation-sidebar-panel-comments"
        class="annotation-sidebar-panel comments-panel"
        role="tabpanel"
        tabindex="0"
        aria-labelledby="annotation-sidebar-tab-comments"
      >
        <div class="comment-compose">
          <Avatar :label="props.userName.slice(0, 1)" shape="circle" />
          <RichTextEditor
            class="compact-rich-text-editor"
            :model-value="props.commentBody"
            @update:model-value="emit('update:commentBody', $event)"
          />
          <Button label="发送" icon="pi pi-send" :disabled="!props.commentBody.trim()" @click="emit('post-comment')" />
        </div>
        <div v-if="props.comments.length === 0" class="empty-comments">还没有讨论，来提出第一个问题吧。</div>
        <div v-for="item in props.comments" :key="item.id" class="comment-item">
          <Avatar :label="item.author_name.slice(0, 1)" shape="circle" />
          <div>
            <strong>{{ item.author_name }}</strong>
            <small>{{ new Date(item.created_at).toLocaleString() }}</small>
            <RichTextContent :content="item.body" />
          </div>
        </div>
      </section>
    </div>
  </aside>
</template>
