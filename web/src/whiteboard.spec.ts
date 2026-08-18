import { describe, expect, it } from 'vitest'

import { chapterContentRevision, excalidrawViewport, isCompatibleWhiteboard, isLegacyTldrawWhiteboard, isSupportedExcalidrawElement, viewportScale, WHITEBOARD_MAX_HEIGHT, WHITEBOARD_MAX_WINDOW_HEIGHT, whiteboardReferenceHeight, whiteboardWindow } from './whiteboard'

describe('anchored whiteboard contract', () => {
  it('keeps one canonical coordinate space while the viewport resizes', () => {
    expect(viewportScale(960, 960)).toBe(1)
    expect(viewportScale(480, 960)).toBe(0.5)
    expect(viewportScale(1440, 960)).toBe(1)
  })

  it('shifts scene coordinates by the drawable window offset', () => {
    expect(excalidrawViewport(480, 960, 300)).toEqual({
      zoom: 0.5,
      scrollX: 0,
      scrollY: -600,
    })
    expect(excalidrawViewport(1440, 960, 300)).toEqual({
      zoom: 1,
      scrollX: 0,
      scrollY: -300,
    })
  })

  it('keeps the drawable window small enough for a browser canvas on long chapters', () => {
    const window = whiteboardWindow(73600, 0, 900, 0)

    expect(window.height).toBe(2700)
    expect(window.height).toBeLessThanOrEqual(WHITEBOARD_MAX_WINDOW_HEIGHT)
    expect(window.top).toBe(0)
  })

  it('covers the whole stage when the chapter fits in one window', () => {
    expect(whiteboardWindow(2000, 0, 900, 0)).toEqual({ top: 0, height: 2000 })
  })

  it('holds the drawable window still while the reader stays inside its margin', () => {
    expect(whiteboardWindow(73600, 4000, 900, 3000)).toEqual({ top: 3000, height: 2700 })
  })

  it('recenters the drawable window before the reader reaches its edge', () => {
    expect(whiteboardWindow(73600, 5200, 900, 3000)).toEqual({ top: 4300, height: 2700 })
  })

  it('clamps the drawable window to the end of the chapter', () => {
    expect(whiteboardWindow(10000, 9000, 900, 0)).toEqual({ top: 7300, height: 2700 })
  })

  it('keeps the canvas tall enough for reflowed chapter content', () => {
    expect(whiteboardReferenceHeight(900, 640)).toBe(980)
    expect(whiteboardReferenceHeight(100, 1200)).toBe(1200)
    expect(whiteboardReferenceHeight(Number.NaN, 640)).toBe(640)
  })

  // The server rejects a space taller than this outright, so an image-heavy chapter has to be
  // clamped here rather than losing the whole board to 白板数据无效或过大 on save.
  it('never asks for a space taller than the server accepts', () => {
    expect(WHITEBOARD_MAX_HEIGHT).toBe(200_000)
    expect(whiteboardReferenceHeight(400_000, 640)).toBe(WHITEBOARD_MAX_HEIGHT)
    expect(whiteboardReferenceHeight(0, 400_000)).toBe(WHITEBOARD_MAX_HEIGHT)
    expect(whiteboardReferenceHeight(WHITEBOARD_MAX_HEIGHT - 80, 640)).toBe(WHITEBOARD_MAX_HEIGHT)
  })

  it('places the transparent scene directly on the chapter by default', () => {
    expect(excalidrawViewport(960, 960)).toEqual({
      zoom: 1,
      scrollX: 0,
      scrollY: 0,
    })
  })

  it('only restores documents anchored to the current chapter', () => {
    const document = {
      version: 3 as const,
      anchor: { type: 'chapter' as const, id: 'chapter-a', content_revision: 'sha256:test' },
      space: { width: 960, height: 640, fit: 'contain' as const },
      document: { type: 'excalidraw' as const, version: 2, elements: [], appState: { viewBackgroundColor: 'transparent' }, files: {} },
    }
    expect(isCompatibleWhiteboard(document, 'chapter-a')).toBe(true)
    expect(isCompatibleWhiteboard(document, 'chapter-b')).toBe(false)
  })

  it('recognizes legacy tldraw documents without loading them as Excalidraw', () => {
    const legacy = {
      version: 2,
      anchor: { type: 'chapter', id: 'chapter-a', content_revision: 'sha256:test' },
      space: { width: 960, height: 640, fit: 'contain' },
      document: { store: {}, schema: { schemaVersion: 2 } },
    }
    expect(isLegacyTldrawWhiteboard(legacy)).toBe(true)
    expect(isCompatibleWhiteboard(legacy, 'chapter-a')).toBe(false)
  })

  it('persists every shape exposed in the primary toolbar but not embedded content', () => {
    for (const type of ['rectangle', 'diamond', 'ellipse', 'arrow', 'line', 'freedraw', 'text']) {
      expect(isSupportedExcalidrawElement(type)).toBe(true)
    }
    expect(isSupportedExcalidrawElement('image')).toBe(false)
    expect(isSupportedExcalidrawElement('embeddable')).toBe(false)
  })

  it('produces a stable content revision', async () => {
    await expect(chapterContentRevision('abc')).resolves.toBe('sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  })
})
