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
          <div v-for="p in report.platforms" :key="p.platform" class="card flex flex-col items-stretch justify-start space-y-4 p-5" :data-testid="`platform-${p.platform}`">
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
          </div>
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

        <p class="text-xs leading-relaxed text-gray-400">{{ t('admin.accountReport.quotaNote') }} {{ t('admin.accountReport.identityNote') }}</p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import QuotaProgress from '@/components/admin/account-report/QuotaProgress.vue'
import { getAccountReport, type AccountReport } from '@/api/admin/accountReport'

const { t, locale } = useI18n()
const report = ref<AccountReport | null>(null)
const loading = ref(false)
const error = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let disposed = false

const platformName = (value: string) => ({ gpt: 'GPT', gemini: 'Gemini', kiro: 'Kiro' }[value] || value)
const planName = (value: string) => ({ aistudio_free: 'AI Studio Free', aistudio_paid: 'AI Studio Paid', google_one_free: 'Google One Free', google_ai_pro: 'Google AI Pro', google_ai_ultra: 'Google AI Ultra', gcp_standard: 'Code Assist Standard', gcp_enterprise: 'Code Assist Enterprise', plus: 'Plus', pro: 'Pro', free: 'Free', team: 'Team', business: 'Business', enterprise: 'Enterprise' }[value.toLowerCase()] || value || t('admin.accountReport.unknownPlan'))
const date = (value: string | null) => value ? new Date(value).toLocaleString(locale.value) : t('admin.accountReport.notCollected')
const headlineStats = computed(() => [
  { key: 'total', value: report.value?.counts.total ?? 0 },
  { key: 'available', value: report.value?.counts.available ?? 0 },
  { key: 'abnormal', value: (report.value?.counts.error ?? 0) + (report.value?.counts.limited ?? 0) },
  { key: 'exhausted', value: report.value?.counts.exhausted ?? 0 }
])

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
