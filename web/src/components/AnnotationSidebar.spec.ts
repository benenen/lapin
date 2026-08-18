import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AnnotationSidebar from './AnnotationSidebar.vue'
import type { Annotation, Comment } from '../types'

const annotation = (id: string, note: string): Annotation => ({
  id,
  chapter_id: 'chapter-a',
  user_id: 'user-a',
  author_name: '学习者',
  start_offset: 0,
  end_offset: 4,
  quote: '上下文',
  note,
  color: 'yellow',
  created_at: '2026-08-18T00:00:00Z',
})

const comment = (id: string, body: string): Comment => ({
  id,
  chapter_id: 'chapter-a',
  user_id: 'user-a',
  author_name: '学习者',
  body,
  created_at: '2026-08-18T00:00:00Z',
})

function mountSidebar(props: Record<string, unknown> = {}) {
  return mount(AnnotationSidebar, {
    props: {
      open: true,
      tab: 'annotations',
      annotations: [annotation('note-1', '第一条标注')],
      comments: [comment('comment-1', '第一条讨论')],
      draft: { quote: '', note: '', color: 'yellow' },
      activeAnnotationId: '',
      commentBody: '',
      userName: '学习者',
      ...props,
    },
    global: {
      stubs: {
        Avatar: { template: '<span />' },
        Button: {
          props: ['label', 'disabled'],
          emits: ['click'],
          template: `<button :disabled="disabled" @click="$emit('click')">{{ label }}</button>`,
        },
        RichTextEditor: {
          props: ['modelValue'],
          template: `<textarea class="rich-editor" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
        },
        RichTextContent: { props: ['content'], template: '<div class="rich-content">{{ content }}</div>' },
      },
    },
  })
}

describe('AnnotationSidebar', () => {
  it('toggles with the handle on its left edge', async () => {
    const wrapper = mountSidebar()

    const handle = wrapper.get('.annotation-sidebar-handle')
    expect(handle.attributes('aria-expanded')).toBe('true')
    await handle.trigger('click')

    expect(wrapper.emitted('update:open')).toEqual([[false]])
  })

  it('reports being collapsed to assistive technology', () => {
    const wrapper = mountSidebar({ open: false })

    expect(wrapper.get('.annotation-sidebar').classes()).toContain('is-collapsed')
    expect(wrapper.get('.annotation-sidebar-handle').attributes('aria-expanded')).toBe('false')
  })

  it('completes the tablist contract for assistive technology', async () => {
    const wrapper = mountSidebar()

    const annotationsTab = wrapper.get('button[data-tab="annotations"]')
    const commentsTab = wrapper.get('button[data-tab="comments"]')
    expect(wrapper.get('[role="tablist"]').attributes('aria-label')).toBe('标注与讨论')
    expect(annotationsTab.attributes('role')).toBe('tab')
    expect(commentsTab.attributes('role')).toBe('tab')
    expect(annotationsTab.attributes('aria-selected')).toBe('true')
    expect(commentsTab.attributes('aria-selected')).toBe('false')

    const panel = wrapper.get('.annotation-sidebar-panel')
    expect(panel.attributes('role')).toBe('tabpanel')
    expect(panel.attributes('aria-labelledby')).toBe(annotationsTab.attributes('id'))
    expect(annotationsTab.attributes('aria-controls')).toBe(panel.attributes('id'))

    await wrapper.setProps({ tab: 'comments' })

    const commentsPanel = wrapper.get('.annotation-sidebar-panel')
    expect(wrapper.get('button[data-tab="comments"]').attributes('aria-selected')).toBe('true')
    expect(commentsPanel.attributes('role')).toBe('tabpanel')
    expect(commentsPanel.attributes('aria-labelledby')).toBe(commentsTab.attributes('id'))
    expect(wrapper.get('button[data-tab="comments"]').attributes('aria-controls')).toBe(commentsPanel.attributes('id'))
  })

  it('switches between the annotation and discussion tabs', async () => {
    const wrapper = mountSidebar()

    await wrapper.get('button[data-tab="comments"]').trigger('click')

    expect(wrapper.emitted('update:tab')).toEqual([['comments']])
  })

  it('shows the captured quote and colour in the composer', () => {
    const wrapper = mountSidebar({ draft: { quote: '上下文工程', note: '', color: 'blue' } })

    expect(wrapper.get('.annotation-composer blockquote').text()).toContain('上下文工程')
    expect(wrapper.get('.annotation-composer button[data-color="blue"]').classes()).toContain('active')
  })

  it('keeps saving disabled until the note has content', async () => {
    const wrapper = mountSidebar()
    const save = wrapper.findAll('button').find((button) => button.text() === '保存标注')!
    expect(save.attributes('disabled')).toBeDefined()

    await wrapper.setProps({ draft: { quote: '上下文', note: '想法', color: 'yellow' } })
    const enabled = wrapper.findAll('button').find((button) => button.text() === '保存标注')!
    await enabled.trigger('click')

    expect(wrapper.emitted('save-annotation')).toHaveLength(1)
  })

  it('publishes note edits through update:draft', async () => {
    const wrapper = mountSidebar({ draft: { quote: '上下文', note: '', color: 'yellow' } })

    await wrapper.get('.annotation-composer .rich-editor').setValue('新的想法')

    expect(wrapper.emitted('update:draft')).toEqual([[{ quote: '上下文', note: '新的想法', color: 'yellow' }]])
  })

  it('highlights the annotation opened from the chapter text', () => {
    const wrapper = mountSidebar({ activeAnnotationId: 'note-1' })

    expect(wrapper.get('.annotation-card').classes()).toContain('is-active')
  })

  it('renders the discussion tab with its composer and list', async () => {
    const wrapper = mountSidebar({ tab: 'comments' })

    expect(wrapper.text()).toContain('第一条讨论')
    expect(wrapper.find('.annotation-composer').exists()).toBe(false)
    await wrapper.get('.comment-compose .rich-editor').setValue('新讨论')

    expect(wrapper.emitted('update:commentBody')).toEqual([['新讨论']])
  })
})
