import type { LegacyTldrawWhiteboardData, WhiteboardData } from './types'

export const WHITEBOARD_WIDTH = 960
export const WHITEBOARD_MIN_HEIGHT = 640
// Browsers cap a canvas at 65535 device pixels per side and every repaint costs the
// whole surface, so the drawable overlay only follows the reader through a long
// chapter instead of covering it end to end.
export const WHITEBOARD_WINDOW_VIEWPORTS = 3
export const WHITEBOARD_MAX_WINDOW_HEIGHT = 12000
// Mirrors maxWhiteboardHeight in internal/httpapi/handler/interactions.go. An image-heavy
// chapter can measure taller than that, and the server would reject the whole save with a
// generic error, so clamp here instead of losing the session's ink at the API boundary.
export const WHITEBOARD_MAX_HEIGHT = 200_000
const SUPPORTED_EXCALIDRAW_ELEMENTS = new Set(['rectangle', 'diamond', 'ellipse', 'arrow', 'line', 'freedraw', 'text'])

export function isSupportedExcalidrawElement(type: string): boolean {
  return SUPPORTED_EXCALIDRAW_ELEMENTS.has(type)
}

export async function chapterContentRevision(content: string): Promise<string> {
  const bytes = new TextEncoder().encode(content)
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return `sha256:${Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, '0')).join('')}`
}

export function isCompatibleWhiteboard(data: unknown, chapterID: string): data is WhiteboardData {
  if (!isRecord(data) || data.version !== 3 || !isRecord(data.anchor) || !isRecord(data.space) || !isRecord(data.document)) {
    return false
  }
  return data.anchor.type === 'chapter'
    && data.anchor.id === chapterID
    && typeof data.anchor.content_revision === 'string'
    && data.space.fit === 'contain'
    && typeof data.space.width === 'number'
    && data.space.width >= 100
    && typeof data.space.height === 'number'
    && data.space.height >= 100
    && data.document.type === 'excalidraw'
    && data.document.version === 2
    && Array.isArray(data.document.elements)
    && isRecord(data.document.appState)
    && isRecord(data.document.files)
}

export function isLegacyTldrawWhiteboard(data: unknown): data is LegacyTldrawWhiteboardData {
  return isRecord(data)
    && data.version === 2
    && isRecord(data.anchor)
    && data.anchor.type === 'chapter'
    && typeof data.anchor.id === 'string'
    && isRecord(data.space)
    && data.space.fit === 'contain'
    && isRecord(data.document)
    && isRecord(data.document.store)
    && isRecord(data.document.schema)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function viewportScale(viewportWidth: number, referenceWidth = WHITEBOARD_WIDTH): number {
  if (!Number.isFinite(viewportWidth) || viewportWidth <= 0 || !Number.isFinite(referenceWidth) || referenceWidth <= 0) {
    return 1
  }
  return Math.min(1, viewportWidth / referenceWidth)
}

export function whiteboardReferenceHeight(contentHeight: number, minimumHeight = WHITEBOARD_MIN_HEIGHT): number {
  const measuredHeight = Number.isFinite(contentHeight) && contentHeight > 0 ? contentHeight : 0
  return Math.min(WHITEBOARD_MAX_HEIGHT, Math.max(minimumHeight, measuredHeight + 80))
}

export function excalidrawViewport(viewportWidth: number, referenceWidth = WHITEBOARD_WIDTH, offsetTop = 0) {
  const zoom = viewportScale(viewportWidth, referenceWidth)
  const offset = Number.isFinite(offsetTop) ? offsetTop : 0
  return {
    zoom,
    scrollX: 0,
    scrollY: offset === 0 ? 0 : -offset / zoom,
  }
}

export interface WhiteboardWindow {
  top: number
  height: number
}

// All arguments and the result are reference-space lengths, never CSS pixels.
export function whiteboardWindow(stageHeight: number, visibleTop: number, visibleHeight: number, currentTop: number): WhiteboardWindow {
  const stage = Number.isFinite(stageHeight) && stageHeight > 0 ? stageHeight : 0
  const visible = Number.isFinite(visibleHeight) && visibleHeight > 0 ? visibleHeight : 0
  const height = Math.min(stage, Math.min(WHITEBOARD_MAX_WINDOW_HEIGHT, Math.max(WHITEBOARD_MIN_HEIGHT, visible * WHITEBOARD_WINDOW_VIEWPORTS)))
  const maxTop = stage - height
  if (maxTop <= 0) return { top: 0, height }
  const anchor = Number.isFinite(visibleTop) ? visibleTop : 0
  const margin = visible / 2
  const held = clampWindowTop(Number.isFinite(currentTop) ? currentTop : 0, maxTop)
  if (held <= anchor - margin && held + height >= anchor + visible + margin) return { top: held, height }
  return { top: clampWindowTop(anchor + visible / 2 - height / 2, maxTop), height }
}

function clampWindowTop(top: number, maxTop: number): number {
  return Math.min(Math.max(top, 0), maxTop)
}
