import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import EncryptedImportModal from '@/components/admin/account/EncryptedImportModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning: vi.fn()
  })
}))

const getEncryptedImportPublicKey = vi.fn()
const importEncryptedAccounts = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getEncryptedImportPublicKey: (...args: unknown[]) => getEncryptedImportPublicKey(...args),
      importEncryptedAccounts: (...args: unknown[]) => importEncryptedAccounts(...args)
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const mountModal = () =>
  mount(EncryptedImportModal, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
      }
    }
  })

describe('EncryptedImportModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    getEncryptedImportPublicKey.mockReset().mockResolvedValue({
      configured: true,
      kind: 'sub2api.encrypted_account_bundle',
      version: 1,
      alg: 'x25519-hkdf-sha256-xchacha20poly1305',
      kid: 'sha256:abc',
      public_key: 'pub-key-material'
    })
    importEncryptedAccounts.mockReset()
  })

  it('loads the public key and does not show a private key', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(getEncryptedImportPublicKey).toHaveBeenCalled()
    expect(wrapper.text()).toContain('pub-key-material')
    expect(wrapper.text()).not.toContain('ACCOUNT_IMPORT_PRIVATE_KEY')
  })

  it('requires a file or paste before importing', async () => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.encryptedImportSelectFile')
    expect(importEncryptedAccounts).not.toHaveBeenCalled()
  })

  it('imports pasted encrypted JSON and reports per-item results', async () => {
    importEncryptedAccounts.mockResolvedValue({
      total: 2,
      created: 1,
      updated: 0,
      skipped: 0,
      failed: 1,
      items: [
        { index: 1, name: 'desk-1', action: 'created' },
        { index: 2, name: 'desk-2', action: 'failed', message: '解密失败' }
      ]
    })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.find('textarea.input-field.min-h-\\[8rem\\]').setValue('{"kind":"sub2api.encrypted_account_bundle"}')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(importEncryptedAccounts).toHaveBeenCalledTimes(1)
    const payload = importEncryptedAccounts.mock.calls[0][0]
    expect(payload.contents[0]).toContain('encrypted_account_bundle')
    expect(wrapper.text()).toContain('desk-1')
    expect(wrapper.text()).toContain('failed')
  })
})
