import {
  CaptureUpdateAction,
  Excalidraw,
} from '@excalidraw/excalidraw'
import type { ExcalidrawImperativeAPI, ExcalidrawInitialDataState, NormalizedZoomValue } from '@excalidraw/excalidraw/types'
import { createElement, Fragment, useLayoutEffect, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { createPortal, flushSync } from 'react-dom'
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
  setSaving: (saving: boolean) => void
  getDocument: () => WhiteboardData['document']
}

interface MountOptions {
  data?: WhiteboardData | null
  width: number
  height: number
  offsetTop?: () => number
  onSave?: () => void
  onReady?: () => void
  onError?: (error: Error) => void
}

const DEFAULT_TOOLBAR_X = -80
const DEFAULT_TOOLBAR_Y = -73

function toolbarIcon(...paths: string[]): ReactNode {
  return createElement('span', { className: 'lapin-toolbar-icon', 'aria-hidden': 'true' },
    createElement('svg', {
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 2,
      strokeLinecap: 'round',
      strokeLinejoin: 'round',
    }, paths.map((path) => createElement('path', { key: path, d: path }))),
  )
}

function historyShortcutModifiers(): { ctrlKey: boolean; metaKey: boolean } {
  const platform = typeof navigator === 'undefined' ? '' : `${navigator.platform} ${navigator.userAgent}`
  const applePlatform = /Mac|iPhone|iPad|iPod/i.test(platform)
  return { ctrlKey: !applePlatform, metaKey: applePlatform }
}

interface ToolbarExtensionProps {
  host: HTMLElement
  undo: () => void
  redo: () => void
  clear: () => void
  save: () => void
}

function ToolbarExtension({ host, undo, redo, clear, save }: ToolbarExtensionProps) {
  const [target, setTarget] = useState<Element | null>(null)
  const offset = useRef({ x: DEFAULT_TOOLBAR_X, y: DEFAULT_TOOLBAR_Y })
  const dragCleanup = useRef<(() => void) | null>(null)

  useLayoutEffect(() => {
    const toolbar = host.querySelector('.App-top-bar .App-toolbar > .Stack_horizontal')
    setTarget(toolbar)
  }, [host])

  useLayoutEffect(() => () => dragCleanup.current?.(), [])

  useLayoutEffect(() => {
    const container = target?.closest('.App-toolbar-container') as HTMLElement | null
    if (!container) return
    container.dataset.lapinDraggableToolbar = 'true'
    container.style.transform = `translate(${offset.current.x}px, ${offset.current.y}px)`
    return () => {
      delete container.dataset.lapinDraggableToolbar
      container.style.removeProperty('transform')
    }
  }, [target])

  const startDragging = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const container = target?.closest('.App-toolbar-container') as HTMLElement | null
    if (!container) return
    event.preventDefault()
    event.stopPropagation()
    dragCleanup.current?.()
    const origin = { ...offset.current }
    const pointer = { x: event.clientX, y: event.clientY }
    const move = (current: PointerEvent) => {
      const next = {
        x: origin.x + current.clientX - pointer.x,
        y: origin.y + current.clientY - pointer.y,
      }
      offset.current = next
      container.style.transform = `translate(${next.x}px, ${next.y}px)`
    }
    const stop = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
      if (dragCleanup.current === stop) dragCleanup.current = null
    }
    dragCleanup.current = stop
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', stop, { once: true })
    window.addEventListener('pointercancel', stop, { once: true })
  }

  if (!target) return null
  const button = (key: string, label: string, icon: ReactNode, action: () => void, className = '') => createElement('button', {
    key,
    type: 'button',
    className: `ToolIcon_type_button ToolIcon_size_medium ToolIcon lapin-excalidraw-action ${className}`.trim(),
    'aria-label': label,
    title: label,
    'data-prevent-outside-click': 'true',
    onClick: action,
    onPointerDown: className.includes('drag-handle') ? startDragging : undefined,
  }, icon)

  return createPortal(createElement(Fragment, null,
    createElement('div', { key: 'divider', className: 'App-toolbar__divider' }),
    button('undo', '撤销', toolbarIcon('M9 7H5V3', 'M5 7l3.5-3.5', 'M5.5 7H13a6 6 0 1 1-5.3 8.8'), undo),
    button('redo', '重做', toolbarIcon('M15 7h4V3', 'M19 7l-3.5-3.5', 'M18.5 7H11a6 6 0 1 0 5.3 8.8'), redo),
    button('clear', '清空白板', toolbarIcon('M4 7h16', 'M9 7V4h6v3', 'M7 7l1 13h8l1-13', 'M10 11v5', 'M14 11v5'), clear),
    button('save', '保存白板', toolbarIcon('M5 12.5l4 4L19 7'), save, 'lapin-save-whiteboard'),
    button('drag', '拖动白板工具栏', toolbarIcon('M8 6h.01', 'M8 12h.01', 'M8 18h.01', 'M16 6h.01', 'M16 12h.01', 'M16 18h.01'), () => {}, 'lapin-toolbar-drag-handle'),
  ), target)
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

  const triggerHistoryShortcut = (redo: boolean) => {
    element.focus()
    element.dispatchEvent(new KeyboardEvent('keydown', {
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

  const renderTopRightUI = () => createElement(ToolbarExtension, {
    host: element,
    undo: () => triggerHistoryShortcut(false),
    redo: () => triggerHistoryShortcut(true),
    clear,
    save: () => options.onSave?.(),
  })

  if (!loadFailed) {
    root.render(createElement(Excalidraw, {
      initialData,
      autoFocus: false,
      detectScroll: true,
      handleKeyboardGlobally: false,
      zenModeEnabled: true,
      renderTopRightUI,
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
    setSaving: (saving) => {
      const saveButton = element.querySelector<HTMLButtonElement>('.lapin-save-whiteboard')
      if (!saveButton) return
      saveButton.disabled = saving
      saveButton.setAttribute('aria-busy', String(saving))
    },
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
