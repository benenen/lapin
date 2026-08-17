import type { LegacyTldrawWhiteboardData, WhiteboardData } from './types'

export const WHITEBOARD_WIDTH = 960
export const WHITEBOARD_MIN_HEIGHT = 640
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
  return Math.max(minimumHeight, measuredHeight + 80)
}

export function excalidrawViewport(viewportWidth: number, referenceWidth = WHITEBOARD_WIDTH, topInset = 0) {
  const zoom = viewportScale(viewportWidth, referenceWidth)
  return {
    zoom,
    scrollX: 0,
    scrollY: topInset / zoom,
  }
}
