import {
  CaptureUpdateAction,
  Excalidraw,
} from '@excalidraw/excalidraw'
import type { ExcalidrawImperativeAPI, ExcalidrawInitialDataState, NormalizedZoomValue } from '@excalidraw/excalidraw/types'
import { createElement } from 'react'
import { flushSync } from 'react-dom'
import { createRoot } from 'react-dom/client'

import type { WhiteboardData } from './types'
import { loadExcalidrawScene } from './whiteboardScene'
import { excalidrawViewport, isSupportedExcalidrawElement } from './whiteboard'

export interface ExcalidrawBridge {
  destroy: () => void
  isReady: () => boolean
  resize: () => void
  undo: () => void
  redo: () => void
  clear: () => void
  getDocument: () => WhiteboardData['document']
}

interface MountOptions {
  data?: WhiteboardData | null
  width: number
  height: number
  offsetTop?: () => number
  onReady?: () => void
  onError?: (error: Error) => void
}

function historyShortcutModifiers(): { ctrlKey: boolean; metaKey: boolean } {
  const platform = typeof navigator === 'undefined' ? '' : `${navigator.platform} ${navigator.userAgent}`
  const applePlatform = /Mac|iPhone|iPad|iPod/i.test(platform)
  return { ctrlKey: !applePlatform, metaKey: applePlatform }
}

