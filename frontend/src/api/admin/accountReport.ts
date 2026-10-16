import { apiClient } from '../client'

export type ReportPlatform = 'gpt' | 'gemini' | 'kiro'
export type ReportStatus = 'available' | 'limited' | 'error' | 'disabled' | 'exhausted' | 'unknown'
export interface ReportCounts {
  total: number
  available: number
  limited: number
  error: number
  disabled: number
  exhausted: number
  unknown: number
  unknown_quota: number
  near_limit: number
  identity_uncertain: number
}
export interface ReportQuota {
  key: string
  unit: 'percent' | 'requests' | 'credits'
  used: number | null
  limit: number | null
  percent: number
  reset_at: string | null
  updated_at: string | null
  estimated: boolean
  stale: boolean
}
export interface ReportLocation {
  id: string
  name: string
  account_id: string
  status: ReportStatus
  updated_at: string
  stale: boolean
}
export interface ReportAccount {
  id: string
  platform: ReportPlatform
  name: string
  plan: string
  type: string
  status: ReportStatus
  locations: ReportLocation[]
  quotas: ReportQuota[]
  quota_error: boolean
  identity_uncertain: boolean
}
export interface ReportSource {
  id: string
  name: string
  platform: ReportPlatform
  updated_at: string | null
  attempted_at: string
  error: string
  stale: boolean
}
export interface ReportQuotaSummary {
  key: string
  plan: string
  unit: ReportQuota['unit']
  count: number
  percent: number
  used: number | null
  limit: number | null
  estimated: boolean
}
export interface AccountReport {
  updated_at: string | null
  pending: boolean
  complete: boolean
  counts: ReportCounts
  sources: ReportSource[]
  accounts: ReportAccount[]
  platforms: {
    platform: ReportPlatform
    counts: ReportCounts
    plans: { name: string; count: number }[]
    quotas: ReportQuotaSummary[]
  }[]
}

export async function getAccountReport(signal?: AbortSignal): Promise<AccountReport> {
  const { data } = await apiClient.get<AccountReport>('/admin/account-report', { signal })
  return data
}
