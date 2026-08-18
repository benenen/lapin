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

const mountExcalidrawMock = vi.hoisted(() => vi.fn((_host: HTMLElement, _options: { offsetTop?: () => number }) => bridgeMock))
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

  it('bounds the drawable overlay on a chapter taller than a browser canvas', async () => {
    const scrollHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollHeight')
    Object.defineProperty(HTMLElement.prototype, 'scrollHeight', {
      configurable: true,
      get(this: HTMLElement) { return this.classList.contains('chapter-content') ? 73600 : 0 },
    })
    vi.stubGlobal('innerHeight', 900)

    try {
      const wrapper = mount(ExcalidrawWhiteboard, {
        props: {
          chapterId: 'chapter-a',
          content: '# 正文',
          active: true,
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

      expect(wrapper.get('.whiteboard-stage').attributes('style')).toContain('height: 73680px')
      const host = wrapper.get('.excalidraw-host').attributes('style') ?? ''
      expect(host).toContain('height: 2700px')
      expect(host).toContain('top: 0px')
      expect(mountExcalidrawMock.mock.calls[0]?.[1].offsetTop?.()).toBe(0)
    } finally {
      if (scrollHeight) Object.defineProperty(HTMLElement.prototype, 'scrollHeight', scrollHeight)
      vi.unstubAllGlobals()
    }
  })

  it('follows the reader down a long chapter without growing the overlay', async () => {
    const scrollHeight = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollHeight')
    const boundingRect = HTMLElement.prototype.getBoundingClientRect
    Object.defineProperty(HTMLElement.prototype, 'scrollHeight', {
      configurable: true,
      get(this: HTMLElement) { return this.classList.contains('chapter-content') ? 73600 : 0 },
    })
    vi.stubGlobal('innerHeight', 900)

    try {
      const wrapper = mount(ExcalidrawWhiteboard, {
        props: {
          chapterId: 'chapter-a',
          content: '# 正文',
          active: true,
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

      HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement) {
        const top = this.classList.contains('whiteboard-stage') ? -6000 : 0
        return { x: 0, y: top, top, bottom: top, left: 0, right: 0, width: 0, height: 0, toJSON: () => ({}) }
      }
      window.dispatchEvent(new Event('scroll'))
      animationFrames.splice(0).forEach((frame) => frame(performance.now()))
      await flushPromises()

      const host = wrapper.get('.excalidraw-host').attributes('style') ?? ''
      expect(host).toContain('height: 2700px')
      expect(host).toContain('top: 5100px')
      expect(mountExcalidrawMock.mock.calls[0]?.[1].offsetTop?.()).toBe(5100)
      expect(bridgeMock.resize).toHaveBeenCalled()
    } finally {
      HTMLElement.prototype.getBoundingClientRect = boundingRect
      if (scrollHeight) Object.defineProperty(HTMLElement.prototype, 'scrollHeight', scrollHeight)
      vi.unstubAllGlobals()
    }
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
