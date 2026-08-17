import { beforeEach, describe, expect, it, vi } from 'vitest'

import { mountExcalidraw } from './excalidrawBridge'
import { loadExcalidrawScene } from './whiteboardScene'

const reactRoot = vi.hoisted(() => ({
  render: vi.fn(),
  unmount: vi.fn(),
}))

vi.mock('react-dom/client', () => ({
  createRoot: () => reactRoot,
}))

vi.mock('@excalidraw/excalidraw', () => ({
  CaptureUpdateAction: { NEVER: 'never', IMMEDIATELY: 'immediately' },
  Excalidraw: () => null,
}))

describe('Excalidraw bridge', () => {
  beforeEach(() => {
    reactRoot.render.mockClear()
    reactRoot.unmount.mockClear()
  })

  it('reports a persisted scene load failure without becoming ready', () => {
    const errors: Error[] = []

    const scene = loadExcalidrawScene(
      () => { throw new Error('invalid scene') },
      (error) => { errors.push(error) },
    )

    expect(scene).toBeNull()
    expect(errors[0]?.message).toBe('invalid scene')
  })

  it('extends the built-in toolbar instead of rendering a second toolbar', () => {
    const host = document.createElement('div')
    mountExcalidraw(host, { width: 960, height: 640, topInset: 0 })

    const excalidrawElement = reactRoot.render.mock.calls[0]?.[0]
    expect(excalidrawElement?.props.renderTopRightUI).toBeTypeOf('function')
  })

  it('locks scrolling and zooming to the chapter coordinate system', () => {
    const host = document.createElement('div')
    mountExcalidraw(host, { width: 960, height: 640, topInset: 0 })

    const excalidrawElement = reactRoot.render.mock.calls[0]?.[0]
    expect(excalidrawElement?.props.onScrollChange).toBeTypeOf('function')
    expect(host.dispatchEvent(new WheelEvent('wheel', { cancelable: true }))).toBe(false)
  })

  it('uses the macOS command modifier for toolbar history actions', () => {
    const platform = vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue('MacIntel')
    const host = document.createElement('div')
    const shortcuts: KeyboardEvent[] = []
    host.addEventListener('keydown', (event) => { shortcuts.push(event) })

    const bridge = mountExcalidraw(host, { width: 960, height: 640, topInset: 0 })
    bridge.undo()

    expect(shortcuts[0]?.metaKey).toBe(true)
    expect(shortcuts[0]?.ctrlKey).toBe(false)
    platform.mockRestore()
  })
})
