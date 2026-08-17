import { Mathematics } from '@tiptap/extension-mathematics'
import Image from '@tiptap/extension-image'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'

interface EditorExtensionOptions {
  editInlineMath?: (latex: string, position: number) => void
  editBlockMath?: (latex: string, position: number) => void
}

const localAssetURL = /^\/api\/v1\/assets\/[A-Za-z0-9]{10,64}\/content$/

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
    Mathematics.configure({
      inlineOptions: options.editInlineMath
        ? { onClick: (node, position) => options.editInlineMath?.(node.attrs.latex as string, position) }
        : undefined,
      blockOptions: options.editBlockMath
        ? { onClick: (node, position) => options.editBlockMath?.(node.attrs.latex as string, position) }
        : undefined,
      katexOptions: { throwOnError: false },
    }),
    Markdown.configure({ markedOptions: { gfm: false } }),
  ]
}
