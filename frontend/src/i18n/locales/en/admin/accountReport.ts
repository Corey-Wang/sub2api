export default {
  accountReport: {
    title: 'Account report',
    description: 'Account status and current quota across GPT, Gemini and Kiro.',
    refresh: 'Refresh report',
    previous: 'Previous',
    updatedAt: 'Report updated',
    socialLogin: 'Personal account login',
    organizationLogin: 'Organization login',
    serviceAccount: 'Service account',
    noInstances: 'No Kiro instances configured',
    loadError: 'Unable to load the report. Please retry. Displayed data may be outdated.',
    pending: 'Collecting accounts for the first time. This page will update automatically.',
    incomplete: 'Some sources are pending or unavailable. Last successful observations are retained; account totals may be incomplete.',
    total: 'Known accounts', available: 'Available accounts', abnormal: 'Errors / rate limits', exhausted: 'Quota exhausted',
    accounts: 'accounts', unknownPlan: 'Unknown plan', unknownType: 'Unknown type', noQuota: 'No current quota data',
    unknownQuotaCount: '{count} accounts have unknown or stale quotas', estimated: 'Local estimate', stale: 'Outdated',
    resetsAt: 'Resets', observedAt: 'Observed', average: 'Average utilization of {count} accounts', coverage: 'Current quotas from {count} accounts',
    sourceError: 'Collection failed', synced: 'Synced', notCollected: 'Not collected',
    refreshNote: 'This page updates every minute. Kiro is collected every 5 minutes; all viewers share the same results.',
    details: 'Account details', clearFilters: 'Clear filters', search: 'Search account or plan', platform: 'Platform', plan: 'Plan', account: 'Account / type', instance: 'Instance',
    statusLabel: 'Status', progress: 'Quota progress / reset time', allPlatforms: 'All platforms', allPlans: 'All plans', allInstances: 'All instances', allStatuses: 'All statuses',
    noMatches: 'No matching accounts', noAccounts: 'No account data yet', identityUncertain: 'Identity needs review; quota excluded from totals',
    quotaError: 'Quota query failed. The last successful observation is retained.',
    quotaNote: 'Quotas are reported separately by platform. GPT uses existing observations; direct Gemini estimates from local requests and configured limits; Gemini through Antigravity and Kiro use upstream quotas. Unknown and outdated quotas are excluded from totals.',
    identityNote: 'Kiro is deduplicated by matching credentials or configured account mappings. Quotas are counted once across instances. Available means the instance permits scheduling, not that a live request has been verified.',
    status: { available: 'Available', limited: 'Rate limited / cooling down', exhausted: 'Quota exhausted', error: 'Error', disabled: 'Disabled', unknown: 'Unknown' },
    unit: { percent: '%', requests: 'requests', credits: 'credits' },
    quota: {
      antigravity_model: 'Gemini model quota',
      gpt_5h: '5-hour window', gpt_7d: '7-day window',
      gemini_shared_daily: 'Shared daily requests', gemini_pro_daily: 'Pro daily requests', gemini_flash_daily: 'Flash daily requests',
      gemini_shared_minute: 'Shared requests per minute', gemini_pro_minute: 'Pro requests per minute', gemini_flash_minute: 'Flash requests per minute',
      kiro_credits: 'Current cycle quota'
    }
  }
}
