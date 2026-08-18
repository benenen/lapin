import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import RichTextContent from './RichTextContent.vue'

describe('RichTextContent', () => {
  it('uses an auto-height presentation surface', () => {
    const wrapper = shallowMount(RichTextContent, {
      props: { content: '一行短文本' },
      global: { stubs: { EditorContent: { template: '<div />' } } },
    })

    expect(wrapper.classes()).toContain('autosize-rich-text')
  })

  it('marks annotated text and reports which annotation was clicked', async () => {
    const wrapper = mount(RichTextContent, {
      props: {
        content: '上下文工程是核心。',
        annotations: [{ id: 'note-1', start_offset: 0, end_offset: 5, color: 'blue' }],
      },
      attachTo: document.body,
    })
    await flushPromises()

    const mark = wrapper.get('[data-annotation-id="note-1"]')
    expect(mark.classes()).toContain('annotation-mark-blue')
    expect(mark.text()).toBe('上下文工程')

    await mark.trigger('click')
    expect(wrapper.emitted('annotation-click')).toEqual([['note-1']])
    wrapper.unmount()
  })

  it('refreshes the marks when annotations change', async () => {
    const wrapper = mount(RichTextContent, {
      props: { content: '上下文工程是核心。', annotations: [] },
      attachTo: document.body,
    })
    await flushPromises()
    expect(wrapper.find('[data-annotation-id]').exists()).toBe(false)

    await wrapper.setProps({ annotations: [{ id: 'note-1', start_offset: 0, end_offset: 3, color: 'yellow' }] })
    await flushPromises()

    expect(wrapper.get('[data-annotation-id="note-1"]').text()).toBe('上下文')
    wrapper.unmount()
  })
})
