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

  it('keeps the canvas camera locked while allowing document wheel scrolling', async () => {
    const host = document.createElement('div')
    const parent = document.createElement('div')
    const bubbledWheel = vi.fn()
    const scrollBy = vi.spyOn(window, 'scrollBy').mockImplementation(() => {})
    parent.addEventListener('wheel', bubbledWheel)
    parent.append(host)
    const bridge = mountExcalidraw(host, { width: 960, height: 640 })

    const excalidrawElement = reactRoot.render.mock.calls[0]?.[0]
    expect(excalidrawElement?.props.detectScroll).toBe(true)
    expect(excalidrawElement?.props.onScrollChange).toBeTypeOf('function')
    const documentScroll = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY: 120 })
    expect(host.dispatchEvent(documentScroll)).toBe(false)
    expect(documentScroll.defaultPrevented).toBe(true)
    expect(scrollBy).toHaveBeenCalledWith(0, 120)
    expect(bubbledWheel).not.toHaveBeenCalled()

    const zoom = new WheelEvent('wheel', { bubbles: true, cancelable: true, ctrlKey: true, deltaY: 120 })
    expect(host.dispatchEvent(zoom)).toBe(false)
    expect(zoom.defaultPrevented).toBe(true)
    expect(scrollBy).toHaveBeenCalledTimes(1)
    expect(bubbledWheel).not.toHaveBeenCalled()

    const api = {
      refresh: vi.fn(),
      setActiveTool: vi.fn(),
      updateScene: vi.fn(),
    }
    excalidrawElement.props.excalidrawAPI(api)
    api.refresh.mockClear()
    window.dispatchEvent(new Event('scroll'))
    window.dispatchEvent(new Event('scroll'))
    window.dispatchEvent(new Event('scroll'))
    expect(api.refresh).not.toHaveBeenCalled()
    await new Promise((resolve) => requestAnimationFrame(resolve))
    expect(api.refresh).toHaveBeenCalledTimes(1)

    host.setAttribute('aria-hidden', 'true')
    api.refresh.mockClear()
    window.dispatchEvent(new Event('scroll'))
    await new Promise((resolve) => requestAnimationFrame(resolve))
    expect(api.refresh).not.toHaveBeenCalled()
    host.removeAttribute('aria-hidden')

    api.refresh.mockClear()
    window.dispatchEvent(new Event('scroll'))
    host.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    expect(api.refresh).toHaveBeenCalledTimes(1)
    await new Promise((resolve) => requestAnimationFrame(resolve))
    expect(api.refresh).toHaveBeenCalledTimes(1)

    api.updateScene.mockClear()
    excalidrawElement.props.onScrollChange(120, 80, { value: 2 })
    await new Promise((resolve) => requestAnimationFrame(resolve))

    expect(api.updateScene).toHaveBeenCalledWith(expect.objectContaining({
      appState: {
        scrollX: 0,
        scrollY: 0,
        zoom: { value: 1 },
      },
    }))

    api.refresh.mockClear()
    window.dispatchEvent(new Event('scroll'))
    bridge.destroy()
    await new Promise((resolve) => requestAnimationFrame(resolve))
    host.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
    expect(api.refresh).not.toHaveBeenCalled()
    scrollBy.mockRestore()
  })

  it('anchors the scene to the drawable window offset inside the chapter', () => {
    const host = document.createElement('div')
    let offsetTop = 0
    mountExcalidraw(host, { width: 960, height: 640, offsetTop: () => offsetTop })

    const excalidrawElement = reactRoot.render.mock.calls[0]?.[0]
    const api = {
      refresh: vi.fn(),
      setActiveTool: vi.fn(),
      updateScene: vi.fn(),
    }
    excalidrawElement.props.excalidrawAPI(api)

    expect(api.updateScene).toHaveBeenCalledWith(expect.objectContaining({
      appState: { scrollX: 0, scrollY: 0, zoom: { value: 1 } },
    }))

    offsetTop = 5100
    api.updateScene.mockClear()
    excalidrawElement.props.excalidrawAPI(api)

    expect(api.updateScene).toHaveBeenCalledWith(expect.objectContaining({
      appState: { scrollX: 0, scrollY: -5100, zoom: { value: 1 } },
    }))
  })

  // Excalidraw runs with handleKeyboardGlobally: false, so its key handling lives on a React
  // onKeyDown prop on `.excalidraw-container`. Dispatching on the React root — the container's
  // parent — never reaches that handler, which left 撤销 and 重做 silent no-ops.
  it('sends history shortcuts to the Excalidraw container, not the React root', () => {
    const host = document.createElement('div')
    const container = document.createElement('div')
    container.className = 'excalidraw excalidraw-container'
    host.append(container)
    const onContainer: KeyboardEvent[] = []
    const onHost: KeyboardEvent[] = []
    container.addEventListener('keydown', (event) => { onContainer.push(event) })
    host.addEventListener('keydown', (event) => { onHost.push(event) })

    const bridge = mountExcalidraw(host, { width: 960, height: 640 })
    bridge.undo()

    expect(onContainer).toHaveLength(1)
    expect(onContainer[0]?.target).toBe(container)
    expect(onContainer[0]?.key).toBe('z')
    expect(onContainer[0]?.ctrlKey).toBe(true)
    expect(onContainer[0]?.metaKey).toBe(false)
    expect(onContainer[0]?.shiftKey).toBe(false)
    expect(onHost[0]?.target).toBe(container)

    bridge.redo()

    expect(onContainer[1]?.target).toBe(container)
    expect(onContainer[1]?.shiftKey).toBe(true)
  })

  it('falls back to the React root when Excalidraw has not rendered its container yet', () => {
    const host = document.createElement('div')
    const shortcuts: KeyboardEvent[] = []
    host.addEventListener('keydown', (event) => { shortcuts.push(event) })

    mountExcalidraw(host, { width: 960, height: 640 }).undo()

    expect(shortcuts[0]?.target).toBe(host)
  })

  it('uses the macOS command modifier for toolbar history actions', () => {
    const platform = vi.spyOn(window.navigator, 'platform', 'get').mockReturnValue('MacIntel')
    const host = document.createElement('div')
    const shortcuts: KeyboardEvent[] = []
    host.addEventListener('keydown', (event) => { shortcuts.push(event) })

    const bridge = mountExcalidraw(host, { width: 960, height: 640 })
    bridge.undo()

    expect(shortcuts[0]?.metaKey).toBe(true)
    expect(shortcuts[0]?.ctrlKey).toBe(false)
    platform.mockRestore()
  })
})
