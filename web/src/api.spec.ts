import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from './api'

describe('API HTTP method convention', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: {} }),
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  it('uses explicit POST action routes for updates and revocations', async () => {
    await api.updateSubject('subject-id', { title: '新标题', description: '新简介' })
    await api.updateChapter('chapter-id', { title: '新章节', content: '# 新正文' })
    await api.revokeToken('token-id')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/subjects/subject-id/update', expect.objectContaining({ method: 'POST' }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/chapters/chapter-id/update', expect.objectContaining({ method: 'POST' }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/access-tokens/token-id/revoke', expect.objectContaining({ method: 'POST' }))
  })

  it('uploads assets as multipart without overriding the browser boundary', async () => {
    const file = new File(['png'], 'figure.png', { type: 'image/png' })
    await api.uploadAsset(file)

    const [, options] = fetchMock.mock.calls[0]
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/assets', expect.objectContaining({ method: 'POST' }))
    expect(options.body).toBeInstanceOf(FormData)
    expect((options.headers as Headers).has('Content-Type')).toBe(false)
  })
})
