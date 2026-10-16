<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accountReport.updatedAt') }} {{ date(report?.updated_at ?? null) }}</p>
        <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('admin.accountReport.refresh') }}</button>
      </div>

      <div v-if="error" role="alert" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ t('admin.accountReport.loadError') }}</div>
      <div v-if="loading && !report" class="flex justify-center py-16"><LoadingSpinner /></div>

      <template v-if="report">
        <div v-if="report.pending" role="status" class="rounded-xl bg-primary-50 p-4 text-sm text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ t('admin.accountReport.pending') }}</div>
        <div v-else-if="!report.complete" role="status" class="rounded-xl bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">{{ t('admin.accountReport.incomplete') }}</div>

        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4" data-testid="report-totals">
          <div v-for="stat in headlineStats" :key="stat.key" class="card p-4">
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ t(`admin.accountReport.${stat.key}`) }}</p>
            <p class="mt-2 text-3xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ stat.value }}</p>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
          <button v-for="p in report.platforms" :key="p.platform" type="button" class="card flex flex-col items-stretch justify-start space-y-4 p-5 text-left transition-shadow hover:shadow-md" :class="platform === p.platform ? 'ring-2 ring-primary-500' : ''" :aria-pressed="platform === p.platform" :data-testid="`platform-${p.platform}`" @click="platform = platform === p.platform ? '' : p.platform">
            <div class="flex items-center justify-between gap-3">
              <span class="text-lg font-semibold text-gray-900 dark:text-white">{{ platformName(p.platform) }}</span>
              <span class="text-2xl font-semibold tabular-nums">{{ p.counts.total }} <span class="text-xs font-normal text-gray-500">{{ t('admin.accountReport.accounts') }}</span></span>
            </div>
            <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
              <span class="text-emerald-600 dark:text-emerald-400">{{ t('admin.accountReport.status.available') }} {{ p.counts.available }}</span>
              <span>{{ t('admin.accountReport.status.limited') }} {{ p.counts.limited }}</span>
              <span>{{ t('admin.accountReport.status.error') }} {{ p.counts.error }}</span>
              <span>{{ t('admin.accountReport.status.exhausted') }} {{ p.counts.exhausted }}</span>
              <span v-if="p.counts.disabled">{{ t('admin.accountReport.status.disabled') }} {{ p.counts.disabled }}</span>
              <span v-if="p.counts.unknown">{{ t('admin.accountReport.status.unknown') }} {{ p.counts.unknown }}</span>
            </div>
            <div class="flex flex-wrap gap-1.5">
              <span v-for="plan in p.plans" :key="plan.name" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ planName(plan.name) }} · {{ plan.count }}</span>
            </div>
            <div v-if="p.quotas.some(q => !q.key.endsWith('_minute'))" class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700">
              <div v-for="q in p.quotas.filter(q => !q.key.endsWith('_minute'))" :key="q.key + q.plan + q.model">
                <p v-if="q.plan" class="mb-1 text-xs font-medium">{{ planName(q.plan) }}</p>
                <QuotaProgress :quota="q" />
              </div>
            </div>
            <p v-else class="text-sm text-gray-400">{{ t(p.platform === 'kiro' && !report.sources.some(s => s.platform === 'kiro') ? 'admin.accountReport.noInstances' : 'admin.accountReport.noQuota') }}</p>
            <p v-if="p.counts.unknown_quota" class="text-xs text-gray-500">{{ t('admin.accountReport.unknownQuotaCount', { count: p.counts.unknown_quota }) }}</p>
          </button>
        </div>

        <div class="card p-4">
          <div class="flex flex-wrap gap-x-6 gap-y-3">
            <div v-for="source in report.sources" :key="source.id" class="text-xs">
              <span class="mr-1.5 inline-block h-2 w-2 rounded-full" :class="source.stale ? 'bg-amber-500' : 'bg-emerald-500'" />
              <span class="font-medium">{{ platformName(source.platform) }} · {{ source.name }}</span>
              <span class="ml-2 text-gray-500 dark:text-gray-400">{{ source.error ? t('admin.accountReport.sourceError') : source.stale ? t('admin.accountReport.stale') : t('admin.accountReport.synced') }}</span>
              <span class="ml-2 text-gray-400">{{ date(source.updated_at) }}</span>
            </div>
          </div>
          <p class="mt-3 text-xs text-gray-400">{{ t('admin.accountReport.refreshNote') }}</p>
        </div>

        <div class="card overflow-hidden">
          <div class="space-y-3 border-b border-gray-100 p-4 dark:border-dark-700">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.accountReport.details') }} <span class="ml-1 text-sm font-normal text-gray-500">{{ filtered.length }}</span></h2>
              <button v-if="hasFilters" class="text-sm text-primary-600 hover:underline" @click="clearFilters">{{ t('admin.accountReport.clearFilters') }}</button>
            </div>
            <div class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-5">
              <input v-model="search" type="search" class="input" :aria-label="t('admin.accountReport.search')" :placeholder="t('admin.accountReport.search')" data-testid="report-search" />
              <select v-model="platform" class="input" :aria-label="t('admin.accountReport.platform')" data-testid="report-platform-filter">
                <option value="">{{ t('admin.accountReport.allPlatforms') }}</option>
                <option v-for="p in ['gpt', 'gemini', 'kiro']" :key="p" :value="p">{{ platformName(p) }}</option>
              </select>
              <select v-model="plan" class="input" :aria-label="t('admin.accountReport.plan')" data-testid="report-plan-filter">
                <option value="">{{ t('admin.accountReport.allPlans') }}</option>
                <option v-for="p in plans" :key="p" :value="p || '__unknown__'">{{ planName(p) }}</option>
              </select>
              <select v-model="instance" class="input" :aria-label="t('admin.accountReport.instance')" data-testid="report-instance-filter">
                <option value="">{{ t('admin.accountReport.allInstances') }}</option>
                <option v-for="s in report.sources" :key="s.id" :value="s.id">{{ platformName(s.platform) }} · {{ s.name }}</option>
              </select>
              <select v-model="status" class="input" :aria-label="t('admin.accountReport.statusLabel')" data-testid="report-status-filter">
                <option value="">{{ t('admin.accountReport.allStatuses') }}</option>
                <option v-for="s in statuses" :key="s" :value="s">{{ t(`admin.accountReport.status.${s}`) }}</option>
              </select>
            </div>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full min-w-[900px] text-left text-sm">
              <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400"><tr>
                <th class="px-4 py-3">{{ t('admin.accountReport.platform') }}</th>
                <th class="px-4 py-3">{{ t('admin.accountReport.account') }}</th>
                <th class="px-4 py-3">{{ t('admin.accountReport.plan') }}</th>
                <th class="px-4 py-3">{{ t('admin.accountReport.instance') }}</th>
                <th class="px-4 py-3">{{ t('admin.accountReport.statusLabel') }}</th>
                <th class="w-72 px-4 py-3">{{ t('admin.accountReport.progress') }}</th>
              </tr></thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="a in paged" :key="a.id" class="align-top" data-testid="report-account-row">
                  <td class="px-4 py-4 font-medium">{{ platformName(a.platform) }}</td>
                  <td class="max-w-60 break-words px-4 py-4">
                    <p class="font-medium text-gray-900 dark:text-white">{{ a.name }}</p>
                    <p class="mt-1 text-xs text-gray-500">{{ typeName(a.type) }}</p>
                    <p v-if="a.identity_uncertain" class="mt-1 text-xs text-amber-600">{{ t('admin.accountReport.identityUncertain') }}</p>
                  </td>
                  <td class="px-4 py-4">{{ planName(a.plan) }}</td>
                  <td class="px-4 py-4"><div v-for="location in a.locations" :key="location.id + location.account_id" class="mb-1 text-xs">
                    {{ location.name }} <span class="text-gray-400">#{{ location.account_id }} · {{ t(`admin.accountReport.status.${location.status}`) }}{{ location.stale ? ` · ${t('admin.accountReport.stale')}` : '' }}</span>
                  </div></td>
                  <td class="px-4 py-4"><span class="whitespace-nowrap rounded-md px-2 py-1 text-xs" :class="statusClass(a.status)">{{ t(`admin.accountReport.status.${a.status}`) }}</span></td>
                  <td class="space-y-4 px-4 py-4">
                    <QuotaProgress v-for="q in a.quotas" :key="q.key + q.model" :quota="q" />
                    <p v-if="!a.quotas.length" class="text-xs text-gray-400">{{ t('admin.accountReport.noQuota') }}</p>
                    <p v-if="a.quota_error" class="text-xs text-amber-600">{{ t('admin.accountReport.quotaError') }}</p>
                  </td>
                </tr>
                <tr v-if="!filtered.length"><td colspan="6" class="px-4 py-12 text-center text-gray-500">{{ hasFilters ? t('admin.accountReport.noMatches') : t('admin.accountReport.noAccounts') }}</td></tr>
              </tbody>
            </table>
          </div>
          <div v-if="pages > 1" class="flex items-center justify-end gap-3 border-t border-gray-100 p-4 text-sm dark:border-dark-700">
            <button class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="page--">{{ t('admin.accountReport.previous') }}</button>
            <span>{{ page }} / {{ pages }}</span>
            <button class="btn btn-secondary btn-sm" :disabled="page >= pages" @click="page++">{{ t('common.next') }}</button>
          </div>
        </div>
        <p class="text-xs leading-relaxed text-gray-400">{{ t('admin.accountReport.quotaNote') }} {{ t('admin.accountReport.identityNote') }}</p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import QuotaProgress from '@/components/admin/account-report/QuotaProgress.vue'
