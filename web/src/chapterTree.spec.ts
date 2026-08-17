import { describe, expect, it } from 'vitest'

import type { Chapter } from './types'
import { buildChapterTree } from './chapterTree'

function chapter(id: string, title: string, position: number, parentId?: string): Chapter {
  return {
    id,
    parent_id: parentId,
    title,
    content: '',
    position,
    external_id: '',
    created_at: '2026-08-16T00:00:00Z',
  }
}

describe('chapter tree', () => {
  it('builds nested nodes instead of flattening child chapters', () => {
    const chapters = [
      chapter('child-b', '子章节 B', 2, 'root'),
      chapter('root', '根章节', 0),
      chapter('child-a', '子章节 A', 1, 'root'),
      chapter('orphan', '失去父级的章节', 3, 'missing'),
    ]

    const tree = buildChapterTree(chapters)

    expect(tree.map((node) => node.chapter.id)).toEqual(['root', 'orphan'])
    expect(tree[0]?.children.map((node) => node.chapter.id)).toEqual(['child-a', 'child-b'])
    expect(chapters.map((item) => item.id)).toEqual(['child-b', 'root', 'child-a', 'orphan'])
  })
})
