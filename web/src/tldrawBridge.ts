import {
  getSnapshot,
  loadSnapshot,
  TldrawEditor,
  type Editor,
  type TLEditorComponents,
  type TLStoreSnapshot,
} from '@tldraw/editor'
import { createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { DrawShapeTool, DrawShapeUtil, EraserTool, SelectTool } from 'tldraw'

import type { WhiteboardData } from './types'

export type WhiteboardTool = 'select' | 'draw' | 'eraser'

export interface TldrawBridge {
  destroy: () => void
  isReady: () => boolean
  resize: () => void
  setTool: (tool: WhiteboardTool) => void
  undo: () => void
  redo: () => void
  clear: () => void
  getDocument: () => WhiteboardData['document']
}

interface MountOptions {
  data?: WhiteboardData | null
  width: number
  height: number
  onReady?: () => void
  onError?: (error: Error) => void
}

const transparentComponents: TLEditorComponents = {
  Background: null,
  Grid: null,
}

export function mountTldraw(element: HTMLElement, options: MountOptions): TldrawBridge {
  const root = createRoot(element)
  let editor: Editor | null = null
  let loadFailed = false

  const onMount = (mountedEditor: Editor) => {
    editor = mountedEditor
    mountedEditor.setCameraOptions({
      isLocked: true,
      wheelBehavior: 'none',
      constraints: {
        bounds: { x: 0, y: 0, w: options.width, h: options.height },
        padding: { x: 0, y: 0 },
        origin: { x: 0, y: 0 },
        initialZoom: 'fit-x-100',
        baseZoom: 'fit-x-100',
        behavior: 'fixed',
      },
    })
    const data = options.data
    if (data) {
      const loaded = loadWhiteboardSnapshot(() => {
        loadSnapshot(mountedEditor.store, { document: data.document as unknown as TLStoreSnapshot })
      }, options.onError)
      if (!loaded) {
        loadFailed = true
        return
      }
    }
    resetCamera(mountedEditor)
    mountedEditor.setCurrentTool('draw')
    options.onReady?.()
  }

  root.render(createElement(TldrawEditor, {
    components: transparentComponents,
    shapeUtils: [DrawShapeUtil],
    tools: [SelectTool, DrawShapeTool, EraserTool],
    initialState: 'draw',
    autoFocus: false,
    onMount,
  }))

  function withEditor(action: (current: Editor) => void) {
    if (editor) action(editor)
  }

  function resetCamera(current: Editor) {
    current.updateViewportScreenBounds(element)
    const viewportWidth = element.getBoundingClientRect().width
    const zoom = Math.min(1, viewportWidth / options.width)
    current.setCamera({ x: 0, y: 0, z: zoom }, { force: true, immediate: true })
  }

  return {
    destroy: () => {
      editor = null
      root.unmount()
    },
    isReady: () => editor !== null && !loadFailed,
    resize: () => withEditor(resetCamera),
    setTool: (tool) => withEditor((current) => current.setCurrentTool(tool)),
    undo: () => withEditor((current) => current.undo()),
    redo: () => withEditor((current) => current.redo()),
    clear: () => withEditor((current) => current.deleteShapes([...current.getCurrentPageShapeIds()])),
    getDocument: () => {
      if (!editor) throw new Error('白板仍在加载')
      const snapshot = getSnapshot(editor.store).document
      return JSON.parse(JSON.stringify(snapshot)) as WhiteboardData['document']
    },
  }
}

export function loadWhiteboardSnapshot(load: () => void, onError?: (error: Error) => void): boolean {
  try {
    load()
    return true
  } catch (caught) {
    onError?.(caught instanceof Error ? caught : new Error('白板快照无法读取'))
    return false
  }
}
