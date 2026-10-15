import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AppHeader from '../AppHeader.vue'

const state = {
  app: reactive({ cachedPublicSettings: { hidden_menu_items: [] as string[], model_plaza_enabled: true, subscription_enabled: true }, docUrl: 'https://docs.example.com/docs', contactInfo: 'Support fixture', publicSettingsLoaded: true }),
  auth: reactive({ user: { role: 'admin', username: 'Fixture', email: 'fixture@example.com', balance: 0 }, isAdmin: true, isSimpleMode: false, logout: vi.fn() }),
}
vi.mock('@/components/common/LocaleSwitcher.vue', () => ({ default: { template: '<span data-testid="language" />' } }))
vi.mock('@/components/common/SubscriptionProgressMini.vue', () => ({ default: { template: '<span data-testid="subscription-progress" />' } }))
vi.mock('@/components/common/AnnouncementBell.vue', () => ({ default: { template: '<span data-testid="announcements" />' } }))
vi.mock('@/stores', () => ({ useAppStore: () => state.app, useAuthStore: () => state.auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => state.app }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({}) }))
vi.mock('vue-i18n', async importOriginal => ({ ...(await importOriginal<typeof import('vue-i18n')>()), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({ useRoute: () => ({ path: '/admin/settings', meta: {}, matched: [] }), useRouter: () => ({ push: vi.fn() }) }))
const Link = defineComponent({ props: ['to'], template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>' })
const make = () => mount(AppHeader, { global: { mocks: { $t: (key: string) => key }, stubs: { RouterLink: Link, Icon: true, AnnouncementBell: { template: '<span data-testid="announcements" />' }, LocaleSwitcher: { template: '<span data-testid="language" />' }, SubscriptionProgressMini: { template: '<span data-testid="subscription-progress" />' } } } })

beforeEach(() => { state.app.cachedPublicSettings.hidden_menu_items = []; state.app.cachedPublicSettings.model_plaza_enabled = true })
describe('header menu visibility', () => {
  it('hides and restores navigation, personal dropdown items and header actions reactively', async () => {
    const wrapper = make()
    await wrapper.get('button[aria-label="common.userMenu"]').trigger('click')
    expect(wrapper.find('a[href="/profile"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/keys"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/model-plaza"]').exists()).toBe(true)
    expect(wrapper.find('a[href="https://docs.example.com/docs"]').exists()).toBe(true)
    expect(wrapper.find('a[href="https://github.com/Wei-Shaw/sub2api"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="subscription-progress"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('onboarding.restartTour')
    state.app.cachedPublicSettings.hidden_menu_items = ['profile', 'keys', 'docs', 'model_plaza', 'github', 'announcements_user', 'language_switcher', 'subscriptions', 'contact_support', 'logout', 'onboarding']
    await nextTick()
    expect(wrapper.findAll('a')).toHaveLength(0)
    for (const id of ['announcements', 'language', 'subscription-progress']) expect(wrapper.find(`[data-testid="${id}"]`).exists()).toBe(false)
    for (const text of ['nav.logout', 'onboarding.restartTour', 'Support fixture']) expect(wrapper.text()).not.toContain(text)
    state.app.cachedPublicSettings.hidden_menu_items = []
    await nextTick()
    expect(wrapper.find('a[href="/profile"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="announcements"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('nav.logout')
    wrapper.unmount()
  })
  it('never enables a disabled feature merely because its menu is visible', () => {
    state.app.cachedPublicSettings.model_plaza_enabled = false
    const wrapper = make()
    expect(wrapper.find('a[href="/model-plaza"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
