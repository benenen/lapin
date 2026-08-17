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

  it('renders semantic paragraphs and same-origin asset images', () => {
    const markdown = '## 章节标题\n\n第一段跨行前半，后半仍属于第一段。\n\n第二段。\n\n![图 1-1](/api/v1/assets/abcdefghij/content "示意图")'
    const editor = new Editor({
      extensions: createEditorExtensions(),
      content: markdown,
      contentType: 'markdown',
    })

    const html = editor.getHTML()
    expect(html).toContain('<h2>章节标题</h2>')
    expect(html.match(/<p>/g)).toHaveLength(2)
    expect(html).toContain('第一段跨行前半，后半仍属于第一段。')
    expect(html).toContain('<img')
    expect(html).toContain('src="/api/v1/assets/abcdefghij/content"')
    expect(html).toContain('alt="图 1-1"')

    const stored = editor.getMarkdown()
    expect(stored).toContain('![图 1-1](/api/v1/assets/abcdefghij/content')
    editor.destroy()
  })

  it('does not render remote or active-content image URLs', () => {
    const editor = new Editor({
      extensions: createEditorExtensions(),
      content: '![tracker](https://tracker.example/pixel.png)\n\n![inline](data:image/png;base64,AAAA)',
      contentType: 'markdown',
    })

    const html = editor.getHTML()
    expect(html).not.toContain('<img')
    expect(html).not.toContain('tracker.example')
    expect(html).not.toContain('data:image')
    expect(html.match(/已阻止外部图片/g)?.length).toBeGreaterThanOrEqual(2)
    editor.destroy()
  })
})
