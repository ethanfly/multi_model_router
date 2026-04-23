<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useModelsStore } from '../stores/models'
import { GetProxyStatus, StartProxy, StopProxy, GetConfig, SetConfig } from '../../wailsjs/go/main/App'
import ModelCard from '../components/ModelCard.vue'
import ModelEditor from '../components/ModelEditor.vue'
import type { Model } from '../stores/models'

const modelsStore = useModelsStore()

const showEditor = ref(false)
const editingModel = ref<Model | null>(null)

const proxyRunning = ref(false)
const proxyPort = ref(8080)
const proxyUrl = ref('')
const proxyLoading = ref(false)
const copySuccess = ref(false)

onMounted(async () => {
  await modelsStore.fetchModels()
  await loadProxyStatus()
  try {
    const port = await GetConfig('proxyPort')
    if (port) proxyPort.value = parseInt(port, 10) || 8080
  } catch { /* ignore */ }
})

async function loadProxyStatus() {
  try {
    const status = await GetProxyStatus() as any
    proxyRunning.value = status.running
    proxyPort.value = status.port || 8080
    proxyUrl.value = status.url || ''
  } catch { /* ignore */ }
}

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

async function toggleProxy() {
  proxyLoading.value = true
  try {
    if (proxyRunning.value) {
      await StopProxy()
    } else {
      await SetConfig('proxyPort', String(proxyPort.value))
      await StartProxy(proxyPort.value)
    }
    await loadProxyStatus()
  } catch (err: any) {
    alert('Proxy error: ' + (err.message || err))
  } finally {
    proxyLoading.value = false
  }
}

async function copyProxyUrl() {
  if (!proxyUrl.value) return
  try {
    await navigator.clipboard.writeText(proxyUrl.value)
    copySuccess.value = true
    setTimeout(() => { copySuccess.value = false }, 1500)
  } catch { /* ignore */ }
}
</script>

<template>
  <div class="settings">
    <h2 class="page-title">Settings</h2>

    <section class="section">
      <div class="section-header">
        <h3>Models</h3>
        <button @click="openAddEditor" class="btn btn-primary">+ Add Model</button>
      </div>

      <div v-if="modelsStore.loading" class="loading">Loading models...</div>
      <div v-else-if="modelsStore.models.length === 0" class="no-data">
        No models configured. Click "Add Model" to get started.
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
    </section>

    <section class="section">
      <h3>Proxy Configuration</h3>
      <div class="proxy-row">
        <div class="proxy-field">
          <label>Port</label>
          <input
            type="number"
            v-model.number="proxyPort"
            :disabled="proxyRunning"
            min="1"
            max="65535"
          />
        </div>
        <button
          @click="toggleProxy"
          :disabled="proxyLoading"
          :class="['btn', proxyRunning ? 'btn-danger' : 'btn-success']"
        >
          {{ proxyLoading ? '...' : proxyRunning ? 'Stop Proxy' : 'Start Proxy' }}
        </button>
        <div class="proxy-status">
          <span :class="['status-dot', proxyRunning ? 'active' : 'inactive']"></span>
          {{ proxyRunning ? 'Running' : 'Stopped' }}
        </div>
        <button
          v-if="proxyRunning && proxyUrl"
          @click="copyProxyUrl"
          class="btn btn-secondary"
        >
          {{ copySuccess ? 'Copied!' : 'Copy URL' }}
        </button>
      </div>
      <div v-if="proxyUrl" class="proxy-url">
        Proxy URL: <code>{{ proxyUrl }}</code>
      </div>
    </section>

    <ModelEditor
      v-if="showEditor"
      :model="editingModel"
      @save="handleSave"
      @cancel="closeEditor"
    />
  </div>
</template>

<style scoped>
.settings {
  padding: 24px;
  color: #e5e7eb;
  max-width: 960px;
  margin: 0 auto;
}

.page-title {
  margin: 0 0 20px 0;
  font-size: 22px;
  font-weight: 600;
}

.section {
  margin-bottom: 32px;
}

.section h3 {
  margin: 0 0 12px 0;
  font-size: 16px;
  font-weight: 600;
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.section-header h3 {
  margin: 0;
}

.btn {
  padding: 6px 16px;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  transition: opacity 0.2s;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background-color: #3b82f6;
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background-color: #2563eb;
}

.btn-success {
  background-color: #22c55e;
  color: white;
}

.btn-danger {
  background-color: #ef4444;
  color: white;
}

.btn-secondary {
  background-color: #374151;
  color: #e5e7eb;
  border: 1px solid #4b5563;
}

.btn-secondary:hover:not(:disabled) {
  background-color: #4b5563;
}

.loading, .no-data {
  color: #9ca3af;
  padding: 16px;
  text-align: center;
}

.model-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}

.proxy-row {
  display: flex;
  align-items: flex-end;
  gap: 12px;
  flex-wrap: wrap;
}

.proxy-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.proxy-field label {
  font-size: 12px;
  color: #9ca3af;
}

.proxy-field input {
  background-color: #374151;
  color: #e5e7eb;
  border: 1px solid #4b5563;
  border-radius: 6px;
  padding: 6px 10px;
  width: 100px;
  font-size: 14px;
  outline: none;
}

.proxy-field input:focus {
  border-color: #3b82f6;
}

.proxy-field input:disabled {
  opacity: 0.6;
}

.proxy-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 14px;
  padding-bottom: 4px;
}

.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}

.status-dot.active {
  background-color: #22c55e;
}

.status-dot.inactive {
  background-color: #6b7280;
}

.proxy-url {
  margin-top: 10px;
  font-size: 13px;
  color: #9ca3af;
}

.proxy-url code {
  background-color: #374151;
  padding: 2px 6px;
  border-radius: 4px;
  color: #60a5fa;
}
</style>
