import type { AccessToken, Annotation, Chapter, Comment, Subject, User, Whiteboard, WhiteboardData } from './types'

interface Envelope<T> {
  data?: T
  error?: {
    code: string
    message: string
  }
}

type AllowedRequestInit = Omit<RequestInit, 'method'> & {
  method?: 'GET' | 'POST'
}

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code: string,
  ) {
    super(message)
  }
}

function cookie(name: string): string {
  const prefix = `${encodeURIComponent(name)}=`
  const part = document.cookie.split('; ').find((entry) => entry.startsWith(prefix))
  return part ? decodeURIComponent(part.slice(prefix.length)) : ''
}

async function request<T>(path: string, options: AllowedRequestInit = {}): Promise<T> {
  const method = options.method?.toUpperCase() ?? 'GET'
  if (method !== 'GET' && method !== 'POST') {
    throw new Error(`unsupported HTTP method: ${method}`)
  }
  const headers = new Headers(options.headers)
  if (options.body) {
    headers.set('Content-Type', 'application/json')
  }
  if (method === 'POST') {
    headers.set('X-CSRF-Token', cookie('lapin_csrf'))
  }
  const response = await fetch(path, {
    ...options,
    credentials: 'same-origin',
    headers,
  })
  const payload = (await response.json()) as Envelope<T>
  if (!response.ok || payload.error) {
    throw new ApiError(payload.error?.message ?? '请求失败', response.status, payload.error?.code ?? 'request_failed')
  }
  return payload.data as T
}

export const api = {
  register: (input: { email: string; name: string; avatar_url: string; password: string }) =>
    request<{ user: User; csrf_token: string }>('/api/v1/auth/register', { method: 'POST', body: JSON.stringify(input) }),
  login: (input: { email: string; password: string }) =>
    request<{ user: User; csrf_token: string }>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify(input) }),
  logout: () => request<{ logged_out: boolean }>('/api/v1/auth/logout', { method: 'POST', body: '{}' }),
  me: () => request<User>('/api/v1/me'),
  listSubjects: () => request<Subject[]>('/api/v1/subjects'),
  getSubject: (id: string) => request<Subject>(`/api/v1/subjects/${id}`),
  createSubject: (input: { title: string; description: string; tags: string[]; chapters: Array<{ title: string; content: string }> }) =>
    request<Subject>('/api/v1/subjects', { method: 'POST', body: JSON.stringify(input) }),
  updateSubject: (id: string, input: { title: string; description: string }) =>
    request<Subject>(`/api/v1/subjects/${id}/update`, { method: 'POST', body: JSON.stringify(input) }),
  createChapter: (subjectId: string, input: { parent_id?: string; title: string; content: string }) =>
    request<Chapter>(`/api/v1/subjects/${subjectId}/chapters`, { method: 'POST', body: JSON.stringify(input) }),
  updateChapter: (id: string, input: { title: string; content: string }) =>
    request<Chapter>(`/api/v1/chapters/${id}/update`, { method: 'POST', body: JSON.stringify(input) }),
  listAnnotations: (chapterId: string) => request<Annotation[]>(`/api/v1/chapters/${chapterId}/annotations`),
  createAnnotation: (chapterId: string, input: Omit<Annotation, 'id' | 'chapter_id' | 'user_id' | 'author_name' | 'created_at'>) =>
    request<Annotation>(`/api/v1/chapters/${chapterId}/annotations`, { method: 'POST', body: JSON.stringify(input) }),
  listWhiteboards: (chapterId: string) => request<Whiteboard[]>(`/api/v1/chapters/${chapterId}/whiteboards`),
  saveWhiteboard: (chapterId: string, data: WhiteboardData) =>
    request<Whiteboard>(`/api/v1/chapters/${chapterId}/whiteboard`, { method: 'POST', body: JSON.stringify({ data }) }),
  listComments: (chapterId: string) => request<Comment[]>(`/api/v1/chapters/${chapterId}/comments`),
  createComment: (chapterId: string, body: string) =>
    request<Comment>(`/api/v1/chapters/${chapterId}/comments`, { method: 'POST', body: JSON.stringify({ body }) }),
  listTokens: () => request<AccessToken[]>('/api/v1/access-tokens'),
  createToken: (name: string) =>
    request<{ access_token: string; token: AccessToken }>('/api/v1/access-tokens', { method: 'POST', body: JSON.stringify({ name }) }),
  revokeToken: (id: string) => request<{ revoked: boolean }>(`/api/v1/access-tokens/${id}/revoke`, { method: 'POST', body: '{}' }),
}
