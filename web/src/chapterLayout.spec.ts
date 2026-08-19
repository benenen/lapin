import { describe, expect, it } from 'vitest'

import { chapterColumnShift } from './chapterLayout'

describe('chapter column shift', () => {
  it('does not move a column the panel already clears', () => {
    // 1440px: column 464..1200, navigation ends at 224, panel starts at 1216.
    expect(chapterColumnShift({ left: 464, right: 1200 }, 224, 1216)).toBe(0)
  })

  it('slides exactly far enough to clear the panel when there is room', () => {
    // 1440px with the panel at 1120: 96px of overlap, 224px of room to the navigation.
    expect(chapterColumnShift({ left: 464, right: 1200 }, 224, 1120)).toBe(96)
  })

  it('never slides the column over the chapter navigation', () => {
    // 1200px: clearing the panel would need 216px but only 104px is free before the navigation.
    expect(chapterColumnShift({ left: 344, right: 1080 }, 224, 880)).toBe(104)
  })

  it('stays put when the navigation leaves no room at all', () => {
    expect(chapterColumnShift({ left: 240, right: 976 }, 224, 800)).toBe(0)
    expect(chapterColumnShift({ left: 200, right: 936 }, 224, 800)).toBe(0)
  })

  it('ignores a measurement it cannot trust', () => {
    expect(chapterColumnShift({ left: Number.NaN, right: 1080 }, 224, 880)).toBe(0)
    expect(chapterColumnShift({ left: 344, right: 1080 }, 224, Number.NaN)).toBe(0)
  })

  it('keeps a breathing gap on both sides', () => {
    // Same geometry, a wider gap costs one gap on the need side and one on the room side.
    expect(chapterColumnShift({ left: 464, right: 1200 }, 224, 1120, 40)).toBe(120)
    expect(chapterColumnShift({ left: 344, right: 1080 }, 224, 880, 40)).toBe(80)
  })
})
