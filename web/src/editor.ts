import { Mathematics } from '@tiptap/extension-mathematics'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'

interface EditorExtensionOptions {
  editInlineMath?: (latex: string, position: number) => void
  editBlockMath?: (latex: string, position: number) => void
}

export function createEditorExtensions(options: EditorExtensionOptions = {}) {
  return [
    StarterKit.configure({
      heading: { levels: [2, 3] },
      link: {
        openOnClick: false,
        HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
      },
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
