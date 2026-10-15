/** Menu presentation only: hiding an entry never disables its route or API. */
export const CONFIGURABLE_MENUS = [
  {"id": "admin_dashboard", "label": "nav.dashboard", "paths": ["/admin/dashboard"], "group": "admin"},
  {"id": "ops", "label": "nav.ops", "paths": ["/admin/ops"], "group": "admin"},
  {"id": "users", "label": "nav.users", "paths": ["/admin/users"], "group": "admin"},
  {"id": "groups", "label": "nav.groups", "paths": ["/admin/groups"], "group": "admin"},
  {"id": "channels", "label": "nav.channelManagement", "paths": ["/admin/channels"], "group": "admin"},
  {"id": "channel_pricing", "label": "nav.channelPricing", "paths": ["/admin/channels/pricing"], "group": "admin", "parent": "channels"},
  {"id": "channel_monitor", "label": "nav.channelMonitor", "paths": ["/admin/channels/monitor"], "group": "admin", "parent": "channels"},
  {"id": "accounts", "label": "nav.accounts", "paths": ["/admin/accounts"], "group": "admin"},
  {"id": "plugins", "label": "nav.plugins", "paths": ["/admin/plugins"], "group": "admin"},
  {"id": "announcements", "label": "nav.announcements", "paths": ["/admin/announcements"], "group": "admin"},
  {"id": "proxies", "label": "nav.proxies", "paths": ["/admin/proxies"], "group": "admin"},
  {"id": "security_audit", "label": "nav.securityAudit", "paths": ["/admin/security-audit"], "group": "admin"},
  {"id": "content_moderation", "label": "nav.contentModeration", "paths": ["/admin/risk-control"], "group": "admin", "parent": "security_audit"},
  {"id": "prompt_audit", "label": "nav.promptAudit", "paths": ["/admin/prompt-audit"], "group": "admin", "parent": "security_audit"},
  {"id": "promo_codes", "label": "nav.promoCodes", "paths": ["/admin/promo-codes"], "group": "admin"},
  {"id": "affiliate_management", "label": "nav.affiliateManagement", "paths": ["/admin/affiliates"], "group": "admin"},
  {"id": "affiliate_invites", "label": "nav.affiliateInviteRecords", "paths": ["/admin/affiliates/invites"], "group": "admin", "parent": "affiliate_management"},
  {"id": "affiliate_rebates", "label": "nav.affiliateRebateRecords", "paths": ["/admin/affiliates/rebates"], "group": "admin", "parent": "affiliate_management"},
  {"id": "affiliate_transfers", "label": "nav.affiliateTransferRecords", "paths": ["/admin/affiliates/transfers"], "group": "admin", "parent": "affiliate_management"},
  {"id": "order_management", "label": "nav.orderManagement", "paths": [], "group": "admin"},
  {"id": "payment_dashboard", "label": "nav.paymentDashboard", "paths": ["/admin/orders/dashboard"], "group": "admin", "parent": "order_management"},
  {"id": "orders_admin", "label": "nav.orderManagement", "paths": ["/admin/orders"], "group": "admin", "parent": "order_management"},
  {"id": "order_plans", "label": "nav.paymentPlans", "paths": ["/admin/orders/plans"], "group": "admin", "parent": "order_management"},
  {"id": "admin_usage", "label": "nav.usage", "paths": ["/admin/usage"], "group": "admin"},
  {"id": "audit_logs", "label": "nav.auditLogs", "paths": ["/admin/audit-logs"], "group": "admin"},
  {"id": "settings", "label": "nav.settings", "paths": ["/admin/settings"], "group": "admin"},
  {"id": "dashboard", "label": "nav.dashboard", "paths": ["/dashboard"], "group": "user"},
  {"id": "keys", "label": "nav.apiKeys", "paths": ["/keys"], "group": "user"},
  {"id": "batch_image", "label": "nav.batchImage", "paths": ["/batch-image"], "group": "user"},
  {"id": "usage", "label": "nav.usage", "paths": ["/usage"], "group": "user"},
  {"id": "available_channels", "label": "nav.availableChannels", "paths": ["/available-channels"], "group": "user"},
  {"id": "channel_status", "label": "nav.channelStatus", "paths": ["/monitor"], "group": "user"},
  {"id": "purchase", "label": "nav.buySubscription", "paths": ["/purchase"], "group": "user"},
  {"id": "orders", "label": "nav.myOrders", "paths": ["/orders"], "group": "user"},
  {"id": "affiliate", "label": "nav.affiliate", "paths": ["/affiliate"], "group": "user"},
  {"id": "profile", "label": "nav.profile", "paths": ["/profile"], "group": "user"},
  {"id": "docs", "label": "nav.docs", "paths": ["/navigation/docs"], "group": "header"},
  {"id": "model_plaza", "label": "nav.modelPlaza", "paths": ["/model-plaza"], "group": "header"},
  {"id": "github", "label": "nav.github", "paths": ["/navigation/github"], "group": "header"},
  {"id": "announcements_user", "label": "nav.announcements", "paths": ["/navigation/announcements"], "group": "header"},
  {"id": "contact_support", "label": "common.contactSupport", "paths": ["/navigation/contact-support"], "group": "header"},
  {"id": "language_switcher", "label": "admin.settings.features.menuVisibility.language", "paths": ["/navigation/language"], "group": "header"},
  {"id": "logout", "label": "nav.logout", "paths": ["/navigation/logout"], "group": "header"},
  {"id": "theme_switcher", "label": "admin.settings.features.menuVisibility.theme", "paths": ["/navigation/theme"], "group": "header"},
  {"id": "sidebar_collapse", "label": "admin.settings.features.menuVisibility.collapse", "paths": ["/navigation/sidebar-collapse"], "group": "header"},
  {"id": "subscriptions", "label": "nav.subscriptions", "paths": ["/admin/subscriptions", "/subscriptions"], "group": "shared"},
  {"id": "redeem", "label": "nav.redeemCodes", "paths": ["/admin/redeem", "/redeem"], "group": "shared"},
] as const

export type ConfigurableMenuID = typeof CONFIGURABLE_MENUS[number]['id'] | `custom:${string}`
export const MENU_GROUPS = ['admin', 'user', 'shared', 'header'] as const
const menuIDByPath = new Map<string, ConfigurableMenuID>(
  CONFIGURABLE_MENUS.flatMap(menu => menu.paths.map(path => [path, menu.id] as const)),
)

export function isMenuVisible(path: string, hiddenItems?: readonly string[] | null, explicitID?: ConfigurableMenuID): boolean {
  const id = explicitID || (path.startsWith('/custom/') ? `custom:${path.slice('/custom/'.length)}` : menuIDByPath.get(path))
  return !id || !hiddenItems?.includes(id)
}
