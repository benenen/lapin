import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DashboardView from './DashboardView.vue'

const apiMock = vi.hoisted(() => ({
  listSubjects: vi.fn(),
  getSubject: vi.fn(),
  listAnnotations: vi.fn(),
  listWhiteboards: vi.fn(),
  listComments: vi.fn(),
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
  ExcalidrawWhiteboard: { template: '<div data-testid="excalidraw-whiteboard" />' },
  RichTextEditor: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: `<textarea class="rich-editor" :value="modelValue" @input="$emit('update:modelValue', $event.target.value)" />`,
  },
}

describe('DashboardView ownership editing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.listSubjects.mockResolvedValue([{ ...subject, chapters: undefined }])
    apiMock.getSubject.mockResolvedValue(subject)
    apiMock.listAnnotations.mockResolvedValue([])
    apiMock.listWhiteboards.mockResolvedValue([])
    apiMock.listComments.mockResolvedValue([])
    apiMock.updateSubject.mockImplementation(async (_id, input) => ({ ...subject, ...input }))
    apiMock.updateChapter.mockImplementation(async (_id, input) => ({ ...subject.chapters[0], ...input }))
  })

  it('lets the owner edit the selected subject and active chapter', async () => {
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: subject.id },
      global: { stubs },
    })
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
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: subject.id },
      global: { stubs },
    })
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
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: subject.id },
      global: { stubs },
    })
    await flushPromises()

    expect(wrapper.find('nav[aria-label="章节"] [role="tree"]').exists()).toBe(true)
    expect(wrapper.findAll('nav[aria-label="章节"] [role="treeitem"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('子章节')
  })

  it('loads the exact subject from a changed detail route', async () => {
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: subject.id },
      global: { stubs },
    })
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
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: 'missing-subject' },
      global: { stubs },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('无法打开这个科目')
    expect(wrapper.get('a.text-link').attributes('href')).toBe('/')
  })

  it('keeps the transparent whiteboard hidden over the chapter until requested', async () => {
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: subject.id },
      global: { stubs },
    })
    await flushPromises()

    const tabLabels = wrapper.findAll('[role="tablist"] button').map((button) => button.text())
    expect(tabLabels).not.toContain('白板')
    expect(wrapper.find('[data-testid="excalidraw-whiteboard"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('选中')

    const showButton = wrapper.findAll('button').find((button) => button.text() === '显示白板')
    expect(showButton).toBeDefined()
    await showButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="excalidraw-whiteboard"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('隐藏白板')
    expect(wrapper.text()).not.toContain('我的白板')
    expect(wrapper.text()).not.toContain('白板内容仅你自己可见')
  })

  it('uses rich-text editors for annotations and discussions', async () => {
    const wrapper = mount(DashboardView, {
      props: { user, subjectId: subject.id },
      global: { stubs },
    })
    await flushPromises()

    expect(wrapper.find('.annotation-panel .rich-editor').exists()).toBe(true)

    const commentsTab = wrapper.findAll('[role="tablist"] button').find((button) => button.text().includes('讨论'))
    expect(commentsTab).toBeDefined()
    await commentsTab!.trigger('click')

    expect(wrapper.find('.comment-compose .rich-editor').exists()).toBe(true)
    expect(wrapper.find('.comment-compose textarea:not(.rich-editor)').exists()).toBe(false)
  })
})
