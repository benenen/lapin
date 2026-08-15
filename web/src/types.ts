export interface User {
  id: string
  email: string
  name: string
  avatar_url: string
  roles: string[]
  created_at: string
}

export interface Chapter {
  id: string
  parent_id?: string
  external_id?: string
  position: number
  title: string
  content: string
  created_at: string
}

export interface Subject {
  id: string
  owner_id: string
  owner_name: string
  external_id?: string
  title: string
  description: string
  tags: string[]
  chapters?: Chapter[]
  created_at: string
  updated_at: string
}

export interface Annotation {
  id: string
  chapter_id: string
  user_id: string
  author_name: string
  start_offset: number
  end_offset: number
  quote: string
  note: string
  color: string
  created_at: string
}

export interface Whiteboard {
  id: string
  chapter_id: string
  user_id: string
  author_name: string
  data: WhiteboardData
  updated_at: string
}

export interface WhiteboardData {
  version: 2
  anchor: {
    type: 'chapter'
    id: string
    content_revision: string
  }
  space: {
    width: number
    height: number
    fit: 'contain'
  }
  document: {
    store: Record<string, unknown>
    schema: Record<string, unknown>
  }
}

export interface Comment {
  id: string
  chapter_id: string
  user_id: string
  author_name: string
  body: string
  created_at: string
}

export interface AccessToken {
  id: string
  name: string
  prefix: string
  last_used_at?: string
  expires_at: string
  created_at: string
}
