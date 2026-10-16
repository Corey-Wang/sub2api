<template>
  <div class="space-y-1" :class="{ 'opacity-60': stale }" data-testid="quota-progress">
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs">
      <span class="text-gray-500 dark:text-gray-400">
        {{ t(`admin.accountReport.quota.${quota.key}`) }}
        <span v-if="quota.estimated" class="ml-1">· {{ t('admin.accountReport.estimated') }}</span>
      </span>
      <span class="font-medium tabular-nums" :class="stale ? 'text-gray-400' : 'text-gray-900 dark:text-gray-100'">
        {{ quota.percent.toFixed(1) }}%
        <span v-if="stale"> · {{ t('admin.accountReport.stale') }}</span>
      </span>
    </div>
    <div role="progressbar" :aria-label="t(`admin.accountReport.quota.${quota.key}`)" :aria-valuenow="Math.min(100, quota.percent)" aria-valuemin="0" aria-valuemax="100" class="h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
      <div class="h-full rounded-full transition-all" :class="stale ? 'bg-gray-400' : quota.percent >= 100 ? 'bg-red-500' : quota.percent >= 80 ? 'bg-amber-500' : 'bg-primary-500'" :style="{ width: `${Math.min(100, Math.max(0, quota.percent))}%` }" />
    </div>
    <div v-if="quota.used !== null && quota.limit !== null" class="text-xs tabular-nums text-gray-500 dark:text-gray-400">
      {{ number(quota.used) }} / {{ number(quota.limit) }} {{ t(`admin.accountReport.unit.${quota.unit}`) }}
    </div>
    <div v-if="'count' in quota" class="text-xs text-gray-500 dark:text-gray-400">
      {{ t(quota.unit === 'percent' ? 'admin.accountReport.average' : 'admin.accountReport.coverage', { count: quota.count }) }}
    </div>
    <div v-if="'reset_at' in quota && quota.reset_at" class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.accountReport.resetsAt') }} {{ date(quota.reset_at) }}
    </div>
    <div v-if="'updated_at' in quota && quota.updated_at" class="text-xs text-gray-400">
      {{ t('admin.accountReport.observedAt') }} {{ date(quota.updated_at) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ReportQuota, ReportQuotaSummary } from '@/api/admin/accountReport'

const props = defineProps<{ quota: ReportQuota | ReportQuotaSummary }>()
const { t, locale } = useI18n()
const stale = computed(() => 'stale' in props.quota && props.quota.stale)
const number = (value: number) => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 2 }).format(value)
const date = (value: string) => new Date(value).toLocaleString(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
</script>
