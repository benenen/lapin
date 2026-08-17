import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import RichTextEditor from './RichTextEditor.vue'

describe('RichTextEditor toolbar', () => {
  it('shows visible bold and italic controls without relying on missing icon fonts', async () => {
    const wrapper = mount(RichTextEditor, {
      props: { modelValue: '' },
    })
    await flushPromises()

    expect(wrapper.get('button[aria-label="粗体"]').text()).toBe('B')
    expect(wrapper.get('button[aria-label="斜体"]').text()).toBe('I')
  })
})
