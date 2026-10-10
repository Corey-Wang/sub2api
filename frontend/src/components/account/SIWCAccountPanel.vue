<template>
  <div class="space-y-4">
    <div
      v-if="mode === 'create' || view === 'create'"
      class="rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950/40 dark:text-amber-100"
    >
      <p class="font-medium">{{ t('admin.accounts.siwc.warning') }}</p>
      <p class="mt-1 text-xs leading-5">{{ t('admin.accounts.siwc.warningDetail') }}</p>
    </div>

    <div v-if="view === 'manage'" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
      <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.siwc.statusTitle') }}</p>
      <dl class="grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
        <div>
          <dt class="text-gray-400">{{ t('admin.accounts.siwc.email') }}</dt>
          <dd>{{ display.email || '—' }}</dd>
        </div>
        <div>
          <dt class="text-gray-400">{{ t('admin.accounts.siwc.expiresAt') }}</dt>
          <dd>{{ display.expiresAt || '—' }}</dd>
        </div>
        <div class="sm:col-span-2">
          <dt class="text-gray-400">{{ t('admin.accounts.siwc.scopes') }}</dt>
          <dd class="break-all">{{ display.scopes || '—' }}</dd>
        </div>
        <div class="sm:col-span-2">
          <dt class="text-gray-400">{{ t('admin.accounts.siwc.clientId') }}</dt>
          <dd class="break-all font-mono">{{ display.clientId || '—' }}</dd>
        </div>
      </dl>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" @click="view = 'reauth'">
          {{ t('admin.accounts.siwc.reauthorize') }}
        </button>
        <button type="button" class="btn btn-secondary btn-sm text-red-600" :disabled="busy" @click="revoke">
          {{ confirmRevoke ? t('admin.accounts.siwc.revokeConfirm') : t('admin.accounts.siwc.revoke') }}
        </button>
      </div>
    </div>

    <div v-else class="space-y-3">
      <div>
        <label class="input-label">{{ t('admin.accounts.siwc.redirectUri') }}</label>
        <input v-model="redirectUri" type="text" class="input font-mono text-xs" />
        <p class="input-hint">{{ t('admin.accounts.siwc.redirectHint') }}</p>
      </div>
      <label v-if="view === 'reauth'" class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input v-model="reconsent" type="checkbox" class="rounded border-gray-300" />
        {{ t('admin.accounts.siwc.reconsent') }}
      </label>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="generate">
          {{ busy && !callbackUrl ? t('admin.accounts.siwc.generating') : t('admin.accounts.siwc.generate') }}
        </button>
        <a
          v-if="authUrl"
          :href="authUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-primary btn-sm"
        >
          {{ t('admin.accounts.siwc.open') }}
        </a>
        <button v-if="authUrl" type="button" class="btn btn-secondary btn-sm" @click="copyUrl">
          {{ copied ? t('admin.accounts.siwc.copied') : t('admin.accounts.siwc.copy') }}
        </button>
      </div>
      <p v-if="authUrl" class="break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
        {{ authUrl }}
      </p>
      <div>
        <label class="input-label">{{ t('admin.accounts.siwc.callbackUrl') }}</label>
        <textarea
          v-model="callbackUrl"
          rows="3"
          class="input font-mono text-xs"
          :placeholder="t('admin.accounts.siwc.callbackPlaceholder')"
        ></textarea>
      </div>
      <button type="button" class="btn btn-primary btn-sm" :disabled="busy || !callbackUrl.trim()" @click="exchange">
        {{ busy ? t('admin.accounts.siwc.exchanging') : t('admin.accounts.siwc.exchange') }}
      </button>
    </div>

    <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
    <p v-if="success" class="text-sm text-green-600">{{ success }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'

const props = withDefaults(
  defineProps<{
    mode: 'create' | 'reauth' | 'manage'
    accountId?: number
    proxyId?: number | null
    credentials?: Record<string, unknown> | null
  }>(),
  {
    accountId: 0,
    proxyId: null,
    credentials: null
  }
)

const emit = defineEmits<{
  credentials: [value: Record<string, unknown>]
  authorized: []
  revoked: []
}>()

const { t } = useI18n()
const view = ref(props.mode)
const redirectUri = ref('http://127.0.0.1:1455/auth/callback')
const reconsent = ref(false)
const authUrl = ref('')
const sessionId = ref('')
const callbackUrl = ref('')
const busy = ref(false)
const error = ref('')
const success = ref('')
const copied = ref(false)
const confirmRevoke = ref(false)
const localStatus = ref<{ email?: string; scopes?: string; expiresAt?: string; clientId?: string }>({})

watch(
  () => props.mode,
  (mode) => {
    view.value = mode
  }
)

function textCred(key: string): string {
  const value = props.credentials?.[key]
  return typeof value === 'string' ? value : ''
}

const display = computed(() => ({
  email: localStatus.value.email || textCred('email'),
  scopes: localStatus.value.scopes || textCred('scopes'),
  expiresAt: localStatus.value.expiresAt || textCred('expires_at'),
  clientId: localStatus.value.clientId || textCred('client_id')
}))

function errorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'message' in err && typeof (err as { message: unknown }).message === 'string') {
    return (err as { message: string }).message
  }
  return t('admin.accounts.siwc.requestFailed')
}

async function generate() {
  error.value = ''
  success.value = ''
  copied.value = false
  busy.value = true
  try {
    const result = await adminAPI.accounts.generateSIWCAuthURL({
      proxy_id: props.proxyId ?? undefined,
      redirect_uri: redirectUri.value.trim(),
      account_id: props.mode === 'reauth' || view.value === 'reauth' ? props.accountId : undefined,
      reconsent: reconsent.value
    })
    authUrl.value = result.auth_url
    sessionId.value = result.session_id
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    busy.value = false
  }
}

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(authUrl.value)
    copied.value = true
  } catch {
    copied.value = false
  }
}

async function exchange() {
  if (!sessionId.value) {
    error.value = t('admin.accounts.siwc.missingSession')
    return
  }
  error.value = ''
  success.value = ''
  busy.value = true
  try {
    const result = await adminAPI.accounts.exchangeSIWCCallback({
      session_id: sessionId.value,
      callback_url: callbackUrl.value.trim()
    })
    localStatus.value = {
      email: result.email,
      scopes: result.scopes,
      expiresAt: result.expires_at,
      clientId: result.client_id
    }
    success.value = t('admin.accounts.siwc.authorized')
    if (props.mode === 'create') {
      emit('credentials', result.credentials || {})
    } else {
      emit('authorized')
      if (props.mode === 'manage') {
        view.value = 'manage'
      }
    }
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    busy.value = false
  }
}

async function revoke() {
  if (!confirmRevoke.value) {
    confirmRevoke.value = true
    return
  }
  error.value = ''
  success.value = ''
  busy.value = true
  try {
    await adminAPI.accounts.revokeSIWCAccount(props.accountId)
    localStatus.value = { ...display.value, expiresAt: '' }
    success.value = t('admin.accounts.siwc.revoked')
    confirmRevoke.value = false
    emit('revoked')
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    busy.value = false
  }
}
</script>
