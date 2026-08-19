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

  // Chapters imported from PDFs are Chinese prose full of prices. The stock inline-math
  // tokenizer turns any pair of dollar signs on a line into KaTeX, so a sentence quoting two
  // amounts rendered as a serif formula with the currency signs swallowed.
  it('leaves currency alone instead of rendering it as a formula', () => {
    const markdown = '"年度总收入$7,061,089.71，季度均值$2,353,696.57"\n\n合计 $9,602,895.73, Average: $2,400,723.93\n\n方案 A 花了 $150 就省了 $50。'
    const editor = new Editor({
      extensions: createEditorExtensions(),
      content: markdown,
      contentType: 'markdown',
    })

    const kinds: string[] = []
    editor.state.doc.descendants((node) => { kinds.push(node.type.name) })
    expect(kinds).not.toContain('inlineMath')

    const text = editor.state.doc.textBetween(0, editor.state.doc.content.size, '\n', '')
    expect(text).toContain('$7,061,089.71')
    expect(text).toContain('$150')
    editor.destroy()
  })

  it('still recognises real inline formulas', () => {
    const markdown = '复杂度是 $O(n \\log n)$，损失为 $\\mathcal{L}(\\theta)$。'
    const editor = new Editor({
      extensions: createEditorExtensions(),
      content: markdown,
      contentType: 'markdown',
    })

    const latex: string[] = []
    editor.state.doc.descendants((node) => {
      if (node.type.name === 'inlineMath') latex.push(String(node.attrs.latex))
    })
    expect(latex).toEqual(['O(n \\log n)', '\\mathcal{L}(\\theta)'])
    editor.destroy()
  })
})
