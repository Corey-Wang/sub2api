import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, reactive } from 'vue'
import { mount } from '@vue/test-utils'
import AppSidebar from '../AppSidebar.vue'
import { CONFIGURABLE_MENUS } from '@/utils/menuVisibility'
import type { PublicSettings } from '@/types'

const state = {
  app: reactive({
    cachedPublicSettings: {} as Partial<PublicSettings>,
    sidebarCollapsed: false,
    publicSettingsLoaded: true,
    siteName: 'Example',
  }),
  auth: reactive({ isAdmin: true, isSimpleMode: false }),
  admin: reactive({ fetch: vi.fn(), customMenuItems: [] as { id: string; label: string; visibility: string; sort_order: number; icon_svg: string }[], paymentEnabled: true, opsMonitoringEnabled: true }),
  route: reactive({ path: '/admin/channels/pricing' }),
}
vi.mock('@/components/common/VersionBadge.vue', () => ({ default: { template: '<span />' } }))
vi.mock('@/stores', () => ({
  useAppStore: () => state.app,
  useAuthStore: () => state.auth,
  useAdminSettingsStore: () => state.admin,
  useOnboardingStore: () => ({ isCurrentStep: () => false }),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => state.app }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({
  useRoute: () => state.route,
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: true, refreshBatchImageAccess: vi.fn() }),
}))

const Link = defineComponent({
  props: ['to'],
  template: '<a :href="to"><slot /></a>',
})
const hiddenMenus = ['subscriptions', 'channels', 'announcements', 'redeem', 'promo_codes']

function mountSidebar() {
  return mount(AppSidebar, {
    global: { stubs: { RouterLink: Link, VersionBadge: true, Icon: true } },
  })
}

beforeEach(() => {
  state.app.cachedPublicSettings = { hidden_menu_items: [], subscription_enabled: true }
  state.admin.customMenuItems = []
  state.route.path = '/admin/channels/pricing'
  state.auth.isAdmin = true
  state.auth.isSimpleMode = false
})

