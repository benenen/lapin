import { mount } from '@vue/test-utils'
import PrimeVue from 'primevue/config'
import { afterEach, describe, expect, it, vi } from 'vitest'

import AuthPanel from './AuthPanel.vue'

describe('AuthPanel', () => {
  afterEach(() => vi.restoreAllMocks())

  it('lets a new user switch to registration', async () => {
    const wrapper = mount(AuthPanel, {
      global: { plugins: [PrimeVue] },
    })

    await wrapper.get('button.text-button').trigger('click')

    expect(wrapper.text()).toContain('创建学习空间')
    expect(wrapper.find('input[autocomplete="name"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('注册并进入')
  })
})
