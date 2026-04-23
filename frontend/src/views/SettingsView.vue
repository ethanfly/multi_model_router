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
        <h3 class="section-title">Models</h3>
        <button @click="openAddEditor" class="btn btn-add">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
            <line x1="12" y1="5" x2="12" y2="19" />
            <line x1="5" y1="12" x2="19" y2="12" />
          </svg>
          Add Model
        </button>
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
      <h3 class="section-title">Proxy Configuration</h3>
      <div class="proxy-card">
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
            :class="['btn', proxyRunning ? 'btn-stop' : 'btn-start']"
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
  padding: 28px;
  color: var(--text);
  max-width: 960px;
  margin: 0 auto;
}

.page-title {
  margin: 0 0 28px 0;
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--text), var(--text-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.section {
  margin-bottom: 36px;
}

/* Section title with gradient underline */
.section-title {
  margin: 0 0 16px 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-secondary);
  padding-bottom: 10px;
  position: relative;
}

.section-title::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 0;
  width: 48px;
  height: 3px;
  border-radius: 2px;
  background: linear-gradient(90deg, var(--primary), var(--accent));
}

.section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.section-header .section-title {
  margin: 0;
}

/* Dashed add button with gradient hover */
.btn-add {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 18px;
  border: 2px dashed var(--border);
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-add:hover {
  border-color: var(--primary);
  color: var(--primary);
  background: rgba(59, 130, 246, 0.06);
  border-style: solid;
}

/* Glass proxy card */
.proxy-card {
  background: rgba(30, 41, 59, 0.6);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(71, 85, 105, 0.4);
  border-radius: var(--radius);
  padding: 20px;
}

/* Buttons */
.btn {
  padding: 8px 18px;
  border: none;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-start {
  background: linear-gradient(135deg, var(--success), #059669);
  color: white;
  box-shadow: 0 2px 10px rgba(16, 185, 129, 0.25);
}

.btn-start:hover:not(:disabled) {
  box-shadow: 0 4px 16px rgba(16, 185, 129, 0.35);
  transform: translateY(-1px);
}

.btn-stop {
  background: linear-gradient(135deg, var(--error), #dc2626);
  color: white;
  box-shadow: 0 2px 10px rgba(239, 68, 68, 0.25);
}

.btn-stop:hover:not(:disabled) {
  box-shadow: 0 4px 16px rgba(239, 68, 68, 0.35);
  transform: translateY(-1px);
}

.btn-secondary {
  background-color: var(--surface-light);
  color: var(--text-secondary);
  border: 1px solid var(--border);
}

.btn-secondary:hover:not(:disabled) {
  background-color: var(--border);
  color: var(--text);
}

.loading, .no-data {
  color: var(--text-muted);
  padding: 24px;
  text-align: center;
}

.model-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.proxy-row {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  flex-wrap: wrap;
}

.proxy-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.proxy-field label {
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}

.proxy-field input {
  background-color: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px 12px;
  width: 110px;
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.proxy-field input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.proxy-field input:disabled {
  opacity: 0.5;
}

.proxy-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  padding-bottom: 6px;
  color: var(--text-secondary);
}

.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  transition: all 0.3s ease;
}

.status-dot.active {
  background-color: var(--success);
  box-shadow: 0 0 8px var(--success);
  animation: status-pulse 2s ease-in-out infinite;
}

.status-dot.inactive {
  background-color: var(--text-muted);
}

@keyframes status-pulse {
  0%, 100% { box-shadow: 0 0 4px var(--success); }
  50% { box-shadow: 0 0 12px var(--success); }
}

.proxy-url {
  margin-top: 14px;
  font-size: 13px;
  color: var(--text-muted);
}

.proxy-url code {
  background-color: var(--bg);
  padding: 4px 10px;
  border-radius: 6px;
  color: var(--primary);
  border: 1px solid rgba(71, 85, 105, 0.3);
}
</style>
