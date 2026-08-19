import { BlockMath, InlineMath } from '@tiptap/extension-mathematics'
import Image from '@tiptap/extension-image'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'

interface EditorExtensionOptions {
  editInlineMath?: (latex: string, position: number) => void
  editBlockMath?: (latex: string, position: number) => void
}

const localAssetURL = /^\/api\/v1\/assets\/[A-Za-z0-9]{10,64}\/content$/

// The stock inline-math tokenizer is `/^\$([^$]+)\$(?!\$)/` — any two dollar signs on a line.
// Chapters imported from PDFs are Chinese prose quoting prices, so "年度总收入$7,061,089.71，季度均值$…"
// rendered as a serif KaTeX formula with the currency signs swallowed. Across the whole imported
// course that tokenizer produced 12 matches, all of them money, and not one real formula.
//
// Require instead what a formula looks like and prose does not: no whitespace hugging either
// delimiter, no digit straight after the closing one (that is the next price), and no CJK inside.
const INLINE_MATH = /^\$([^$\s](?:[^$]*[^$\s])?)\$(?![\d$])/
const CJK = /[\u2E80-\u9FFF\uF900-\uFAFF\uFF00-\uFFEF]/

const CurrencySafeInlineMath = InlineMath.extend({
  markdownTokenizer: {
    name: 'inlineMath',
    level: 'inline',
    start: (source: string) => source.indexOf('$'),
    tokenize: (source: string) => {
      const match = source.match(INLINE_MATH)
      if (!match) return undefined
      const [raw, latex] = match
      if (CJK.test(latex)) return undefined
      return { type: 'inlineMath', raw, latex: latex.trim() }
    },
  },
})

export function isLocalAssetImageURL(value: unknown): value is string {
  return typeof value === 'string' && localAssetURL.test(value)
}

const LocalAssetImage = Image.extend({
  renderHTML({ HTMLAttributes }) {
    if (!isLocalAssetImageURL(HTMLAttributes.src)) {
      return ['span', { class: 'blocked-course-image', role: 'img', 'aria-label': '已阻止外部图片' }, '已阻止外部图片']
    }
    return ['img', { ...this.options.HTMLAttributes, ...HTMLAttributes }]
  },
})

export function createEditorExtensions(options: EditorExtensionOptions = {}) {
  return [
    StarterKit.configure({
      heading: { levels: [2, 3] },
      link: {
        openOnClick: false,
        HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
      },
    }),
    LocalAssetImage.configure({
      allowBase64: false,
      HTMLAttributes: { loading: 'lazy', decoding: 'async' },
    }),
    BlockMath.configure({
      ...(options.editBlockMath
        ? { onClick: (node, position) => options.editBlockMath?.(node.attrs.latex as string, position) }
        : {}),
      katexOptions: { throwOnError: false },
    }),
    CurrencySafeInlineMath.configure({
      ...(options.editInlineMath
        ? { onClick: (node, position) => options.editInlineMath?.(node.attrs.latex as string, position) }
        : {}),
      katexOptions: { throwOnError: false },
    }),
    Markdown.configure({ markedOptions: { gfm: false } }),
  ]
}
