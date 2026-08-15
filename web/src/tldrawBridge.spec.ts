import { describe, expect, it } from 'vitest'

import { loadWhiteboardSnapshot } from './tldrawBridge'

describe('tldraw bridge', () => {
  it('reports a persisted snapshot load failure without becoming ready', () => {
    const errors: Error[] = []

    const loaded = loadWhiteboardSnapshot(
      () => { throw new Error('invalid snapshot') },
      (error) => { errors.push(error) },
    )

    expect(loaded).toBe(false)
    expect(errors[0]?.message).toBe('invalid snapshot')
  })
})
