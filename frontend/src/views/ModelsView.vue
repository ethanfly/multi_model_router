<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useModelsStore } from '../stores/models'
import ModelCard from '../components/ModelCard.vue'
import ModelEditor from '../components/ModelEditor.vue'
import PasswordDialog from '../components/PasswordDialog.vue'
import type { Model } from '../stores/models'
import {
  ExportModels,
  ImportModels,
  SaveExportFile,
  ReadImportFile,
} from '../../wailsjs/go/main/App'

const modelsStore = useModelsStore()
const { t } = useI18n()

const showEditor = ref(false)
const editingModel = ref<Model | null>(null)
const showPasswordDialog = ref(false)
const passwordDialogMode = ref<'export' | 'import'>('export')
const importData = ref('')
const toast = ref('')
let toastTimer: ReturnType<typeof setTimeout> | null = null

onMounted(async () => {
  try { await modelsStore.fetchModels() } catch { /* ignore */ }
})

function openAddEditor() {
  editingModel.value = null
  showEditor.value = true
}

function openEditEditor(model: Model) {
  editingModel.value = { ...model }
  showEditor.value = true
}

function closeEditor() {
  showEditor.value = false
  editingModel.value = null
}

async function handleSave(model: Model) {
  await modelsStore.save(model)
  closeEditor()
}

function showToast(msg: string) {
  toast.value = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.value = '' }, 3000)
}

async function handleExport() {
  if (modelsStore.models.length === 0) {
    showToast(t('settings.noModelsToExport'))
    return
  }
  passwordDialogMode.value = 'export'
  showPasswordDialog.value = true
}

async function handleImport() {
  try {
    const content = await ReadImportFile()
    if (!content) return
    importData.value = content
    passwordDialogMode.value = 'import'
    showPasswordDialog.value = true
  } catch {
    // User cancelled dialog
  }
}

async function handlePasswordConfirm(password: string) {
  showPasswordDialog.value = false

  if (passwordDialogMode.value === 'export') {
    try {
      const jsonData = await ExportModels(password)
      await SaveExportFile(jsonData)
      showToast(t('settings.exportSuccess'))
    } catch (err: any) {
      showToast(err?.message || String(err))
    }
  } else {
    try {
      const countStr = await ImportModels(importData.value, password)
      const count = parseInt(countStr, 10)
      showToast(t('settings.importSuccess').replace('{0}', String(count)))
    } catch (err: any) {
      showToast(t('settings.importError') + ' ' + (err?.message || String(err)))
    }
    importData.value = ''
  }
}
</script>

<template>
  <div class="models-page">
    <div class="page-header">
      <h2 class="page-title">{{ $t('settings.models') }}</h2>
      <div class="header-actions">
        <button @click="handleExport" class="btn btn-action" :title="$t('settings.exportModels')">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="7 10 12 15 17 10"/>
            <line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
          {{ $t('settings.exportModels') }}
        </button>
        <button @click="handleImport" class="btn btn-action" :title="$t('settings.importModels')">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="17 8 12 3 7 8"/>
            <line x1="12" y1="3" x2="12" y2="15"/>
          </svg>
          {{ $t('settings.importModels') }}
        </button>
        <button @click="openAddEditor" class="btn btn-add">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          {{ $t('settings.addModel') }}
        </button>
      </div>
    </div>

    <div v-if="modelsStore.loading" class="loading">{{ $t('settings.loadingModels') }}</div>
    <div v-else-if="modelsStore.models.length === 0" class="no-data">
      {{ $t('settings.noModels') }}
    </div>
    <div v-else class="model-grid">
      <ModelCard
        v-for="m in modelsStore.models"
        :key="m.id"
        :model="m"
        @edit="openEditEditor(m)"
        @delete="modelsStore.remove(m.id)"
      />
    </div>

    <ModelEditor
      v-if="showEditor"
      :model="editingModel"
      @save="handleSave"
      @cancel="closeEditor"
    />

    <PasswordDialog
      v-if="showPasswordDialog"
      :title="passwordDialogMode === 'export' ? $t('settings.exportPassword') : $t('settings.importPassword')"
      :hint="passwordDialogMode === 'export' ? $t('settings.exportPasswordHint') : $t('settings.importPasswordHint')"
      :requireConfirm="passwordDialogMode === 'export'"
      @confirm="handlePasswordConfirm"
      @cancel="showPasswordDialog = false"
    />

    <Transition name="toast">
      <div v-if="toast" class="toast">{{ toast }}</div>
    </Transition>
  </div>
</template>

<style scoped>
.models-page {
  padding: 28px;
  color: var(--text);
  height: 100%;
  overflow-y: auto;
  position: relative;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 28px;
}

.page-title {
  margin: 0;
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--text), var(--text-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.btn-action {
  display: flex;
  align-items: center;
  gap: 5px;
  padding: 8px 14px;
  border: 1px solid rgba(71, 85, 105, 0.4);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.2s ease, background 0.2s ease, border-color 0.2s ease;
}

.btn-action:hover {
  border-color: var(--primary);
  color: var(--primary);
  background: rgba(59, 130, 246, 0.06);
}

.btn-add {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 18px;
  border: 1px dashed var(--border);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: color 0.2s ease, background 0.2s ease, border-color 0.2s ease;
}

.btn-add:hover {
  border-color: var(--primary);
  color: var(--primary);
  background: rgba(59, 130, 246, 0.06);
}

.loading, .no-data {
  color: var(--text-muted);
  padding: 48px 24px;
  text-align: center;
}

.model-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.toast {
  position: fixed;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 10px 20px;
  font-size: 13px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
  z-index: 2000;
  white-space: nowrap;
}

.toast-enter-active {
  animation: toast-in 0.25s ease;
}

.toast-leave-active {
  animation: toast-in 0.2s ease reverse;
}

@keyframes toast-in {
  from {
    opacity: 0;
    transform: translateX(-50%) translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateX(-50%) translateY(0);
  }
}

@media (max-width: 900px) {
  .model-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 600px) {
  .model-grid {
    grid-template-columns: 1fr;
  }
}
</style>
