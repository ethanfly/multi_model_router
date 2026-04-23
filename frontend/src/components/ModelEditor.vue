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
/* Backdrop: dark with blur */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background-color: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  animation: overlay-fade 0.2s ease;
}

@keyframes overlay-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

/* Modal card */
.modal {
  background-color: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 28px;
  width: 480px;
  max-height: 90vh;
  overflow-y: auto;
  color: var(--text);
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.4);
  animation: modal-enter 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes modal-enter {
  from {
    opacity: 0;
    transform: translateY(12px) scale(0.97);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.modal h3 {
  margin: 0 0 20px 0;
  font-size: 20px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--text), var(--text-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.editor-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-row label {
  font-size: 12px;
  color: var(--text-muted);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

/* Dark bg inputs with primary border on focus */
.form-row input[type="text"],
.form-row input[type="password"],
.form-row input[type="number"],
.form-row select {
  background-color: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 10px 12px;
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.form-row input:focus,
.form-row select:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.form-row input::placeholder {
  color: var(--text-muted);
}

.scores-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px 0;
  border-top: 1px solid rgba(71, 85, 105, 0.4);
}

.section-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.score-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.score-label {
  width: 100px;
  font-size: 12px;
  color: var(--text-muted);
  text-align: right;
  flex-shrink: 0;
}

/* Styled range sliders */
.score-row input[type="range"] {
  flex: 1;
  -webkit-appearance: none;
  appearance: none;
  height: 6px;
  border-radius: 3px;
  background: var(--surface-light);
  outline: none;
  transition: background 0.2s ease;
}

.score-row input[type="range"]::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: linear-gradient(135deg, var(--primary), var(--accent));
  cursor: pointer;
  border: 2px solid var(--surface);
  box-shadow: 0 2px 6px rgba(59, 130, 246, 0.3);
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.score-row input[type="range"]::-webkit-slider-thumb:hover {
  transform: scale(1.15);
  box-shadow: 0 2px 10px rgba(59, 130, 246, 0.4);
}

.score-val {
  width: 24px;
  text-align: center;
  font-size: 13px;
  color: var(--text-secondary);
  flex-shrink: 0;
  font-weight: 600;
}

.limits-row {
  display: flex;
  gap: 14px;
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
  color: var(--text);
  cursor: pointer;
  text-transform: none;
  letter-spacing: normal;
  font-weight: normal;
}

.checkbox-row input[type="checkbox"] {
  width: 18px;
  height: 18px;
  accent-color: var(--primary);
  cursor: pointer;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 12px;
  border-top: 1px solid rgba(71, 85, 105, 0.4);
}

.btn {
  padding: 10px 24px;
  border: none;
  border-radius: var(--radius-sm);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-cancel {
  background-color: var(--surface-light);
  color: var(--text-secondary);
}

.btn-cancel:hover {
  background-color: var(--border);
  color: var(--text);
}

/* Gradient save button */
.btn-save {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #ffffff;
  box-shadow: 0 2px 12px rgba(59, 130, 246, 0.25);
}

.btn-save:hover {
  box-shadow: 0 4px 20px rgba(59, 130, 246, 0.4);
  transform: translateY(-1px);
}

.btn-save:active {
  transform: translateY(0);
}
</style>
