import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export interface AnnotationOffsets {
  id: string
  start_offset: number
  end_offset: number
  color: string
  // The passage the offsets were measured against. Present on every stored annotation, so a
  // range whose text no longer matches means the chapter moved underneath it.
  quote?: string
}

export interface AnnotationRange {
  id: string
  from: number
  to: number
  color: string
}

const ANNOTATION_COLORS = new Set(['yellow', 'green', 'blue', 'pink'])

export const annotationDecorationPluginKey = new PluginKey('lapinAnnotationDecorations')

// The selection contract in RichTextContent measures offsets with textBetween(…, '', leafText),
// so the reverse mapping has to walk the document with exactly the same accounting. Both sides
// import this one function: two copies drifting apart would silently misplace every mark.
export function leafText(node: ProseMirrorNode): string {
  if (node.type.name === 'inlineMath' || node.type.name === 'blockMath') {
    return String(node.attrs.latex ?? '')
  }
  return ''
}

interface OffsetMarker {
  offset: number
  position: number
}

function offsetMarkers(doc: ProseMirrorNode): { markers: OffsetMarker[]; total: number; text: string } {
  const markers: OffsetMarker[] = []
  const chunks: string[] = []
  let offset = 0
  doc.descendants((node, position) => {
    if (node.isText) {
      markers.push({ offset, position })
      const text = node.text ?? ''
      chunks.push(text)
      offset += text.length
      return false
    }
    const leaf = leafText(node)
    if (leaf) {
      markers.push({ offset, position })
      chunks.push(leaf)
      offset += leaf.length
      return false
    }
    return true
  })
  return { markers, total: offset, text: chunks.join('') }
}

// The stored offset is a hint, not the anchor. Two clients derive it from different Markdown
// parsers that cannot be made to agree, and editing a chapter shifts it anyway, so the quote is
// what actually identifies the passage. The hint only decides which occurrence was meant.
function locateQuote(text: string, quote: string, hint: number): number | null {
  if (text.startsWith(quote, hint)) return hint
  let best: number | null = null
  for (let index = text.indexOf(quote); index !== -1; index = text.indexOf(quote, index + 1)) {
    if (best === null || Math.abs(index - hint) < Math.abs(best - hint)) best = index
  }
  return best
}

function positionAt(markers: OffsetMarker[], doc: ProseMirrorNode, offset: number): number | null {
  for (let index = markers.length - 1; index >= 0; index--) {
    const marker = markers[index]!
    if (marker.offset > offset) continue
    const node = doc.nodeAt(marker.position)
    if (!node) return null
    const length = node.isText ? node.text?.length ?? 0 : leafText(node).length
    const inner = offset - marker.offset
    if (inner > length) return null
    return marker.position + (node.isText ? inner : inner > 0 ? node.nodeSize : 0)
  }
  return null
}

export function annotationDecorationRanges(doc: ProseMirrorNode, annotations: readonly AnnotationOffsets[]): AnnotationRange[] {
  const { markers, total, text } = offsetMarkers(doc)
  const ranges: AnnotationRange[] = []
  for (const annotation of annotations) {
    const hint = annotation.start_offset
    if (!Number.isFinite(hint) || !Number.isFinite(annotation.end_offset)) continue
    let start = hint
    let end = annotation.end_offset
    if (annotation.quote) {
      const located = locateQuote(text, annotation.quote, Math.max(0, hint))
      // The passage itself is gone — an edited chapter, or a repeated OpenAPI import, which keeps
      // interaction records while the content changes. There is nothing left to point at.
      if (located === null) continue
      start = located
      end = located + annotation.quote.length
    }
    if (end <= start || start < 0 || end > total) continue
    const from = positionAt(markers, doc, start)
    const to = positionAt(markers, doc, end)
    if (from === null || to === null || to <= from) continue
    ranges.push({
      id: annotation.id,
      from,
      to,
      color: ANNOTATION_COLORS.has(annotation.color) ? annotation.color : 'yellow',
    })
  }
  return ranges
}

export function annotationDecorationPlugin(getAnnotations: () => readonly AnnotationOffsets[]): Plugin {
  const build = (doc: ProseMirrorNode) => DecorationSet.create(doc, annotationDecorationRanges(doc, getAnnotations()).map((range) => (
    Decoration.inline(range.from, range.to, {
      class: `annotation-mark annotation-mark-${range.color}`,
      'data-annotation-id': range.id,
    })
  )))
  return new Plugin({
    key: annotationDecorationPluginKey,
    state: {
      init: (_config, state) => build(state.doc),
      apply: (transaction, value, _oldState, newState) => (
        transaction.docChanged || transaction.getMeta(annotationDecorationPluginKey) ? build(newState.doc) : value
      ),
    },
    props: {
      decorations(state) {
        return this.getState(state)
      },
    },
  })
}
