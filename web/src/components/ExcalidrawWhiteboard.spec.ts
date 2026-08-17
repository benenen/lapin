import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import ExcalidrawWhiteboard from './ExcalidrawWhiteboard.vue'

const bridgeMock = vi.hoisted(() => ({
  destroy: vi.fn(),
  getDocument: vi.fn(),
  isReady: vi.fn(() => true),
  resize: vi.fn(),
  setSaving: vi.fn(),
}))

const mountExcalidrawMock = vi.hoisted(() => vi.fn(() => bridgeMock))
const animationFrames: FrameRequestCallback[] = []

vi.mock('../excalidrawBridge', () => ({
  mountExcalidraw: mountExcalidrawMock,
}))

vi.mock('../whiteboard', async (importOriginal) => ({
  ...await importOriginal<typeof import('../whiteboard')>(),
  chapterContentRevision: vi.fn(async () => 'sha256:test'),
}))

class ResizeObserverStub {
  observe() {}
  disconnect() {}
}

describe('ExcalidrawWhiteboard visibility', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', ResizeObserverStub)
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      animationFrames.push(callback)
      return animationFrames.length
    })
    animationFrames.length = 0
    vi.clearAllMocks()
  })

  it('keeps the same drawing session while the overlay is hidden', async () => {
    const wrapper = mount(ExcalidrawWhiteboard, {
      props: {
        chapterId: 'chapter-a',
        content: '# 正文',
        active: false,
      },
      global: {
        stubs: {
          Button: { template: '<button><slot /></button>' },
          Message: { template: '<div><slot /></div>' },
          RichTextContent: { template: '<div class="chapter-content">正文</div>' },
        },
      },
    })
    await flushPromises()

    expect(mountExcalidrawMock).toHaveBeenCalledTimes(1)
    animationFrames.length = 0
    await wrapper.setProps({ active: true })
    await flushPromises()

    const stage = wrapper.get('.whiteboard-stage')
    expect(stage.classes()).not.toContain('is-interactive')
    expect(animationFrames.length).toBeGreaterThan(0)
    animationFrames.shift()?.(performance.now())
    await flushPromises()
    expect(stage.classes()).not.toContain('is-interactive')
    expect(animationFrames.length).toBeGreaterThan(0)
    animationFrames.shift()?.(performance.now())
    await flushPromises()
    expect(stage.classes()).toContain('is-interactive')

    await wrapper.setProps({ active: false })
    await flushPromises()

    expect(bridgeMock.destroy).not.toHaveBeenCalled()
    expect(wrapper.get('.excalidraw-host').attributes('aria-hidden')).toBe('true')
    expect(stage.classes()).not.toContain('is-interactive')

    await wrapper.setProps({ active: true })
    await flushPromises()
    expect(mountExcalidrawMock).toHaveBeenCalledTimes(1)
  })

  it('does not let an old interaction frame unlock a rebuilt scene', async () => {
    const wrapper = mount(ExcalidrawWhiteboard, {
      props: {
        chapterId: 'chapter-a',
        content: '# 正文',
        active: false,
      },
      global: {
        stubs: {
          Button: { template: '<button><slot /></button>' },
          Message: { template: '<div><slot /></div>' },
          RichTextContent: { template: '<div class="chapter-content">正文</div>' },
        },
      },
    })
    await flushPromises()
    animationFrames.length = 0

    await wrapper.setProps({ active: true })
    await flushPromises()
    animationFrames.shift()?.(performance.now())
    await flushPromises()
    expect(animationFrames.length).toBeGreaterThan(0)

    await wrapper.setProps({
      modelValue: {
        version: 3,
        anchor: { type: 'chapter', id: 'chapter-a', content_revision: 'sha256:test' },
        space: { width: 960, height: 640, fit: 'contain' },
        document: { type: 'excalidraw', version: 2, elements: [], appState: {}, files: {} },
      },
    })
    await flushPromises()
    expect(mountExcalidrawMock).toHaveBeenCalledTimes(2)

    animationFrames.shift()?.(performance.now())
    await flushPromises()
    expect(wrapper.get('.whiteboard-stage').classes()).not.toContain('is-interactive')
  })
})
