import type { WhiteboardData } from './types'

export const WHITEBOARD_WIDTH = 960
export const WHITEBOARD_MIN_HEIGHT = 640

export async function chapterContentRevision(content: string): Promise<string> {
  const bytes = new TextEncoder().encode(content)
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return `sha256:${Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, '0')).join('')}`
}

export function isCompatibleWhiteboard(data: WhiteboardData | null | undefined, chapterID: string): data is WhiteboardData {
  return data?.version === 2
    && data.anchor.type === 'chapter'
    && data.anchor.id === chapterID
    && data.space.fit === 'contain'
    && data.space.width >= 100
    && data.space.height >= 100
    && typeof data.document === 'object'
    && data.document !== null
    && typeof data.document.store === 'object'
    && data.document.store !== null
    && typeof data.document.schema === 'object'
    && data.document.schema !== null
}

export function viewportScale(viewportWidth: number, referenceWidth = WHITEBOARD_WIDTH): number {
  if (!Number.isFinite(viewportWidth) || viewportWidth <= 0 || !Number.isFinite(referenceWidth) || referenceWidth <= 0) {
    return 1
  }
  return Math.min(1, viewportWidth / referenceWidth)
}
