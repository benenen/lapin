import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DashboardView from './DashboardView.vue'

const apiMock = vi.hoisted(() => ({
  listSubjects: vi.fn(),
  getSubject: vi.fn(),
  listAnnotations: vi.fn(),
  listWhiteboards: vi.fn(),
  listComments: vi.fn(),
  createAnnotation: vi.fn(),
  updateSubject: vi.fn(),
  updateChapter: vi.fn(),
}))

vi.mock('../api', () => ({ api: apiMock }))

const user = {
  id: 'owner-id',
  email: 'owner@example.com',
  name: 'Owner',
  avatar_url: '',
  roles: ['learner'],
  created_at: '2026-08-16T00:00:00Z',
}

const subject = {
  id: 'subject-id',
  owner_id: user.id,
  owner_name: user.name,
  title: '旧科目',
  description: '旧简介',
  tags: ['Go'],
  external_id: 'openapi-subject',
  created_at: '2026-08-16T00:00:00Z',
  updated_at: '2026-08-16T00:00:00Z',
  chapters: [{
    id: 'chapter-id',
    subject_id: 'subject-id',
    title: '旧章节',
    content: '# 旧正文',
    position: 0,
    external_id: 'openapi-chapter',
    created_at: '2026-08-16T00:00:00Z',
    updated_at: '2026-08-16T00:00:00Z',
  }],
}

const annotationRecord = {
  id: 'note-1',
  chapter_id: 'chapter-id',
  user_id: user.id,
  author_name: user.name,
  start_offset: 0,
  end_offset: 3,
  quote: '正文片段',
  note: '<p>旧笔记</p>',
  color: 'yellow',
  created_at: '2026-08-17T00:00:00Z',
}

