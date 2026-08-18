<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  mode: 'reading' | 'selecting' | 'whiteboard'
  annotationCount: number
  commentCount: number
  quote: string
  color: string
  whiteboardDisabled: boolean
  whiteboardLoading: boolean
  whiteboardError: boolean
  saving: boolean
}>()

const emit = defineEmits<{
  'toggle-whiteboard': []
  'retry-whiteboard': []
  'open-sidebar': [tab: 'annotations' | 'comments']
  'pick-color': [color: string]
  'compose-annotation': []
  'cancel-selection': []
  undo: []
  redo: []
  clear: []
  'save-whiteboard': []
}>()

const colors = ['yellow', 'green', 'blue', 'pink']
const selecting = computed(() => props.mode === 'selecting')
const whiteboardOpen = computed(() => props.mode === 'whiteboard')
</script>

<template>
  <div class="chapter-toolbar" :class="`is-${props.mode}`" role="toolbar" aria-label="章节操作">
    <template v-if="selecting">
      <span class="chapter-toolbar-quote">“{{ props.quote }}”</span>
      <div class="chapter-toolbar-colors">
        <button
          v-for="item in colors"
          :key="item"
          type="button"
          :data-color="item"
          :class="[item, { active: props.color === item }]"
          :aria-label="`标注颜色 ${item}`"
          :aria-pressed="props.color === item"
          @click="emit('pick-color', item)"
        />
      </div>
      <button type="button" data-action="compose" class="chapter-toolbar-primary" @click="emit('compose-annotation')">
        <i class="pi pi-pencil" /> 写标注
      </button>
      <span class="chapter-toolbar-divider" aria-hidden="true" />
    </template>

    <!-- The whiteboard exit stays reachable in every mode, so it is defined once here. -->
    <button
      v-if="props.whiteboardError"
      type="button"
      data-action="retry-whiteboard"
      @click="emit('retry-whiteboard')"
    >
      <i class="pi pi-refresh" /> 重试白板
    </button>
    <button
      v-else
      type="button"
      data-action="whiteboard"
      :class="{ active: whiteboardOpen }"
      :disabled="props.whiteboardDisabled || props.whiteboardLoading"
      :aria-pressed="whiteboardOpen"
      @click="emit('toggle-whiteboard')"
    >
      <i class="pi" :class="whiteboardOpen ? 'pi-eye-slash' : 'pi-eye'" /> 白板
    </button>

    <template v-if="whiteboardOpen">
      <span class="chapter-toolbar-divider" aria-hidden="true" />
      <button type="button" data-action="undo" aria-label="撤销" @click="emit('undo')"><i class="pi pi-undo" /></button>
      <button type="button" data-action="redo" aria-label="重做" @click="emit('redo')"><i class="pi pi-refresh" /></button>
      <button type="button" data-action="clear" aria-label="清空白板" @click="emit('clear')"><i class="pi pi-trash" /></button>
      <button type="button" data-action="save-whiteboard" aria-label="保存白板" :disabled="props.saving" @click="emit('save-whiteboard')"><i class="pi pi-check" /></button>
      <span class="chapter-toolbar-divider" aria-hidden="true" />
    </template>

    <button v-if="selecting" type="button" data-action="cancel" aria-label="取消选区" @click="emit('cancel-selection')">
      <i class="pi pi-times" />
    </button>
    <template v-else>
      <button type="button" data-action="annotations" @click="emit('open-sidebar', 'annotations')">
        <i class="pi pi-pencil" /> 标注 {{ props.annotationCount }}
      </button>
      <button v-if="!whiteboardOpen" type="button" data-action="comments" @click="emit('open-sidebar', 'comments')">
        <i class="pi pi-comments" /> 讨论 {{ props.commentCount }}
      </button>
    </template>
  </div>
</template>
