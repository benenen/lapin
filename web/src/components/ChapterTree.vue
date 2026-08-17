<script setup lang="ts">
import { ref, watch } from 'vue'

import type { ChapterTreeNode } from '../chapterTree'

defineOptions({ name: 'ChapterTree' })

const props = defineProps<{
  nodes: readonly ChapterTreeNode[]
  activeChapterId: string
  nested?: boolean
}>()

const emit = defineEmits<{
  select: [chapterId: string]
}>()

const expandedIDs = ref<ReadonlySet<string>>(new Set())

watch(
  () => props.nodes,
  (nodes) => {
    expandedIDs.value = new Set(nodes.filter((node) => node.children.length > 0).map((node) => node.chapter.id))
  },
  { immediate: true },
)

function hasChildren(node: ChapterTreeNode): boolean {
  return node.children.length > 0
}

function isExpanded(node: ChapterTreeNode): boolean {
  return expandedIDs.value.has(node.chapter.id)
}

function toggle(node: ChapterTreeNode) {
  expandedIDs.value = isExpanded(node)
    ? new Set([...expandedIDs.value].filter((id) => id !== node.chapter.id))
    : new Set([...expandedIDs.value, node.chapter.id])
}
</script>

<template>
  <ul class="chapter-tree" :role="nested ? 'group' : 'tree'">
    <li
      v-for="node in nodes"
      :key="node.chapter.id"
      role="treeitem"
      :aria-expanded="hasChildren(node) ? isExpanded(node) : undefined"
    >
      <div class="chapter-tree-row">
        <button
          v-if="hasChildren(node)"
          type="button"
          class="chapter-tree-toggle"
          :aria-label="`${isExpanded(node) ? '收起' : '展开'}${node.chapter.title}`"
          @click="toggle(node)"
        >
          <i class="pi" :class="isExpanded(node) ? 'pi-chevron-down' : 'pi-chevron-right'" />
        </button>
        <span v-else class="chapter-tree-toggle-placeholder" aria-hidden="true" />
        <button
          type="button"
          class="chapter-tree-label"
          :class="{ active: node.chapter.id === activeChapterId }"
          @click="emit('select', node.chapter.id)"
        >
          <span>{{ String(node.chapter.position + 1).padStart(2, '0') }}</span>
          {{ node.chapter.title }}
        </button>
      </div>
      <ChapterTree
        v-if="hasChildren(node) && isExpanded(node)"
        :nodes="node.children"
        :active-chapter-id="activeChapterId"
        nested
        @select="emit('select', $event)"
      />
    </li>
  </ul>
</template>
