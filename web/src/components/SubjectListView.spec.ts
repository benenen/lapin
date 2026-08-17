import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SubjectListView from './SubjectListView.vue'

const apiMock = vi.hoisted(() => ({
  listSubjects: vi.fn(),
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

describe('SubjectListView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.listSubjects.mockResolvedValue([{
      id: 'subject-hash',
      owner_id: user.id,
      owner_name: user.name,
      title: 'Go 并发',
      description: '从 goroutine 开始',
      tags: ['Go'],
      created_at: '2026-08-16T00:00:00Z',
      updated_at: '2026-08-16T00:00:00Z',
    }])
  })

  it('shows course links that open standalone detail pages in a new tab', async () => {
    const wrapper = mount(SubjectListView, {
      props: { user },
      global: {
        stubs: {
          Avatar: { template: '<span />' },
          Button: { props: ['label'], template: '<button>{{ label }}</button>' },
          Dialog: { template: '<div />' },
          Tag: { props: ['value'], template: '<span>{{ value }}</span>' },
        },
      },
    })
    await flushPromises()

    const link = wrapper.get('a.subject-card')
    expect(link.attributes('href')).toBe('/subjects/subject-hash')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toContain('noopener')
    expect(link.attributes('rel')).toContain('noreferrer')
    expect(wrapper.text()).toContain('Go 并发')
    expect(wrapper.text()).not.toContain('正文与标注')
  })
})