const stubs = {
  Avatar: { template: '<span />' },
  Button: {
    props: ['label', 'type', 'disabled'],
    emits: ['click'],
    template: `<button :type="type || 'button'" :disabled="disabled" @click="$emit('click')">{{ label }}</button>`,
  },
  Dialog: {
    props: ['visible', 'header'],
    emits: ['update:visible'],
    template: '<section v-if="visible" role="dialog"><h2>{{ header }}</h2><slot /></section>',
  },
  InputText: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: `<input :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
  },
  Textarea: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: `<textarea :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
  },
  Message: { template: '<div><slot /></div>' },
  Select: { template: '<select />' },
  Tag: { props: ['value'], template: '<span>{{ value }}</span>' },
  RichTextContent: { template: '<div />' },
  ExcalidrawWhiteboard: {
    name: 'ExcalidrawWhiteboard',
    props: ['chapterId', 'content', 'active', 'annotations', 'modelValue'],
    emits: ['selection', 'annotation-click', 'save'],
    template: '<div data-testid="excalidraw-whiteboard" :data-active="String(active)" :data-revision="modelValue?.anchor?.content_revision || \x27\x27" />',
  },
  RichTextEditor: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: `<textarea class="rich-editor" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
  },
}

function mountDashboard(subjectId = subject.id) {
  return mount(DashboardView, {
    props: { user, subjectId },
    global: { stubs },
  })
}

function twoChapterSubject() {
  return {
    ...subject,
    chapters: [subject.chapters[0], { ...subject.chapters[0], id: 'chapter-b', title: '第二章', position: 1 }],
  }
}

async function openChapter(wrapper: ReturnType<typeof mountDashboard>, title: string) {
  await wrapper.findAll('.chapter-tree-label').find((button) => button.text().includes(title))!.trigger('click')
  await flushPromises()
}

async function saveDraft(wrapper: ReturnType<typeof mountDashboard>, note: string) {
  await wrapper.get('.annotation-composer .rich-editor').setValue(note)
  await wrapper.findAll('.annotation-composer button').find((button) => button.text() === '保存标注')!.trigger('click')
}

describe('DashboardView ownership editing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.listSubjects.mockResolvedValue([{ ...subject, chapters: undefined }])
    apiMock.getSubject.mockResolvedValue(subject)
    apiMock.listAnnotations.mockResolvedValue([annotationRecord])
    apiMock.listWhiteboards.mockResolvedValue([])
    apiMock.listComments.mockResolvedValue([])
    apiMock.createAnnotation.mockImplementation(async (chapterId, input) => ({
      ...annotationRecord,
      ...input,
      id: 'note-2',
      chapter_id: chapterId,
    }))
    apiMock.updateSubject.mockImplementation(async (_id, input) => ({ ...subject, ...input }))
    apiMock.updateChapter.mockImplementation(async (_id, input) => ({ ...subject.chapters[0], ...input }))
  })

  it('lets the owner edit the selected subject and active chapter', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    const editSubject = wrapper.findAll('button').find((button) => button.text() === '编辑科目')
    expect(editSubject).toBeDefined()
    await editSubject!.trigger('click')

    const subjectDialog = wrapper.get('[role="dialog"]')
    expect(subjectDialog.text()).toContain('编辑科目')
    expect(subjectDialog.findAll('input')[0].element.value).toBe('旧科目')
    expect(subjectDialog.text()).toContain('再次通过 OpenAPI 导入时可能被覆盖')
    await subjectDialog.findAll('input')[0].setValue('新科目')
    await subjectDialog.find('textarea').setValue('新简介')
    await subjectDialog.get('form').trigger('submit')
    await flushPromises()

    expect(apiMock.updateSubject).toHaveBeenCalledWith('subject-id', {
      title: '新科目',
      description: '新简介',
    })
    expect(wrapper.text()).toContain('新科目')

    const editChapter = wrapper.findAll('button').find((button) => button.text() === '编辑章节')
    expect(editChapter).toBeDefined()
    await editChapter!.trigger('click')

    const chapterDialog = wrapper.get('[role="dialog"]')
    expect(chapterDialog.text()).toContain('编辑章节')
    expect(chapterDialog.find('input').element.value).toBe('旧章节')
    expect((chapterDialog.find('.rich-editor').element as HTMLTextAreaElement).value).toBe('# 旧正文')
    expect(chapterDialog.text()).toContain('正文位置变化可能使既有标注和白板提示需要重新校对')
    await chapterDialog.find('input').setValue('新章节')
    await chapterDialog.find('.rich-editor').setValue('# 新正文')
    await chapterDialog.get('form').trigger('submit')
    await flushPromises()

    expect(apiMock.updateChapter).toHaveBeenCalledWith('chapter-id', {
      title: '新章节',
      content: '# 新正文',
    })
    expect(wrapper.text()).toContain('新章节')
  })

  it('does not show edit actions to another learner', async () => {
    apiMock.getSubject.mockResolvedValue({ ...subject, owner_id: 'another-user' })
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.text()).not.toContain('编辑科目')
    expect(wrapper.text()).not.toContain('编辑章节')
  })

  it('renders parent and child chapters as a collapsible tree', async () => {
    apiMock.getSubject.mockResolvedValueOnce({
      ...subject,
      chapters: [
        subject.chapters[0],
        { ...subject.chapters[0], id: 'child-chapter', parent_id: 'chapter-id', title: '子章节', position: 1 },
      ],
    })
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.find('nav[aria-label="章节"] [role="tree"]').exists()).toBe(true)
    expect(wrapper.findAll('nav[aria-label="章节"] [role="treeitem"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('子章节')
  })

  it('loads the exact subject from a changed detail route', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    apiMock.getSubject.mockResolvedValueOnce({
      ...subject,
      id: 'next-subject',
      title: '下一门科目',
      chapters: [],
    })
    await wrapper.setProps({ subjectId: 'next-subject' })
    await flushPromises()

    expect(apiMock.getSubject).toHaveBeenLastCalledWith('next-subject')
    expect(wrapper.text()).toContain('下一门科目')
    expect(wrapper.text()).not.toContain('旧科目')
  })

  it('shows a return link when the requested subject cannot be opened', async () => {
    apiMock.getSubject.mockRejectedValueOnce(new Error('not found'))
    const wrapper = mountDashboard('missing-subject')
    await flushPromises()

    expect(wrapper.text()).toContain('无法打开这个科目')
    expect(wrapper.get('a.text-link').attributes('href')).toBe('/')
  })

  it('keeps the transparent whiteboard hidden over the chapter until requested', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-active')).toBe('false')
    expect(wrapper.find('.chapter-toolbar-quote').exists()).toBe(false)

    const showButton = wrapper.get('[data-action="whiteboard"]')
    await showButton.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-active')).toBe('true')
    expect(wrapper.get('[data-action="whiteboard"]').classes()).toContain('active')
    expect(wrapper.text()).not.toContain('我的白板')
    expect(wrapper.text()).not.toContain('白板内容仅你自己可见')

    wrapper.get('[data-testid="excalidraw-whiteboard"]').element.setAttribute('data-session-probe', 'preserved')
    await wrapper.get('[data-tab="comments"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-session-probe')).toBe('preserved')
    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-active')).toBe('true')
  })

  it('does not open an empty whiteboard before persisted data finishes loading', async () => {
    let resolveWhiteboards: (value: never[]) => void = () => {}
    apiMock.listWhiteboards.mockReturnValueOnce(new Promise<never[]>((resolve) => { resolveWhiteboards = resolve }))
    const wrapper = mountDashboard()
    await flushPromises()

    const showButton = () => wrapper.get('[data-action="whiteboard"]').element as HTMLButtonElement
    expect(showButton().disabled).toBe(true)
    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-active')).toBe('false')

    resolveWhiteboards([])
    await flushPromises()
    expect(showButton().disabled).toBe(false)
  })

  it('allows the whiteboard when an unrelated chapter interaction fails', async () => {
    apiMock.listComments.mockRejectedValueOnce(new Error('comments unavailable'))
    const wrapper = mountDashboard()
    await flushPromises()

    expect((wrapper.get('[data-action="whiteboard"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('offers a retry when persisted whiteboards fail to load', async () => {
    apiMock.listWhiteboards.mockRejectedValueOnce(new Error('whiteboards unavailable')).mockResolvedValueOnce([])
    const wrapper = mountDashboard()
    await flushPromises()

    const retry = wrapper.get('[data-action="retry-whiteboard"]')
    expect(retry.text()).toContain('重试白板')
    await retry.trigger('click')
    await flushPromises()

    expect((wrapper.get('[data-action="whiteboard"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('ignores an older response after navigating away and back to the same chapter', async () => {
    apiMock.getSubject.mockResolvedValueOnce(twoChapterSubject())
    let resolveFirstA: (value: unknown[]) => void = () => {}
    let aRequests = 0
    apiMock.listWhiteboards.mockImplementation((chapterId: string) => {
      if (chapterId === 'chapter-id') {
        aRequests++
        if (aRequests === 1) return new Promise<unknown[]>((resolve) => { resolveFirstA = resolve })
        return Promise.resolve([whiteboardRecord('new-revision')])
      }
      return Promise.resolve([])
    })
    const wrapper = mountDashboard()
    await flushPromises()

    await openChapter(wrapper, '第二章')
    await openChapter(wrapper, '旧章节')
    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-revision')).toBe('new-revision')

    resolveFirstA([whiteboardRecord('old-revision')])
    await flushPromises()
    expect(wrapper.get('[data-testid="excalidraw-whiteboard"]').attributes('data-revision')).toBe('new-revision')
  })

  it('uses rich-text editors for annotations and discussions', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.find('.annotation-composer .rich-editor').exists()).toBe(true)

    await wrapper.get('[data-tab="comments"]').trigger('click')

    expect(wrapper.find('.comment-compose .rich-editor').exists()).toBe(true)
    expect(wrapper.find('.comment-compose textarea:not(.rich-editor)').exists()).toBe(false)
  })

  it('turns a chapter selection into a composed annotation', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    await wrapper.get('.annotation-sidebar-handle').trigger('click')
    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('selection', {
      start_offset: 0,
      end_offset: 3,
      quote: '上下文',
    })
    await flushPromises()

    expect(wrapper.get('.chapter-toolbar-quote').text()).toContain('上下文')
    await wrapper.get('.chapter-toolbar button[data-color="green"]').trigger('click')
    await wrapper.get('.chapter-toolbar button[data-action="compose"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('.annotation-sidebar').classes()).not.toContain('is-collapsed')
    expect(wrapper.get('.annotation-composer blockquote').text()).toContain('上下文')
    expect(wrapper.get('.annotation-composer button[data-color="green"]').classes()).toContain('active')
  })

  it('opens the sidebar on the annotation clicked in the chapter text', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    await wrapper.get('.annotation-sidebar-handle').trigger('click')
    expect(wrapper.get('.annotation-sidebar').classes()).toContain('is-collapsed')

    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('annotation-click', 'note-1')
    await flushPromises()

    expect(wrapper.get('.annotation-sidebar').classes()).not.toContain('is-collapsed')
    expect(wrapper.get('[data-annotation-card="note-1"]').classes()).toContain('is-active')
  })

  it('opens the sidebar from the toolbar on the requested tab', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    await wrapper.get('.annotation-sidebar-handle').trigger('click')
    expect(wrapper.get('.annotation-sidebar').classes()).toContain('is-collapsed')

    await wrapper.get('.chapter-toolbar button[data-action="comments"]').trigger('click')
    expect(wrapper.get('.annotation-sidebar').classes()).not.toContain('is-collapsed')
    expect(wrapper.get('[data-tab="comments"]').classes()).toContain('active')
    expect(wrapper.find('.comment-compose').exists()).toBe(true)

    await wrapper.get('.annotation-sidebar-handle').trigger('click')
    await wrapper.get('.chapter-toolbar button[data-action="annotations"]').trigger('click')
    expect(wrapper.get('.annotation-sidebar').classes()).not.toContain('is-collapsed')
    expect(wrapper.get('[data-tab="annotations"]').classes()).toContain('active')
    expect(wrapper.find('.annotation-composer').exists()).toBe(true)
  })

  it('returns the toolbar to reading when the selection is cancelled', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('selection', {
      start_offset: 0,
      end_offset: 3,
      quote: '上下文',
    })
    await flushPromises()
    expect(wrapper.get('.chapter-toolbar-quote').text()).toContain('上下文')

    await wrapper.get('.chapter-toolbar button[data-action="cancel"]').trigger('click')

    expect(wrapper.find('.chapter-toolbar-quote').exists()).toBe(false)
    expect(wrapper.get('.chapter-toolbar').classes()).toContain('is-reading')
  })

  it('keeps the drafted note when only the selection is cancelled', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('selection', {
      start_offset: 0,
      end_offset: 3,
      quote: '上下文',
    })
    await flushPromises()
    await wrapper.get('.chapter-toolbar button[data-color="green"]').trigger('click')
    await wrapper.get('.annotation-composer .rich-editor').setValue('<p>写了一半的笔记</p>')

    await wrapper.get('.chapter-toolbar button[data-action="cancel"]').trigger('click')

    expect(wrapper.get('.chapter-toolbar').classes()).toContain('is-reading')
    expect((wrapper.get('.annotation-composer .rich-editor').element as HTMLTextAreaElement).value).toBe('<p>写了一半的笔记</p>')
    expect(wrapper.get('.annotation-composer button[data-color="green"]').classes()).toContain('active')
  })

  it('clears a pending selection and the highlight when the reader changes chapter', async () => {
    apiMock.getSubject.mockResolvedValueOnce(twoChapterSubject())
    const wrapper = mountDashboard()
    await flushPromises()

    const whiteboard = wrapper.getComponent({ name: 'ExcalidrawWhiteboard' })
    whiteboard.vm.$emit('selection', { start_offset: 0, end_offset: 3, quote: '上下文' })
    whiteboard.vm.$emit('annotation-click', 'note-1')
    await flushPromises()
    expect(wrapper.get('.chapter-toolbar-quote').text()).toContain('上下文')
    expect(wrapper.get('[data-annotation-card="note-1"]').classes()).toContain('is-active')

    await openChapter(wrapper, '第二章')

    expect(wrapper.find('.chapter-toolbar-quote').exists()).toBe(false)
    expect(wrapper.get('[data-annotation-card="note-1"]').classes()).not.toContain('is-active')
  })

  it('highlights the annotation it just saved', async () => {
    const wrapper = mountDashboard()
    await flushPromises()

    wrapper.getComponent({ name: 'ExcalidrawWhiteboard' }).vm.$emit('selection', {
      start_offset: 0,
      end_offset: 3,
      quote: '上下文',
    })
    await flushPromises()
    await saveDraft(wrapper, '<p>新笔记</p>')
    await flushPromises()

    expect(apiMock.createAnnotation).toHaveBeenCalledWith('chapter-id', {
      start_offset: 0,
      end_offset: 3,
      quote: '上下文',
      note: '<p>新笔记</p>',
      color: 'yellow',
    })
    expect(wrapper.get('[data-annotation-card="note-2"]').classes()).toContain('is-active')
    expect(wrapper.get('[data-annotation-card="note-1"]').classes()).not.toContain('is-active')
    expect(wrapper.find('.chapter-toolbar-quote').exists()).toBe(false)
  })

  it('drops an annotation that arrives after the reader left the chapter', async () => {
    apiMock.getSubject.mockResolvedValueOnce(twoChapterSubject())
    let resolveSave: (value: unknown) => void = () => {}
    apiMock.createAnnotation.mockReturnValueOnce(new Promise<unknown>((resolve) => { resolveSave = resolve }))
    const wrapper = mountDashboard()
    await flushPromises()

    await saveDraft(wrapper, '<p>新笔记</p>')
    await openChapter(wrapper, '第二章')

    resolveSave({ ...annotationRecord, id: 'note-2', chapter_id: 'chapter-id' })
    await flushPromises()

    expect(wrapper.findAll('[data-annotation-card]')).toHaveLength(1)
    expect(wrapper.find('[data-annotation-card="note-2"]').exists()).toBe(false)
    expect(wrapper.get('[data-annotation-card="note-1"]').classes()).not.toContain('is-active')
  })
})

function whiteboardRecord(contentRevision: string) {
  return {
    id: `board-${contentRevision}`,
    chapter_id: 'chapter-id',
    user_id: user.id,
    author_name: user.name,
    updated_at: '2026-08-17T00:00:00Z',
    data: {
      version: 3,
      anchor: { type: 'chapter', id: 'chapter-id', content_revision: contentRevision },
      space: { width: 960, height: 640, fit: 'contain' },
      document: { type: 'excalidraw', version: 2, elements: [], appState: {}, files: {} },
    },
  }
}
