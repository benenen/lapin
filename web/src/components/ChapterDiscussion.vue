<script setup lang="ts">
import { computed } from 'vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'

import type { Comment } from '../types'
import RichTextContent from './RichTextContent.vue'
import RichTextEditor from './RichTextEditor.vue'

const props = defineProps<{
  comments: Comment[]
  body: string
  userName: string
}>()

const emit = defineEmits<{
  'update:body': [body: string]
  post: []
}>()

const canPost = computed(() => props.body.trim().length > 0)
</script>

<template>
  <section class="chapter-discussion" aria-labelledby="chapter-discussion-heading">
    <h2 id="chapter-discussion-heading">讨论 {{ props.comments.length }}</h2>
    <div class="comment-compose">
      <Avatar :label="props.userName.slice(0, 1)" shape="circle" />
      <RichTextEditor
        class="compact-rich-text-editor"
        :model-value="props.body"
        @update:model-value="emit('update:body', $event)"
      />
      <Button label="发送" icon="pi pi-send" :disabled="!canPost" @click="emit('post')" />
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
</template>
