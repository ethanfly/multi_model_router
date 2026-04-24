<script lang="ts" setup>
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Model } from '../stores/models'

const { t } = useI18n()

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
const rpmTooLow = computed(() => form.value.maxRpm > 0 && form.value.maxRpm < 5)
const tpmTooLow = computed(() => form.value.maxTpm > 0 && form.value.maxTpm < 1000)

const providers = ['openai', 'anthropic', 'google', 'ollama', 'other']

const scoreKeys: { key: keyof Model; labelKey: string }[] = [
  { key: 'reasoning', labelKey: 'modelEditor.reasoning' },
  { key: 'coding', labelKey: 'modelEditor.coding' },
  { key: 'creativity', labelKey: 'modelEditor.creativity' },
  { key: 'speed', labelKey: 'modelEditor.speed' },
  { key: 'costEfficiency', labelKey: 'modelEditor.costEfficiency' },
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
      <h3>{{ isEditing ? $t('modelEditor.editModel') : $t('modelEditor.addModel') }}</h3>

      <form @submit.prevent="handleSubmit" class="editor-form">
        <div class="form-row">
          <label>{{ $t('modelEditor.name') }} *</label>
          <input type="text" v-model="form.name" required placeholder="e.g. GPT-4o" />
        </div>

        <div class="form-row">
          <label>{{ $t('modelEditor.provider') }}</label>
          <select v-model="form.provider">
            <option v-for="p in providers" :key="p" :value="p">{{ p }}</option>
          </select>
        </div>

        <div class="form-row">
          <label>{{ $t('modelEditor.baseUrl') }}</label>
          <input type="text" v-model="form.baseUrl" placeholder="https://api.openai.com/v1" />
        </div>

        <div class="form-row">
          <label>{{ $t('modelEditor.apiKey') }}</label>
          <input v-model="form.apiKey" type="password" autocomplete="off" placeholder="sk-..." />
        </div>

        <div class="form-row">
          <label>{{ $t('modelEditor.modelId') }} *</label>
          <input type="text" v-model="form.modelId" required placeholder="gpt-4o" />
        </div>

        <div class="scores-section">
          <label class="section-label">{{ $t('modelEditor.scores') }}</label>
          <div v-for="s in scoreKeys" :key="s.key" class="score-row">
            <span class="score-label">{{ t(s.labelKey) }}</span>
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
            <label>{{ $t('modelEditor.maxRpm') }}</label>
            <input v-model.number="form.maxRpm" type="number" min="0" :placeholder="$t('modelEditor.unlimited')" />
            <p class="field-hint">{{ $t('modelEditor.maxRpmHint') }}</p>
            <p v-if="rpmTooLow" class="field-warning">{{ $t('modelEditor.maxRpmWarning') }}</p>
          </div>
          <div class="form-row inline">
            <label>{{ $t('modelEditor.maxTpm') }}</label>
            <input v-model.number="form.maxTpm" type="number" min="0" :placeholder="$t('modelEditor.unlimited')" />
            <p class="field-hint">{{ $t('modelEditor.maxTpmHint') }}</p>
            <p v-if="tpmTooLow" class="field-warning">{{ $t('modelEditor.maxTpmWarning') }}</p>
          </div>
        </div>

        <div class="form-row checkbox-row">
          <label>
            <input type="checkbox" v-model="form.isActive" />
            {{ $t('modelEditor.active') }}
          </label>
        </div>

        <div class="form-actions">
          <button type="button" @click="emit('cancel')" class="btn btn-cancel">{{ $t('modelEditor.cancel') }}</button>
          <button type="submit" class="btn btn-save">{{ $t('modelEditor.save') }}</button>
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
  margin-bottom: 2px;
  font-size: 12px;
  color: var(--text-muted);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.field-hint,
.field-warning {
  margin: 0;
  font-size: 12px;
  line-height: 1.45;
}

.field-hint {
  color: var(--text-muted);
}

.field-warning {
  color: var(--warning);
}

/* Unified form controls */
.form-row input[type="text"],
.form-row input[type="password"],
.form-row input[type="number"],
.form-row select {
  background-color: var(--surface);
  color: var(--text);
  border: 1px solid rgba(71, 85, 105, 0.5);
  border-radius: var(--radius-sm);
  padding: 10px 14px;
  font-size: 14px;
  font-family: inherit;
  height: 40px;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.form-row input:hover,
.form-row select:hover {
  border-color: var(--border);
}

.form-row input:focus,
.form-row select:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2);
}

.form-row input::placeholder {
  color: var(--text-muted);
  opacity: 0.7;
}

/* Custom select arrow */
.form-row select {
  appearance: none;
  -webkit-appearance: none;
  padding-right: 36px;
  cursor: pointer;
  background-image: url("data:image/svg+xml,%3Csvg width='12' height='8' viewBox='0 0 12 8' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1.5L6 6.5L11 1.5' stroke='%2394A3B8' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 12px center;
  background-size: 12px 8px;
}

.form-row select:focus {
  background-image: url("data:image/svg+xml,%3Csvg width='12' height='8' viewBox='0 0 12 8' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1.5L6 6.5L11 1.5' stroke='%233B82F6' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
}

/* Hide number spinners */
.form-row input[type="number"]::-webkit-inner-spin-button,
.form-row input[type="number"]::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.form-row input[type="number"] {
  -moz-appearance: textfield;
}

/* Hide Edge password reveal */
.form-row input::-ms-reveal,
.form-row input::-ms-clear {
  display: none;
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
