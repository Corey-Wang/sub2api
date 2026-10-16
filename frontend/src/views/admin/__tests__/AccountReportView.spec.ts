import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import zh from '@/i18n/locales/zh'
import AccountReportView from '../AccountReportView.vue'
import { getAccountReport, type AccountReport } from '@/api/admin/accountReport'

vi.mock('@/api/admin/accountReport', () => ({ getAccountReport: vi.fn() }))
vi.mock('vue-i18n', async () => await vi.importActual('../../../../node_modules/vue-i18n/dist/vue-i18n.esm-bundler.js'))

const counts = { total: 2, available: 1, error: 0, limited: 0, disabled: 0, exhausted: 1, unknown: 0, unknown_quota: 1, near_limit: 1, identity_uncertain: 0 }
const now = '2026-10-16T02:00:00Z'
const fixture: AccountReport = {
  updated_at: now, pending: false, complete: false, counts,
  sources: [{ id: 'kiro-a', name: 'Kiro A', platform: 'kiro', updated_at: now, attempted_at: now, error: '', stale: false }, { id: 'sub2api-gpt', name: 'Sub2API', platform: 'gpt', updated_at: now, attempted_at: now, error: 'source_unavailable', stale: true }],
  platforms: ['gpt', 'gemini', 'kiro'].map(platform => ({
    platform: platform as 'gpt' | 'gemini' | 'kiro', counts, plans: [{ name: 'Pro', count: 1 }],
    quotas: platform === 'kiro' ? [{ key: 'kiro_credits', plan: 'Pro', unit: 'credits' as const, count: 1, used: 100, limit: 100, percent: 100, estimated: false }] : []
  })),
  accounts: [
    { id: 'kiro:1', platform: 'kiro', name: 'pro@example.test', plan: 'Pro', type: 'social', status: 'exhausted', quota_error: false, identity_uncertain: false, locations: [{ id: 'kiro-a', name: 'Kiro A', account_id: '1', status: 'available', updated_at: now, stale: false }], quotas: [{ key: 'kiro_credits', unit: 'credits', used: 100, limit: 100, percent: 100, reset_at: '2026-11-01T00:00:00Z', updated_at: now, estimated: false, stale: false }] },
    { id: 'gpt:1', platform: 'gpt', name: 'GPT account', plan: 'Plus', type: 'oauth', status: 'unknown', quota_error: true, identity_uncertain: false, locations: [{ id: 'sub2api-gpt', name: 'Sub2API', account_id: '1', status: 'available', updated_at: now, stale: true }], quotas: [{ key: 'gpt_5h', unit: 'percent', used: null, limit: null, percent: 75, reset_at: null, updated_at: now, estimated: false, stale: true }] }
  ]
}
let wrapper: VueWrapper | undefined
function render() {
  wrapper = mount(AccountReportView, { global: {
    plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })],
    stubs: { AppLayout: { template: '<main><slot /></main>' }, LoadingSpinner: true }
  } })
  return wrapper
}
beforeEach(() => { vi.mocked(getAccountReport).mockReset(); vi.mocked(getAccountReport).mockResolvedValue(structuredClone(fixture)) })
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })

describe('AccountReportView', () => {
  it('shows platform summaries, quota progress and source failures without management controls', async () => {
    const w = render(); await flushPromises()
    expect(w.get('[data-testid="report-totals"]').text()).toContain('已知账号总数')
    expect(w.text()).toContain('部分来源尚未同步或采集失败')
    expect(w.get('[data-testid="platform-kiro"]').text()).toContain('100.0%')
    expect(w.get('[data-testid="platform-kiro"]').text()).toContain('100 / 100 额度点')
    expect(w.text()).toContain('汇总 1 个账号的有效额度')
    expect(w.text()).toContain('GPT · Sub2API采集失败')
    expect(w.find('button[data-action="delete"]').exists()).toBe(false)
    expect(w.find('iframe').exists()).toBe(false)
  })

  it('only displays aggregates even when the report contains individual account details', async () => {
    const w = render(); await flushPromises()
    for (const account of fixture.accounts) expect(w.text()).not.toContain(account.name)
    expect(w.text()).not.toContain('账号明细')
    expect(w.findAll('table, input, select, [data-testid="report-account-row"]')).toHaveLength(0)
    for (const platform of fixture.platforms) {
      const card = w.get(`[data-testid="platform-${platform.platform}"]`)
      expect(card.element.tagName).toBe('DIV')
      expect(card.attributes('aria-pressed')).toBeUndefined()
    }
  })

  it('refreshes the cached report every minute and retains data after a read failure', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const w = render(); await flushPromises()
    vi.mocked(getAccountReport).mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(60000); await flushPromises()
    expect(getAccountReport).toHaveBeenCalledTimes(2)
    expect(w.get('[role="alert"]').text()).toContain('报表读取失败')
    expect(w.get('[data-testid="report-totals"]').text()).toContain('已知账号总数2')
    expect(w.get('[data-testid="platform-kiro"]').text()).toContain('100 / 100 额度点')
    w.unmount(); wrapper = undefined
    await vi.advanceTimersByTimeAsync(120000)
    expect(getAccountReport).toHaveBeenCalledTimes(2)
  })

  it('shows an empty instance without inventing quota progress', async () => {
    const emptyCounts = { total: 0, available: 0, error: 0, limited: 0, disabled: 0, exhausted: 0, unknown: 0, unknown_quota: 0, near_limit: 0, identity_uncertain: 0 }
    vi.mocked(getAccountReport).mockResolvedValue({ ...fixture, complete: true, counts: emptyCounts, accounts: [], platforms: fixture.platforms.map(p => ({ ...p, counts: emptyCounts, plans: [], quotas: [] })) })
    const w = render(); await flushPromises()
    expect(w.get('[data-testid="report-totals"]').text()).toContain('已知账号总数0')
    expect(w.get('[data-testid="platform-kiro"]').text()).toContain('0 个账号')
    expect(w.text()).toContain('暂无有效额度数据')
    expect(w.findAll('[role="progressbar"]')).toHaveLength(0)
  })
})
