export interface ColumnEdges {
  left: number
  right: number
}

// How far the chapter column may slide left so the floating annotation panel stops covering it.
// Sliding is capped by the room between the column and the chapter navigation: a column that
// slides past the navigation paints its text on top of the chapter list, which is worse than the
// overlap it was trying to avoid. When the cap bites the panel still covers part of the text —
// at that point the layout simply has no room for both, and the CSS drawer breakpoint takes over.
export function chapterColumnShift(column: ColumnEdges, navigationRight: number, panelLeft: number, gap = 16): number {
  if (![column.left, column.right, navigationRight, panelLeft].every((value) => Number.isFinite(value))) return 0
  const needed = column.right + gap - panelLeft
  const room = column.left - navigationRight - gap
  return Math.max(0, Math.min(needed, room))
}
