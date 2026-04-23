<script lang="ts" setup>
import { ref } from 'vue'
import type { Model } from '../stores/models'

const props = defineProps<{
  model: Model | null
}>()

const emit = defineEmits<{
  save: [model: Model]
  cancel: []
}>()

function createEmpty(): Model {
  return {
    id: '',
    name: '',
    provider: 'openai',
    baseUrl: '',
    apiKey: '',
    modelId: '',
    reasoning: 5,
    coding: 5,
    creativity: 5,
    speed: 5,
    costEfficiency: 5,
    maxRpm: 0,
    maxTpm: 0,
    isActive: true,
  }
}

const form = ref<Model>(props.model ? { ...props.model } : createEmpty())
const isEditing = ref(!!props.model)

const providers = ['openai', 'anthropic', 'google', 'ollama', 'other']

const scores: { key: keyof Model; label: string }[] = [
  { key: 'reasoning', label: 'Reasoning' },
  { key: 'coding', label: 'Coding' },
  { key: 'creativity', label: 'Creativity' },
  { key: 'speed', label: 'Speed' },
  { key: 'costEfficiency', label: 'Cost Efficiency' },
]

function handleSubmit() {
  if (!form.value.name.trim() || !form.value.modelId.trim()) return
  emit('save', { ...form.value })
}

function handleOverlayClick(e: MouseEvent) {
  if ((e.target as HTMLElement).classList.contains('modal-overlay')) {
    emit('cancel')
  }
}
</script>

<template>
  <div class="modal-overlay" @click="handleOverlayClick">
    <div class="modal">
      <h3>{{ isEditing ? 'Edit Model' : 'Add Model' }}</h3>

      <form @submit.prevent="handleSubmit" class="editor-form">
        <div class="form-row">
          <label>Name *</label>
          <input v-model="form.name" required placeholder="e.g. GPT-4o" />
        </div>

        <div class="form-row">
          <label>Provider</label>
          <select v-model="form.provider">
            <option v-for="p in providers" :key="p" :value="p">{{ p }}</option>
          </select>
        </div>

        <div class="form-row">
          <label>Base URL</label>
          <input v-model="form.baseUrl" placeholder="https://api.openai.com/v1" />
        </div>

        <div class="form-row">
          <label>API Key</label>
          <input v-model="form.apiKey" type="password" placeholder="sk-..." />
        </div>

        <div class="form-row">
          <label>Model ID *</label>
          <input v-model="form.modelId" required placeholder="gpt-4o" />
        </div>

        <div class="scores-section">
          <label class="section-label">Scores (1-10)</label>
          <div v-for="s in scores" :key="s.key" class="score-row">
            <span class="score-label">{{ s.label }}</span>
            <input
              type="range"
              min="1"
              max="10"
              :value="(form[s.key] as number)"
              @input="((form as any)[s.key] = parseInt(($event.target as HTMLInputElement).value, 10))"
            />
            <span class="score-val">{{ form[s.key] }}</span>
          </div>
        </div>

        <div class="limits-row">
          <div class="form-row inline">
            <label>Max RPM</label>
            <input v-model.number="form.maxRpm" type="number" min="0" placeholder="0 = unlimited" />
          </div>
          <div class="form-row inline">
            <label>Max TPM</label>
            <input v-model.number="form.maxTpm" type="number" min="0" placeholder="0 = unlimited" />
          </div>
        </div>

        <div class="form-row checkbox-row">
          <label>
            <input type="checkbox" v-model="form.isActive" />
            Active
          </label>
        </div>

        <div class="form-actions">
          <button type="button" @click="emit('cancel')" class="btn btn-cancel">Cancel</button>
          <button type="submit" class="btn btn-save">Save</button>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background-color: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background-color: #1f2937;
  border: 1px solid #374151;
  border-radius: 12px;
  padding: 24px;
  width: 480px;
  max-height: 90vh;
  overflow-y: auto;
  color: #e5e7eb;
}

.modal h3 {
  margin: 0 0 16px 0;
  font-size: 18px;
  font-weight: 600;
}

.editor-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-row label {
  font-size: 12px;
  color: #9ca3af;
  font-weight: 500;
}

.form-row input[type="text"],
.form-row input[type="password"],
.form-row input[type="number"],
.form-row select {
  background-color: #374151;
  color: #e5e7eb;
  border: 1px solid #4b5563;
  border-radius: 6px;
  padding: 8px 10px;
  font-size: 14px;
  outline: none;
}

.form-row input:focus,
.form-row select:focus {
  border-color: #3b82f6;
}

.scores-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 0;
  border-top: 1px solid #374151;
}

.section-label {
  font-size: 13px;
  font-weight: 600;
  color: #d1d5db;
  margin-bottom: 4px;
}

.score-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.score-label {
  width: 100px;
  font-size: 12px;
  color: #9ca3af;
  text-align: right;
  flex-shrink: 0;
}

.score-row input[type="range"] {
  flex: 1;
  accent-color: #3b82f6;
}

.score-val {
  width: 20px;
  text-align: center;
  font-size: 13px;
  color: #d1d5db;
  flex-shrink: 0;
}

.limits-row {
  display: flex;
  gap: 12px;
}

.form-row.inline {
  flex: 1;
}

.checkbox-row {
  flex-direction: row;
  align-items: center;
}

.checkbox-row label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #e5e7eb;
  cursor: pointer;
}

.checkbox-row input[type="checkbox"] {
  accent-color: #3b82f6;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid #374151;
}

.btn {
  padding: 8px 20px;
  border: none;
  border-radius: 6px;
  font-size: 14px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.btn-cancel {
  background-color: #374151;
  color: #e5e7eb;
}

.btn-cancel:hover {
  background-color: #4b5563;
}

.btn-save {
  background-color: #3b82f6;
  color: #ffffff;
}

.btn-save:hover {
  background-color: #2563eb;
}
</style>
