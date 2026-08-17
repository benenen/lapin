import type { Chapter } from './types'

export interface ChapterTreeNode {
  chapter: Chapter
  children: ChapterTreeNode[]
}

export function buildChapterTree(chapters: readonly Chapter[]): ChapterTreeNode[] {
  const knownIDs = new Set(chapters.map((chapter) => chapter.id))
  const ordered = chapters
    .map((chapter, index) => ({ chapter, index }))
    .sort((left, right) => left.chapter.position - right.chapter.position || left.index - right.index)
    .map(({ chapter }) => chapter)

  const buildNode = (chapter: Chapter, ancestors: ReadonlySet<string>): ChapterTreeNode => {
    const nextAncestors = new Set([...ancestors, chapter.id])
    const children = ordered
      .filter((candidate) => candidate.parent_id === chapter.id && !nextAncestors.has(candidate.id))
      .map((candidate) => buildNode(candidate, nextAncestors))
    return { chapter, children }
  }

  return ordered
    .filter((chapter) => !chapter.parent_id || !knownIDs.has(chapter.parent_id))
    .map((chapter) => buildNode(chapter, new Set()))
}