import { getAccountReport, type AccountReport, type ReportStatus } from '@/api/admin/accountReport'

const { t, locale } = useI18n()
const report = ref<AccountReport | null>(null)
const loading = ref(false)
const error = ref(false)
const search = ref('')
const platform = ref('')
const plan = ref('')
const instance = ref('')
const status = ref('')
const page = ref(1)
const statuses: ReportStatus[] = ['available', 'limited', 'exhausted', 'error', 'disabled', 'unknown']
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let disposed = false

const platformName = (value: string) => ({ gpt: 'GPT', gemini: 'Gemini', kiro: 'Kiro' }[value] || value)
const planName = (value: string) => ({ aistudio_free: 'AI Studio Free', aistudio_paid: 'AI Studio Paid', google_one_free: 'Google One Free', google_ai_pro: 'Google AI Pro', google_ai_ultra: 'Google AI Ultra', gcp_standard: 'Code Assist Standard', gcp_enterprise: 'Code Assist Enterprise', plus: 'Plus', pro: 'Pro', free: 'Free', team: 'Team', business: 'Business', enterprise: 'Enterprise' }[value.toLowerCase()] || value || t('admin.accountReport.unknownPlan'))
const typeName = (value: string) => ({ oauth: 'OAuth', apikey: 'API Key', kiro_api_key: 'Kiro API Key', social: t('admin.accountReport.socialLogin'), idc: t('admin.accountReport.organizationLogin'), service_account: t('admin.accountReport.serviceAccount') }[value.toLowerCase()] || value || t('admin.accountReport.unknownType'))
const date = (value: string | null) => value ? new Date(value).toLocaleString(locale.value) : t('admin.accountReport.notCollected')
const statusClass = (value: string) => value === 'available' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300' : value === 'error' || value === 'exhausted' ? 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
const headlineStats = computed(() => [
  { key: 'total', value: report.value?.counts.total ?? 0 },
  { key: 'available', value: report.value?.counts.available ?? 0 },
  { key: 'abnormal', value: (report.value?.counts.error ?? 0) + (report.value?.counts.limited ?? 0) },
  { key: 'exhausted', value: report.value?.counts.exhausted ?? 0 }
])
const plans = computed(() => [...new Set((report.value?.accounts ?? []).filter(a => !platform.value || a.platform === platform.value).map(a => a.plan))].sort())
const filtered = computed(() => (report.value?.accounts ?? []).filter(a =>
  (!platform.value || a.platform === platform.value) &&
  (!plan.value || (plan.value === '__unknown__' ? !a.plan : a.plan === plan.value)) &&
  (!instance.value || a.locations.some(location => location.id === instance.value)) &&
  (!status.value || a.status === status.value) &&
  (!search.value.trim() || `${a.name} ${a.type} ${a.plan} ${planName(a.plan)} ${typeName(a.type)}`.toLowerCase().includes(search.value.trim().toLowerCase()))
))
const hasFilters = computed(() => !!(search.value || platform.value || plan.value || instance.value || status.value))
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / 50)))
const paged = computed(() => filtered.value.slice((page.value - 1) * 50, page.value * 50))
watch([search, platform, plan, instance, status], () => { page.value = 1 })
watch(platform, () => { plan.value = '' })
watch(pages, value => { page.value = Math.min(page.value, value) })
function clearFilters() { search.value = ''; platform.value = ''; plan.value = ''; instance.value = ''; status.value = '' }

async function load() {
  if (loading.value || disposed) return
  clearTimeout(timer)
  loading.value = true
  controller = new AbortController()
  try {
    report.value = await getAccountReport(controller.signal)
    error.value = false
  } catch {
    if (!disposed) error.value = true
  } finally {
    loading.value = false
    if (!disposed) timer = setTimeout(load, report.value?.pending ? 3000 : 60000)
  }
}
onMounted(load)
onUnmounted(() => { disposed = true; clearTimeout(timer); controller?.abort() })
</script>
