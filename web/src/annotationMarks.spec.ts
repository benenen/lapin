import { describe, expect, it } from 'vitest'
import { Editor } from '@tiptap/core'

import { createEditorExtensions } from './editor'
import { annotationDecorationRanges } from './annotationMarks'

function docOf(markdown: string) {
  const editor = new Editor({
    content: markdown,
    contentType: 'markdown',
    editable: false,
    extensions: createEditorExtensions(),
  })
  const doc = editor.state.doc
  editor.destroy()
  return doc
}

function annotation(id: string, start: number, end: number, color = 'yellow', quote?: string) {
  return { id, start_offset: start, end_offset: end, color, quote }
}

function quoted(doc: ReturnType<typeof docOf>, range: { from: number; to: number }) {
  return doc.textBetween(range.from, range.to, '', () => '')
}

describe('annotation decoration ranges', () => {
  it('maps rendered text offsets onto document positions', () => {
    const doc = docOf('上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 5)])

    expect(ranges).toHaveLength(1)
    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('上下文工程')
    expect(ranges[0]!.color).toBe('yellow')
  })

  it('keeps offsets aligned across block boundaries', () => {
    const doc = docOf('第一段。\n\n第二段文字。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 4, 7)])

    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('第二段')
  })

  it('counts math nodes by their latex, matching the selection contract', () => {
    const doc = docOf('设 $a+b$ 成立。')

    // "设 " (offsets 0-2) + inlineMath latex "a+b" (offsets 2-5, length 3) must both be
    // counted for offset 5 to land exactly after the math node. If the math node's latex
    // length were not counted, this range would spill into the following text node instead.
    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 5)])

    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('设 ')
  })

  it('skips annotations that no longer fit the chapter text', () => {
    const doc = docOf('很短。')

    expect(annotationDecorationRanges(doc, [annotation('a', 0, 999)])).toEqual([])
    expect(annotationDecorationRanges(doc, [annotation('a', 900, 950)])).toEqual([])
  })

  // An owner edit or a repeated OpenAPI import keeps the annotation but moves the chapter text.
  // The offsets stay inside the document while pointing at the wrong words, so the quote — not
  // the offsets — decides where the mark goes.
  it('re-anchors on the quote rather than painting the words the offsets now cover', () => {
    const doc = docOf('前言。上下文工程是核心。')

    const stale = annotationDecorationRanges(doc, [{ ...annotation('a', 0, 5), quote: '上下文工程' }])

    expect(stale).toHaveLength(1)
    // Offsets 0..5 now cover '前言。上下'; the annotation quoted '上下文工程' and lands there.
    expect(doc.textBetween(1, 6, '', () => '')).toBe('前言。上下')
    expect(quoted(doc, stale[0]!)).toBe('上下文工程')
  })

  it('renders an annotation whose quote still matches the offsets', () => {
    const doc = docOf('前言。上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [{ ...annotation('a', 3, 8), quote: '上下文工程' }])

    expect(ranges).toHaveLength(1)
    expect(doc.textBetween(ranges[0]!.from, ranges[0]!.to, '', () => '')).toBe('上下文工程')
  })

  it('still renders an annotation stored without a quote', () => {
    const doc = docOf('上下文工程是核心。')

    expect(annotationDecorationRanges(doc, [annotation('a', 0, 5)])).toHaveLength(1)
    expect(annotationDecorationRanges(doc, [{ ...annotation('b', 0, 5), quote: '' }])).toHaveLength(1)
  })

  it('skips empty and reversed ranges', () => {
    const doc = docOf('上下文工程是核心。')

    expect(annotationDecorationRanges(doc, [annotation('a', 3, 3)])).toEqual([])
    expect(annotationDecorationRanges(doc, [annotation('a', 5, 2)])).toEqual([])
  })

  it('keeps overlapping annotations as separate ranges', () => {
    const doc = docOf('上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 5), annotation('b', 3, 8, 'blue')])

    expect(ranges.map((range) => range.id)).toEqual(['a', 'b'])
    expect(ranges[1]!.color).toBe('blue')
  })

  it('falls back to yellow for an unknown colour', () => {
    const doc = docOf('上下文工程是核心。')

    expect(annotationDecorationRanges(doc, [annotation('a', 0, 3, 'orange')])[0]!.color).toBe('yellow')
  })

  // 偏移锚定换成引用锚定：the stored offsets are only a hint, because the web and a future
  // Flutter client derive them from different Markdown parsers and cannot be made to agree.
  it('follows the quote when the stored offset has drifted', () => {
    // The annotation was made against '上下文工程是核心。'; someone later prepended a sentence,
    // so offsets 0..5 now cover '前言。上下' instead.
    const doc = docOf('前言。上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 0, 5, 'yellow', '上下文工程')])

    expect(ranges).toHaveLength(1)
    expect(quoted(doc, ranges[0]!)).toBe('上下文工程')
  })

  it('picks the occurrence nearest the stored offset when the quote repeats', () => {
    const doc = docOf('上下文工程很重要。再谈上下文工程的细节。最后回到上下文工程。')

    const near = annotationDecorationRanges(doc, [annotation('a', 24, 29, 'yellow', '上下文工程')])[0]!
    const far = annotationDecorationRanges(doc, [annotation('b', 0, 5, 'yellow', '上下文工程')])[0]!

    expect(quoted(doc, near)).toBe('上下文工程')
    expect(quoted(doc, far)).toBe('上下文工程')
    // The third occurrence starts at 24, the first at 0 — each annotation lands on its own.
    expect(near.from).toBeGreaterThan(far.from)
  })

  it('keeps the stored offset when it still matches, even if the quote repeats', () => {
    const doc = docOf('上下文工程很重要。再谈上下文工程的细节。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 11, 16, 'yellow', '上下文工程')])

    expect(quoted(doc, ranges[0]!)).toBe('上下文工程')
    expect(ranges[0]!.from).toBeGreaterThan(10)
  })

  it('drops an annotation whose quote is gone from the chapter', () => {
    const doc = docOf('这一段已经被完全改写了。')

    expect(annotationDecorationRanges(doc, [annotation('a', 0, 5, 'yellow', '上下文工程')])).toEqual([])
  })

  it('anchors a quote that the stored offsets no longer even reach', () => {
    // The chapter shrank: the offsets point past the end, but the passage is still there.
    const doc = docOf('上下文工程是核心。')

    const ranges = annotationDecorationRanges(doc, [annotation('a', 900, 905, 'yellow', '上下文工程')])

    expect(quoted(doc, ranges[0]!)).toBe('上下文工程')
  })
})
