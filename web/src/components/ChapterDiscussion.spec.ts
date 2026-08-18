import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import ChapterDiscussion from './ChapterDiscussion.vue'
import type { Comment } from '../types'

const comment = (id: string, body: string): Comment => ({
  id,
  chapter_id: 'chapter-a',
  user_id: 'user-a',
  author_name: '学习者',
  body,
  created_at: '2026-08-18T00:00:00Z',
})

function mountDiscussion(props: Record<string, unknown> = {}) {
  return mount(ChapterDiscussion, {
    props: {
      comments: [comment('comment-1', '第一条讨论')],
      body: '',
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

describe('ChapterDiscussion', () => {
  it('renders the discussion with its heading, composer and list', () => {
    const wrapper = mountDiscussion()

    expect(wrapper.get('.chapter-discussion h2').text()).toContain('讨论 1')
    expect(wrapper.find('.comment-compose .rich-editor').exists()).toBe(true)
    expect(wrapper.text()).toContain('第一条讨论')
  })

  it('keeps sending disabled until the body has content', async () => {
    const wrapper = mountDiscussion()
    const send = wrapper.findAll('button').find((button) => button.text() === '发送')!
    expect(send.attributes('disabled')).toBeDefined()

    await wrapper.setProps({ body: '一个问题' })
    await wrapper.findAll('button').find((button) => button.text() === '发送')!.trigger('click')

    expect(wrapper.emitted('post')).toHaveLength(1)
  })

  it('will not send a body that is only whitespace', async () => {
    const wrapper = mountDiscussion({ body: '   ' })

    expect(wrapper.findAll('button').find((button) => button.text() === '发送')!.attributes('disabled')).toBeDefined()
  })

  it('publishes body edits through update:body', async () => {
    const wrapper = mountDiscussion()

    await wrapper.get('.comment-compose .rich-editor').setValue('新讨论')

    expect(wrapper.emitted('update:body')).toEqual([['新讨论']])
  })

  it('shows the empty state when nobody has posted yet', () => {
    const wrapper = mountDiscussion({ comments: [] })

    expect(wrapper.get('.empty-comments').text()).toBe('还没有讨论，来提出第一个问题吧。')
    expect(wrapper.get('.chapter-discussion h2').text()).toContain('讨论 0')
  })
})
