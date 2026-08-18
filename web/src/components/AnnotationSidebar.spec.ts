import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import AnnotationSidebar from './AnnotationSidebar.vue'
import type { Annotation } from '../types'

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

function mountSidebar(props: Record<string, unknown> = {}) {
  return mount(AnnotationSidebar, {
    props: {
      open: true,
      annotations: [annotation('note-1', '第一条标注')],
      draft: { quote: '', note: '', color: 'yellow' },
      activeAnnotationId: '',
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

  it('names the panel for assistive technology without pretending to be a tablist', () => {
    const wrapper = mountSidebar()

    const heading = wrapper.get('.annotation-sidebar-heading')
    expect(heading.element.tagName).toBe('H2')
    expect(wrapper.get('.annotation-sidebar').attributes('aria-labelledby')).toBe(heading.attributes('id'))
    expect(wrapper.find('[role="tab"]').exists()).toBe(false)
    expect(wrapper.find('[role="tabpanel"]').exists()).toBe(false)
  })

  it('is an annotation panel with no tab strip left', () => {
    const wrapper = mountSidebar()

    expect(wrapper.get('.annotation-sidebar-heading').text()).toBe('标注 1')
    expect(wrapper.find('[role="tablist"]').exists()).toBe(false)
    expect(wrapper.find('[data-tab]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('讨论')
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

})
