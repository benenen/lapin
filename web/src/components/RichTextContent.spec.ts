import { shallowMount } from '@vue/test-utils'
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
})
