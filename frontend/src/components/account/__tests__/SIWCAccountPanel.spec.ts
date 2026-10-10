import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const generateSIWCAuthURL = vi.fn()
const exchangeSIWCCallback = vi.fn()
const revokeSIWCAccount = vi.fn()

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      generateSIWCAuthURL: (...args: unknown[]) => generateSIWCAuthURL(...args),
      exchangeSIWCCallback: (...args: unknown[]) => exchangeSIWCCallback(...args),
      revokeSIWCAccount: (...args: unknown[]) => revokeSIWCAccount(...args)
    }
  }
}))

import SIWCAccountPanel from '../SIWCAccountPanel.vue'

describe('SIWC account panel', () => {
  it('warns that the account is personal-use only and posts the loopback callback', async () => {
    generateSIWCAuthURL.mockResolvedValue({
      auth_url: 'https://auth.openai.com/oauth/authorize?state=abc',
      session_id: 'sess-1'
    })
    exchangeSIWCCallback.mockResolvedValue({
      credentials: { client_id: 'oaiapp_test' },
      email: 'owner@example.com',
      scopes: 'openid chatgpt.tokens.use.direct',
      expires_at: '2026-10-10T00:00:00Z',
      client_id: 'oaiapp_test'
    })

    const wrapper = mount(SIWCAccountPanel, {
      props: { mode: 'create', proxyId: null }
    })

    expect(wrapper.text()).toContain('admin.accounts.siwc.warning')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(generateSIWCAuthURL).toHaveBeenCalledWith({
      proxy_id: undefined,
      redirect_uri: 'http://127.0.0.1:1455/auth/callback',
      account_id: undefined,
      reconsent: false
    })

    await wrapper.get('textarea').setValue('http://127.0.0.1:1455/auth/callback?code=c&state=abc&client_id=oaiapp_test')
    const buttons = wrapper.findAll('button')
    await buttons[buttons.length - 1].trigger('click')
    await flushPromises()
    expect(exchangeSIWCCallback).toHaveBeenCalledWith({
      session_id: 'sess-1',
      callback_url: 'http://127.0.0.1:1455/auth/callback?code=c&state=abc&client_id=oaiapp_test'
    })
    expect(wrapper.emitted('credentials')?.[0]?.[0]).toEqual({ client_id: 'oaiapp_test' })
  })

  it('asks for a second click before revoking', async () => {
    revokeSIWCAccount.mockResolvedValue({ message: 'revoked' })
    const wrapper = mount(SIWCAccountPanel, {
      props: {
        mode: 'manage',
        accountId: 4,
        credentials: { email: 'owner@example.com', client_id: 'oaiapp_test', scopes: 'openid', expires_at: 'soon' }
      }
    })
    expect(wrapper.text()).toContain('owner@example.com')
    const revoke = wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.siwc.revoke')
    expect(revoke).toBeTruthy()
    await revoke!.trigger('click')
    expect(revokeSIWCAccount).not.toHaveBeenCalled()
    await wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.siwc.revokeConfirm')!.trigger('click')
    await flushPromises()
    expect(revokeSIWCAccount).toHaveBeenCalledWith(4)
    expect(wrapper.emitted('revoked')).toBeTruthy()
  })
})

describe('CreateAccountModal SIWC entry', () => {
  const source = readFileSync(resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'), 'utf8')
  const panel = readFileSync(resolve(process.cwd(), 'src/components/account/SIWCAccountPanel.vue'), 'utf8')

  it('adds the official SIWC account type without replacing OAuth or API Key', () => {
    expect(source).toContain('data-testid="openai-account-type-siwc"')
    expect(source).toContain("accountCategory = 'siwc'")
    expect(source).toContain("form.type = 'siwc'")
    expect(source).toContain('<SIWCAccountPanel')
    expect(panel).toContain("t('admin.accounts.siwc.warning')")
    expect(source).toContain('>OAuth<')
    expect(source).toContain('>API Key<')
  })
})
