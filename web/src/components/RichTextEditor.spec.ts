import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import RichTextEditor from './RichTextEditor.vue'

const uploadAsset = vi.hoisted(() => vi.fn())

vi.mock('../api', () => ({ api: { uploadAsset } }))

describe('RichTextEditor toolbar', () => {
  beforeEach(() => uploadAsset.mockReset())

  it('shows visible bold and italic controls without relying on missing icon fonts', async () => {
    const wrapper = mount(RichTextEditor, {
      props: { modelValue: '' },
    })
    await flushPromises()

    expect(wrapper.get('button[aria-label="粗体"]').text()).toBe('B')
    expect(wrapper.get('button[aria-label="斜体"]').text()).toBe('I')
    expect(wrapper.find('button[aria-label="插入图片"]').exists()).toBe(false)
  })

  it('uploads an image and inserts its same-origin Markdown reference', async () => {
    uploadAsset.mockResolvedValue({
      id: 'abcdefghij',
      url: '/api/v1/assets/abcdefghij/content',
      sha256: 'a'.repeat(64),
      mime_type: 'image/png',
      size: 3,
      width: 1,
      height: 1,
    })
    const wrapper = mount(RichTextEditor, { props: { modelValue: '', allowImages: true } })
    await flushPromises()

    await wrapper.get('button[aria-label="插入图片"]').trigger('click')
    const input = wrapper.get('input[type="file"]')
    const file = new File(['png'], 'figure.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await flushPromises()

    expect(uploadAsset).toHaveBeenCalledWith(file)
    expect(wrapper.html()).toContain('src="/api/v1/assets/abcdefghij/content"')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toContain('![figure.png](/api/v1/assets/abcdefghij/content "figure.png")')
  })
})
