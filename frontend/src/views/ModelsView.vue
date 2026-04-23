<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useModelsStore } from '../stores/models'
import ModelCard from '../components/ModelCard.vue'
import ModelEditor from '../components/ModelEditor.vue'
import type { Model } from '../stores/models'

const modelsStore = useModelsStore()
const { t } = useI18n()

const showEditor = ref(false)
const editingModel = ref<Model | null>(null)

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
</script>

<template>
  <div class="models-page">
    <div class="page-header">
      <h2 class="page-title">{{ $t('settings.models') }}</h2>
      <button @click="openAddEditor" class="btn btn-add">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
          <line x1="12" y1="5" x2="12" y2="19" />
          <line x1="5" y1="12" x2="19" y2="12" />
        </svg>
        {{ $t('settings.addModel') }}
      </button>
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
  </div>
</template>

<style scoped>
.models-page {
  padding: 28px;
  color: var(--text);
  height: 100%;
  overflow-y: auto;
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
