<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.encryptedImportTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="encrypted-import-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.encryptedImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
      >
        {{ t('admin.accounts.encryptedImportWarning') }}
      </div>

      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
        <div class="mb-2 text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.encryptedImportPublicKey') }}
        </div>
        <div v-if="!publicKeyInfo?.configured" class="text-sm text-red-600 dark:text-red-400">
          {{ t('admin.accounts.encryptedImportKeyMissing') }}
        </div>
        <template v-else>
          <div class="mb-1 font-mono text-xs text-gray-500 dark:text-dark-400">
            kid {{ publicKeyInfo.kid }}
          </div>
          <div class="flex items-start gap-2">
            <textarea
              class="input-field min-h-[4.5rem] flex-1 font-mono text-xs"
              readonly
              :value="publicKeyInfo.public_key"
            />
            <button type="button" class="btn btn-secondary shrink-0" @click="copyPublicKey">
              {{ t('common.copy') }}
            </button>
          </div>
        </template>
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.encryptedImportFile') }}</label>
        <div
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 transition-colors"
          :class="dragActive
            ? 'border-primary-400 bg-primary-50/70 dark:border-primary-500 dark:bg-primary-900/20'
            : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
          @dragenter.prevent="handleDragEnter"
          @dragover.prevent
          @dragleave.prevent="handleDragLeave"
          @drop.prevent="handleDrop"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200">
              {{ selectedFileLabel || t('admin.accounts.encryptedImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">JSON (.json)</div>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
            {{ t('common.chooseFile') }}
          </button>
        </div>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          accept="application/json,.json"
          @change="handleFileChange"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.encryptedImportPaste') }}</label>
        <textarea
          v-model="pasteContent"
          class="input-field min-h-[8rem] font-mono text-xs"
          :placeholder="t('admin.accounts.encryptedImportPastePlaceholder')"
        />
      </div>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.encryptedImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.encryptedImportResultSummary', result) }}
        </div>
        <div v-if="result.items?.length" class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
          <div v-for="item in result.items" :key="item.index" class="whitespace-pre-wrap">
            #{{ item.index }} {{ item.name || '-' }} — {{ item.action }}{{ item.message ? ` (${item.message})` : '' }}
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          type="submit"
          form="encrypted-import-form"
          :disabled="importing || !publicKeyInfo?.configured"
        >
          {{ importing ? t('admin.accounts.encryptedImporting') : t('admin.accounts.encryptedImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { CodexSessionImportResult, EncryptedAccountImportPublicKey } from '@/types'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const file = ref<File | null>(null)
const pasteContent = ref('')
const dragDepth = ref(0)
const dragActive = computed(() => dragDepth.value > 0)
const hasImported = ref(false)
const result = ref<CodexSessionImportResult | null>(null)
const publicKeyInfo = ref<EncryptedAccountImportPublicKey | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

const selectedFileLabel = computed(() => file.value?.name || '')

watch(
  () => props.show,
  async (open) => {
    if (!open) return
    file.value = null
    pasteContent.value = ''
    dragDepth.value = 0
    hasImported.value = false
    result.value = null
    if (fileInput.value) fileInput.value.value = ''
    try {
      publicKeyInfo.value = await adminAPI.accounts.getEncryptedImportPublicKey()
    } catch (error: any) {
      publicKeyInfo.value = { configured: false, kind: '', version: 1, alg: '' }
      appStore.showError(error?.message || t('admin.accounts.encryptedImportKeyMissing'))
    }
  },
  { immediate: true }
)

const openFilePicker = () => fileInput.value?.click()

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFile(target.files?.[0] || null)
  target.value = ''
}

const setSelectedFile = (sourceFile: File | null) => {
  if (importing.value) return
  file.value = sourceFile
  result.value = null
}

const handleDragEnter = () => {
  if (!importing.value) dragDepth.value += 1
}

const handleDragLeave = () => {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

const handleDrop = (event: DragEvent) => {
  dragDepth.value = 0
  if (importing.value) return
  setSelectedFile(event.dataTransfer?.files?.[0] || null)
}

const handleClose = () => {
  if (importing.value) return
  if (hasImported.value) {
    hasImported.value = false
    emit('imported')
  }
  emit('close')
}

const copyPublicKey = async () => {
  const value = publicKeyInfo.value?.public_key
  if (!value) return
  try {
    await navigator.clipboard.writeText(value)
    appStore.showSuccess(t('admin.accounts.encryptedImportPublicKeyCopied'))
  } catch {
    appStore.showError(t('admin.accounts.encryptedImportCopyFailed'))
  }
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') return sourceFile.text()
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const handleImport = async () => {
  const contents: string[] = []
  if (file.value) {
    try {
      contents.push(await readFileAsText(file.value))
    } catch {
      appStore.showError(t('admin.accounts.encryptedImportParseFailed'))
      return
    }
  }
  if (pasteContent.value.trim()) {
    contents.push(pasteContent.value.trim())
  }
  if (!contents.length) {
    appStore.showError(t('admin.accounts.encryptedImportSelectFile'))
    return
  }

  importing.value = true
  try {
    const res = await adminAPI.accounts.importEncryptedAccounts({
      contents,
      skip_default_group_bind: true,
      update_existing: true
    })
    result.value = res
    const createdOrUpdated = (res.created || 0) + (res.updated || 0)
    if (createdOrUpdated > 0) hasImported.value = true
    const params = {
      created: res.created,
      updated: res.updated,
      skipped: res.skipped,
      failed: res.failed
    }
    if (res.failed > 0) {
      appStore.showError(t('admin.accounts.encryptedImportCompletedWithErrors', params))
    } else {
      appStore.showSuccess(t('admin.accounts.encryptedImportSuccess', params))
      emit('imported')
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.encryptedImportFailed'))
  } finally {
    importing.value = false
  }
}
</script>
