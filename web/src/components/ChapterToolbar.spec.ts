import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import ChapterToolbar from './ChapterToolbar.vue'

function mountToolbar(props: Record<string, unknown> = {}) {
  return mount(ChapterToolbar, {
    props: {
      mode: 'reading',
      annotationCount: 2,
      commentCount: 3,
      quote: '',
      color: 'yellow',
      whiteboardDisabled: false,
      whiteboardLoading: false,
      whiteboardError: false,
      saving: false,
      ...props,
    },
  })
}

describe('ChapterToolbar', () => {
  it('shows the reading actions with their counts', () => {
    const wrapper = mountToolbar()

    const labels = wrapper.findAll('button').map((button) => button.text())
    expect(labels).toEqual(['白板', '标注 2', '讨论 3'])
  })

  it('opens the sidebar on the matching tab', async () => {
    const wrapper = mountToolbar()

    await wrapper.findAll('button')[1]!.trigger('click')
    await wrapper.findAll('button')[2]!.trigger('click')

    expect(wrapper.emitted('open-sidebar')).toEqual([['annotations'], ['comments']])
  })

  it('morphs into the selection actions and keeps the quote visible', async () => {
    const wrapper = mountToolbar({ mode: 'selecting', quote: '上下文工程是核心' })

    expect(wrapper.get('.chapter-toolbar-quote').text()).toContain('上下文工程是核心')
    await wrapper.get('button[data-color="blue"]').trigger('click')
    await wrapper.get('button[data-action="compose"]').trigger('click')
    await wrapper.get('button[data-action="cancel"]').trigger('click')

    expect(wrapper.emitted('pick-color')).toEqual([['blue']])
    expect(wrapper.emitted('compose-annotation')).toHaveLength(1)
    expect(wrapper.emitted('cancel-selection')).toHaveLength(1)
  })

  it('marks the chosen colour as active', () => {
    const wrapper = mountToolbar({ mode: 'selecting', quote: '正文', color: 'pink' })

    expect(wrapper.get('button[data-color="pink"]').classes()).toContain('active')
    expect(wrapper.get('button[data-color="yellow"]').classes()).not.toContain('active')
  })

  it('exposes the whiteboard actions only while the whiteboard is open', async () => {
    const reading = mountToolbar()
    expect(reading.find('button[data-action="undo"]').exists()).toBe(false)

    const wrapper = mountToolbar({ mode: 'whiteboard' })
    await wrapper.get('button[data-action="undo"]').trigger('click')
    await wrapper.get('button[data-action="redo"]').trigger('click')
    await wrapper.get('button[data-action="clear"]').trigger('click')
    await wrapper.get('button[data-action="save-whiteboard"]').trigger('click')

    expect(wrapper.emitted('undo')).toHaveLength(1)
    expect(wrapper.emitted('redo')).toHaveLength(1)
    expect(wrapper.emitted('clear')).toHaveLength(1)
    expect(wrapper.emitted('save-whiteboard')).toHaveLength(1)
  })

  it('disables saving the whiteboard while a save is in flight', () => {
    const wrapper = mountToolbar({ mode: 'whiteboard', saving: true })

    expect(wrapper.get('button[data-action="save-whiteboard"]').attributes('disabled')).toBeDefined()
  })

  it('blocks the whiteboard until persisted data has loaded', () => {
    const wrapper = mountToolbar({ whiteboardDisabled: true })

    expect(wrapper.get('button[data-action="whiteboard"]').attributes('disabled')).toBeDefined()
  })

  it('offers a retry instead of a toggle when the whiteboard failed to load', async () => {
    const wrapper = mountToolbar({ whiteboardError: true })

    const retry = wrapper.get('button[data-action="retry-whiteboard"]')
    expect(retry.text()).toBe('重试白板')
    await retry.trigger('click')

    expect(wrapper.emitted('retry-whiteboard')).toHaveLength(1)
    expect(wrapper.find('button[data-action="whiteboard"]').exists()).toBe(false)
  })
})
