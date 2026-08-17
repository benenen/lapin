// @vitest-environment node

import { describe, expect, it } from 'vitest'
import type { UserConfig } from 'vite'

import config from '../vite.config'

describe('Vite development proxy', () => {
  it('preserves the browser host for authentication origin validation', () => {
    const proxy = (config as UserConfig).server?.proxy

    for (const path of ['/api', '/openapi', '/healthz']) {
      expect(proxy?.[path]).toMatchObject({
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      })
    }
  })
})