export function mountExcalidraw(element: HTMLElement, options: MountOptions): ExcalidrawBridge {
  const root = createRoot(element)
  let api: ExcalidrawImperativeAPI | null = null
  let loadFailed = false
  let viewportResetPending = false
  element.tabIndex = -1

  const lockCanvasWheel = (event: WheelEvent) => {
    // Excalidraw also listens above the host and may cancel the browser default
    // before this capture handler runs. Route plain wheel deltas to the document
    // explicitly, while keeping every wheel gesture out of the fixed canvas.
    event.preventDefault()
    if (!event.ctrlKey && !event.metaKey) {
      const multiplier = event.deltaMode === WheelEvent.DOM_DELTA_LINE
        ? 16
        : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? window.innerHeight : 1
      window.scrollBy(event.deltaX * multiplier, event.deltaY * multiplier)
    }
    event.stopImmediatePropagation()
  }
  const preventPinch = (event: TouchEvent) => {
    if (event.touches.length < 2) return
    event.preventDefault()
    event.stopImmediatePropagation()
  }
  let scrollRefreshFrame: number | null = null
  const refreshViewportOffset = () => {
    if (element.getAttribute('aria-hidden') === 'true' || scrollRefreshFrame !== null) return
    scrollRefreshFrame = requestAnimationFrame(() => {
      scrollRefreshFrame = null
      api?.refresh()
    })
  }
  const refreshViewportOffsetBeforePointer = () => {
    if (scrollRefreshFrame !== null) {
      cancelAnimationFrame(scrollRefreshFrame)
      scrollRefreshFrame = null
    }
    // Excalidraw caches DOM offsets. Commit its refresh before React's bubble-phase pointer handler.
    if (api) flushSync(() => api?.refresh())
  }
  element.addEventListener('wheel', lockCanvasWheel, { capture: true, passive: false })
  element.addEventListener('touchmove', preventPinch, { capture: true, passive: false })
  element.addEventListener('pointerdown', refreshViewportOffsetBeforePointer, { capture: true })
  window.addEventListener('scroll', refreshViewportOffset, { capture: true, passive: true })

  const initialData = loadExcalidrawScene<ExcalidrawInitialDataState>(() => ({
    elements: options.data?.document.elements as ExcalidrawInitialDataState['elements'],
    appState: {
      ...(options.data?.document.appState ?? {}),
      scrollX: 0,
      scrollY: 0,
      viewBackgroundColor: 'transparent',
      zenModeEnabled: true,
    },
    files: {},
  }), options.onError)
  if (!initialData) loadFailed = true

  const expectedViewport = () => excalidrawViewport(element.getBoundingClientRect().width, options.width, options.offsetTop?.() ?? 0)

  const resize = () => {
    if (!api) return
    const viewport = expectedViewport()
    api.refresh()
    api.updateScene({
      appState: {
        scrollX: viewport.scrollX,
        scrollY: viewport.scrollY,
        zoom: { value: viewport.zoom as NormalizedZoomValue },
      },
      captureUpdate: CaptureUpdateAction.NEVER,
    })
  }

  // Excalidraw is mounted with handleKeyboardGlobally: false, so undo/redo hang off a React
  // onKeyDown prop on `.excalidraw-container` — a child of the root we render into. A keydown
  // dispatched on the parent never reaches a child handler, so aim at the container itself.
  const triggerHistoryShortcut = (redo: boolean) => {
    const target = (element.querySelector('.excalidraw-container') as HTMLElement | null) ?? element
    target.focus()
    target.dispatchEvent(new KeyboardEvent('keydown', {
      key: 'z',
      code: 'KeyZ',
      ...historyShortcutModifiers(),
      shiftKey: redo,
      bubbles: true,
      cancelable: true,
    }))
  }

  const clear = () => {
    if (api) api.updateScene({ elements: [], captureUpdate: CaptureUpdateAction.IMMEDIATELY })
  }

  if (!loadFailed) {
    root.render(createElement(Excalidraw, {
      initialData,
      autoFocus: false,
      detectScroll: true,
      handleKeyboardGlobally: false,
      zenModeEnabled: true,
      onScrollChange: (scrollX, scrollY, zoom) => {
        if (!api || viewportResetPending) return
        const expected = expectedViewport()
        if (scrollX === expected.scrollX && scrollY === expected.scrollY && zoom.value === expected.zoom) return
        viewportResetPending = true
        requestAnimationFrame(() => {
          viewportResetPending = false
          resize()
        })
      },
      UIOptions: {
        canvasActions: {
          changeViewBackgroundColor: false,
          clearCanvas: false,
          export: false,
          loadScene: false,
          saveAsImage: false,
          saveToActiveFile: false,
          toggleTheme: false,
        },
        tools: { image: false },
      },
      excalidrawAPI: (mountedAPI) => {
        api = mountedAPI
        resize()
        requestAnimationFrame(() => {
          if (api !== mountedAPI) return
          mountedAPI.setActiveTool({ type: 'freedraw' })
          options.onReady?.()
        })
      },
    }))
  }

  return {
    destroy: () => {
      api = null
      if (scrollRefreshFrame !== null) cancelAnimationFrame(scrollRefreshFrame)
      scrollRefreshFrame = null
      element.removeEventListener('wheel', lockCanvasWheel, { capture: true })
      element.removeEventListener('touchmove', preventPinch, { capture: true })
      element.removeEventListener('pointerdown', refreshViewportOffsetBeforePointer, { capture: true })
      window.removeEventListener('scroll', refreshViewportOffset, { capture: true })
      root.unmount()
    },
    isReady: () => api !== null && !loadFailed,
    resize,
    undo: () => triggerHistoryShortcut(false),
    redo: () => triggerHistoryShortcut(true),
    clear,
    getDocument: () => {
      if (!api) throw new Error('白板仍在加载')
      if (Object.keys(api.getFiles()).length > 0) throw new Error('当前白板暂不支持图片，请删除图片后再保存')
      const elements = api.getSceneElements()
      if (elements.some((item) => !isSupportedExcalidrawElement(item.type))) {
        throw new Error('当前白板不支持这个扩展工具，请删除对应内容后再保存')
      }
      return JSON.parse(JSON.stringify({
        type: 'excalidraw',
        version: 2,
        elements,
        appState: { viewBackgroundColor: 'transparent' },
        files: {},
      })) as WhiteboardData['document']
    },
  }
}
