import { Editor } from '@tiptap/core'
import { describe, expect, it } from 'vitest'

import { createEditorExtensions } from './editor'

describe('Tiptap Markdown storage', () => {
  it('round-trips formatting and formulas as Markdown', () => {
    const markdown = '## 定理\n\n这是 **重点**，公式为 $E = mc^2$。\n\n$$\\frac{a}{b}$$'
    const editor = new Editor({
      extensions: createEditorExtensions(),
      content: markdown,
      contentType: 'markdown',
    })
    const stored = editor.getMarkdown()
    expect(stored).toContain('## 定理')
    expect(stored).toContain('**重点**')
    expect(stored).toContain('$E = mc^2$')
    expect(stored).toContain('$$')
    expect(stored).toContain('\\frac{a}{b}')
    editor.destroy()
  })
})
