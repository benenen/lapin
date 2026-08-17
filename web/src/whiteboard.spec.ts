import { describe, expect, it } from 'vitest'

import { chapterContentRevision, excalidrawViewport, isCompatibleWhiteboard, isLegacyTldrawWhiteboard, isSupportedExcalidrawElement, viewportScale, whiteboardReferenceHeight } from './whiteboard'

describe('anchored whiteboard contract', () => {
  it('keeps one canonical coordinate space while the viewport resizes', () => {
    expect(viewportScale(960, 960)).toBe(1)
    expect(viewportScale(480, 960)).toBe(0.5)
    expect(viewportScale(1440, 960)).toBe(1)
  })

  it('keeps scene coordinates below the embedded Excalidraw toolbar', () => {
    expect(excalidrawViewport(480, 960, 64)).toEqual({
      zoom: 0.5,
      scrollX: 0,
      scrollY: 128,
    })
    expect(excalidrawViewport(1440, 960, 64)).toEqual({
      zoom: 1,
      scrollX: 0,
      scrollY: 64,
    })
  })

  it('keeps the canvas tall enough for reflowed chapter content', () => {
    expect(whiteboardReferenceHeight(900, 640)).toBe(980)
    expect(whiteboardReferenceHeight(100, 1200)).toBe(1200)
    expect(whiteboardReferenceHeight(Number.NaN, 640)).toBe(640)
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
