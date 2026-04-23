<script lang="ts" setup>
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { TestModel } from '../../wailsjs/go/main/App'
import type { Model } from '../stores/models'

const { t } = useI18n()

const props = defineProps<{ model: Model }>()
const emit = defineEmits<{
  edit: []
  delete: []
}>()

const testing = ref(false)
const testResult = ref('')

function providerGradient(provider: string): string {
  switch (provider.toLowerCase()) {
    case 'openai': return 'linear-gradient(135deg, #22c55e, #16a34a)'
    case 'anthropic': return 'linear-gradient(135deg, #f59e0b, #d97706)'
    case 'google': return 'linear-gradient(135deg, #3b82f6, #2563eb)'
    case 'ollama': return 'linear-gradient(135deg, #8b5cf6, #7c3aed)'
    default: return 'linear-gradient(135deg, #6b7280, #4b5563)'
  }
}

const scores = computed(() => [
  { label: t('modelCard.reasoning'), value: props.model.reasoning, color: 'linear-gradient(90deg, #3b82f6, #06b6d4)' },
  { label: t('modelCard.coding'), value: props.model.coding, color: 'linear-gradient(90deg, #10b981, #34d399)' },
  { label: t('modelCard.creativity'), value: props.model.creativity, color: 'linear-gradient(90deg, #f59e0b, #fbbf24)' },
  { label: t('modelCard.speed'), value: props.model.speed, color: 'linear-gradient(90deg, #8b5cf6, #a78bfa)' },
  { label: t('modelCard.costEff'), value: props.model.costEfficiency, color: 'linear-gradient(90deg, #ec4899, #f472b6)' },
])

async function handleTest() {
  testing.value = true
  testResult.value = ''
  try {
    const result = await TestModel(props.model)
    testResult.value = result || 'OK'
  } catch (err: any) {
    testResult.value = 'Error: ' + (err.message || err)
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <div class="model-card">
    <div class="card-header">
      <div class="card-title">
        <span class="model-name">{{ model.name }}</span>
        <span class="provider-badge" :style="{ background: providerGradient(model.provider) }">
          {{ model.provider }}
        </span>
      </div>
      <span :class="['status-indicator', model.isActive ? 'active' : 'inactive']">
        <span class="status-dot-inline"></span>
        {{ model.isActive ? $t('modelCard.active') : $t('modelCard.inactive') }}
      </span>
    </div>

    <div class="scores">
      <div v-for="s in scores" :key="s.label" class="score-row">
        <span class="score-label">{{ s.label }}</span>
        <div class="score-bar-track">
          <div class="score-bar-fill" :style="{ width: (s.value * 10) + '%', background: s.color }"></div>
        </div>
        <span class="score-value">{{ s.value }}</span>
      </div>
    </div>

    <div class="card-meta">
      <span>RPM: {{ model.maxRpm || $t('modelCard.unlimited') }}</span>
    </div>

    <div v-if="testResult" :class="['test-result', { 'test-error': testResult.startsWith('Error') }]">{{ testResult }}</div>

    <div class="card-actions">
      <button @click="emit('edit')" class="btn btn-ghost">{{ $t('modelCard.edit') }}</button>
      <button @click="handleTest" :disabled="testing" class="btn btn-ghost btn-test">
        {{ testing ? '...' : $t('modelCard.test') }}
      </button>
      <button @click="emit('delete')" class="btn btn-ghost btn-delete">{{ $t('modelCard.delete') }}</button>
    </div>
  </div>
</template>

<style scoped>
.model-card {
  background-color: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.model-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.3);
  border-color: rgba(71, 85, 105, 0.6);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.model-name {
  font-weight: 600;
  font-size: 15px;
  color: var(--text);
}

/* Gradient provider badge */
.provider-badge {
  padding: 2px 10px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
  letter-spacing: 0.3px;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  white-space: nowrap;
  color: var(--text-muted);
}

.status-dot-inline {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-indicator.active .status-dot-inline {
  background-color: var(--success);
  box-shadow: 0 0 6px var(--success);
  animation: dot-pulse 2s ease-in-out infinite;
}

.status-indicator.inactive .status-dot-inline {
  background-color: var(--text-muted);
}

@keyframes dot-pulse {
  0%, 100% { box-shadow: 0 0 4px var(--success); }
  50% { box-shadow: 0 0 10px var(--success); }
}

.status-indicator.active {
  color: var(--success);
}

.scores {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.score-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.score-label {
  width: 70px;
  font-size: 12px;
  color: var(--text-muted);
  text-align: right;
  flex-shrink: 0;
}

.score-bar-track {
  flex: 1;
  height: 8px;
  background-color: var(--surface-light);
  border-radius: 4px;
  overflow: hidden;
}

.score-bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.5s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}

.score-bar-fill::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.15), transparent);
  animation: bar-shimmer 2.5s infinite;
}

@keyframes bar-shimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

.score-value {
  width: 20px;
  font-size: 12px;
  color: var(--text-secondary);
  text-align: center;
  flex-shrink: 0;
  font-weight: 600;
}

.card-meta {
  font-size: 12px;
  color: var(--text-muted);
}

.test-result {
  font-size: 12px;
  color: var(--text-muted);
  background: var(--surface);
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  word-break: break-all;
  border: 1px solid rgba(71, 85, 105, 0.3);
}

.test-error {
  color: var(--error);
  border-color: rgba(239, 68, 68, 0.3);
  background: rgba(239, 68, 68, 0.06);
}

/* Ghost style buttons */
.card-actions {
  display: flex;
  gap: 6px;
  margin-top: 4px;
}

.btn {
  padding: 6px 14px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  background: transparent;
  color: var(--text-muted);
  transition: all 0.2s ease;
}

.btn:hover:not(:disabled) {
  background-color: var(--surface-light);
  border-color: var(--border);
  color: var(--text);
}

.btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-test:hover:not(:disabled) {
  border-color: var(--primary);
  color: var(--primary);
  background: rgba(59, 130, 246, 0.08);
}

.btn-delete:hover:not(:disabled) {
  border-color: var(--error);
  color: var(--error);
  background: rgba(239, 68, 68, 0.08);
}
</style>
