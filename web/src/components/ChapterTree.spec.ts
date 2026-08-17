import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { buildChapterTree } from '../chapterTree'
import type { Chapter } from '../types'
import ChapterTree from './ChapterTree.vue'

const chapters: Chapter[] = [
  {
    id: 'root', title: '第一章', content: '', position: 0,
    external_id: '', created_at: '',
  },
  {
    id: 'child', parent_id: 'root', title: '第一节', content: '', position: 1,
    external_id: '', created_at: '',
  },
]

describe('ChapterTree', () => {
  it('renders nested branches that can be collapsed and selected', async () => {
    const wrapper = mount(ChapterTree, {
      props: { nodes: buildChapterTree(chapters), activeChapterId: 'child' },
    })

    expect(wrapper.findAll('[role="tree"]')).toHaveLength(1)
    expect(wrapper.findAll('[role="group"]')).toHaveLength(1)
    expect(wrapper.get('[role="treeitem"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('.chapter-tree-label.active').text()).toContain('第一节')

    await wrapper.get('button[aria-label="收起第一章"]').trigger('click')
    expect(wrapper.findAll('[role="group"]')).toHaveLength(0)
    expect(wrapper.get('[role="treeitem"]').attributes('aria-expanded')).toBe('false')

    await wrapper.get('.chapter-tree-label').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual(['root'])
  })
})
