import { describe, expect, it } from 'vitest'

import { chapterContentRevision, isCompatibleWhiteboard, viewportScale } from './whiteboard'

describe('anchored whiteboard contract', () => {
  it('keeps one canonical coordinate space while the viewport resizes', () => {
    expect(viewportScale(960, 960)).toBe(1)
    expect(viewportScale(480, 960)).toBe(0.5)
    expect(viewportScale(1440, 960)).toBe(1)
  })

  it('only restores documents anchored to the current chapter', () => {
    const document = {
      version: 2 as const,
      anchor: { type: 'chapter' as const, id: 'chapter-a', content_revision: 'sha256:test' },
      space: { width: 960, height: 640, fit: 'contain' as const },
      document: { store: {}, schema: { schemaVersion: 2 } },
    }
    expect(isCompatibleWhiteboard(document, 'chapter-a')).toBe(true)
    expect(isCompatibleWhiteboard(document, 'chapter-b')).toBe(false)
  })

  it('produces a stable content revision', async () => {
    await expect(chapterContentRevision('abc')).resolves.toBe('sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad')
  })
})