describe('sidebar menu visibility', () => {
  it('hides and restores admin entries, the full channel group and personal entries without reloading', async () => {
    const wrapper = mountSidebar()
    const paths = [
      '/admin/subscriptions', '/admin/channels/pricing', '/admin/announcements',
      '/admin/redeem', '/admin/promo-codes', '/subscriptions', '/redeem',
    ]
    paths.forEach(path => expect(wrapper.find(`a[href="${path}"]`).exists()).toBe(true))
    state.app.cachedPublicSettings.hidden_menu_items = hiddenMenus
    await nextTick()
    paths.forEach(path => expect(wrapper.find(`a[href="${path}"]`).exists()).toBe(false))
    expect(wrapper.text()).not.toContain('nav.channelManagement')
    expect(wrapper.find('a[href="/admin/settings"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/keys"]').exists()).toBe(true)
    state.app.cachedPublicSettings.hidden_menu_items = []
    await nextTick()
    paths.forEach(path => expect(wrapper.find(`a[href="${path}"]`).exists()).toBe(true))
    wrapper.unmount()
  })

  it('hides the user entries on the first render with saved settings', () => {
    state.auth.isAdmin = false
    state.app.cachedPublicSettings.hidden_menu_items = hiddenMenus
    const wrapper = mountSidebar()
    expect(wrapper.find('a[href="/subscriptions"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/redeem"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/usage"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('hides every built-in sidebar entry and restores it reactively', async () => {
    state.app.cachedPublicSettings = { hidden_menu_items: [], subscription_enabled: true, plugin_management_enabled: true, risk_control_enabled: true, affiliate_enabled: true, available_channels_enabled: true, channel_monitor_enabled: true, payment_enabled: true }
    state.route.path = '/admin/dashboard'
    const wrapper = mountSidebar()
    for (const button of wrapper.findAll('.sidebar-nav button')) await button.trigger('click')
    const initiallyVisible = wrapper.findAll('.sidebar-nav a').map(link => link.attributes('href'))
    expect(initiallyVisible.length).toBeGreaterThan(25)
    for (const path of initiallyVisible) {
      const menu = CONFIGURABLE_MENUS.find(menu => (menu.paths as readonly string[]).includes(path))
      expect(menu, path).toBeDefined()
      state.app.cachedPublicSettings.hidden_menu_items = [menu!.id]
      await nextTick()
      expect(wrapper.find(`.sidebar-nav a[href="${path}"]`).exists(), path).toBe(false)
      state.app.cachedPublicSettings.hidden_menu_items = []
      await nextTick()
    }
    state.app.cachedPublicSettings.hidden_menu_items = CONFIGURABLE_MENUS.map(menu => menu.id)
    await nextTick()
    expect(wrapper.findAll('.sidebar-nav a')).toHaveLength(0)
    expect(wrapper.findAll('.sidebar-nav button')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('nav.darkMode')
    expect(wrapper.text()).not.toContain('nav.lightMode')
    expect(wrapper.text()).not.toContain('nav.collapse')
    expect(wrapper.text()).not.toContain('nav.myAccount')
    state.app.cachedPublicSettings.hidden_menu_items = []
    await nextTick()
    expect(wrapper.find('.sidebar-nav a[href="/admin/settings"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('removes empty groups and handles order parent and child separately', async () => {
    state.app.cachedPublicSettings.hidden_menu_items = ['channel_pricing', 'channel_monitor']
    const wrapper = mountSidebar()
    expect(wrapper.text()).not.toContain('nav.channelManagement')
    expect(wrapper.find('a[href="/admin/channels"]').exists()).toBe(false)
    state.app.cachedPublicSettings.hidden_menu_items = ['orders_admin']
    await nextTick()
    expect(wrapper.findAll('.sidebar-nav button').some(button => button.text() === 'nav.orderManagement')).toBe(true)
    const button = wrapper.findAll('.sidebar-nav button').find(button => button.text() === 'nav.orderManagement')!
    await button.trigger('click')
    expect(wrapper.find('a[href="/admin/orders"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/admin/orders/dashboard"]').exists()).toBe(true)
    state.app.cachedPublicSettings.hidden_menu_items = ['order_management']
    await nextTick()
    expect(wrapper.find('a[href="/admin/orders/dashboard"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('filters settings, API keys and custom entries in simple mode as well', () => {
    state.auth.isSimpleMode = true
    state.admin.customMenuItems = [{ id: 'docs', label: 'Custom docs', visibility: 'admin', sort_order: 0, icon_svg: '' }]
    state.app.cachedPublicSettings.hidden_menu_items = ['settings', 'keys', 'custom:docs']
    const wrapper = mountSidebar()
    expect(wrapper.find('a[href="/admin/settings"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/keys"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/custom/docs"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('covers all regular-user menus and individual custom entries', async () => {
    state.auth.isAdmin = false
    state.app.cachedPublicSettings = { subscription_enabled: true, available_channels_enabled: true, affiliate_enabled: true, hidden_menu_items: [], custom_menu_items: [{ id: 'docs', label: 'Docs', url: 'https://example.com', visibility: 'user', sort_order: 0, icon_svg: '' }] }
    const wrapper = mountSidebar()
    expect(wrapper.find('a[href="/custom/docs"]').exists()).toBe(true)
    state.app.cachedPublicSettings.hidden_menu_items = [...CONFIGURABLE_MENUS.map(menu => menu.id), 'custom:docs']
    await nextTick()
    expect(wrapper.findAll('.sidebar-nav a')).toHaveLength(0)
    state.app.cachedPublicSettings.hidden_menu_items = ['custom:docs']
    await nextTick()
    expect(wrapper.find('a[href="/dashboard"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/custom/docs"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('preserves old settings defaults and existing subscription feature restrictions', async () => {
    delete state.app.cachedPublicSettings.hidden_menu_items
    const wrapper = mountSidebar()
    expect(wrapper.find('a[href="/redeem"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/admin/subscriptions"]').exists()).toBe(true)
    state.app.cachedPublicSettings.subscription_enabled = false
    await nextTick()
    expect(wrapper.find('a[href="/admin/subscriptions"]').exists()).toBe(false)
    expect(wrapper.find('a[href="/subscriptions"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
