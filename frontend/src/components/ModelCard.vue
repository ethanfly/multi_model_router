<script lang="ts" setup>
import { ref, computed } from 'vue'
import { TestModel } from '../../wailsjs/go/main/App'
import type { Model } from '../stores/models'

const props = defineProps<{ model: Model }>()
const emit = defineEmits<{
  edit: []
  delete: []
}>()

const testing = ref(false)
const testResult = ref('')

function providerColor(provider: string): string {
  switch (provider.toLowerCase()) {
    case 'openai': return '#22c55e'
    case 'anthropic': return '#f59e0b'
    case 'google': return '#3b82f6'
    case 'ollama': return '#8b5cf6'
    default: return '#6b7280'
  }
}

const scores = computed(() => [
  { label: 'Reasoning', value: props.model.reasoning, color: '#3b82f6' },
  { label: 'Coding', value: props.model.coding, color: '#22c55e' },
  { label: 'Creativity', value: props.model.creativity, color: '#f59e0b' },
  { label: 'Speed', value: props.model.speed, color: '#8b5cf6' },
  { label: 'Cost Eff.', value: props.model.costEfficiency, color: '#ec4899' },
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
        <span class="provider-badge" :style="{ backgroundColor: providerColor(model.provider) }">
          {{ model.provider }}
        </span>
      </div>
      <span :class="['status-indicator', model.isActive ? 'active' : 'inactive']">
        {{ model.isActive ? '\u25CF' : '\u25CB' }} {{ model.isActive ? 'Active' : 'Inactive' }}
      </span>
    </div>

    <div class="scores">
      <div v-for="s in scores" :key="s.label" class="score-row">
        <span class="score-label">{{ s.label }}</span>
        <div class="score-bar-track">
          <div class="score-bar-fill" :style="{ width: (s.value * 10) + '%', backgroundColor: s.color }"></div>
        </div>
        <span class="score-value">{{ s.value }}</span>
      </div>
    </div>

    <div class="card-meta">
      <span>RPM: {{ model.maxRpm || 'Unlimited' }}</span>
    </div>

    <div v-if="testResult" class="test-result">{{ testResult }}</div>

    <div class="card-actions">
      <button @click="emit('edit')" class="btn btn-sm">Edit</button>
      <button @click="handleTest" :disabled="testing" class="btn btn-sm btn-test">
        {{ testing ? '...' : 'Test' }}
      </button>
      <button @click="emit('delete')" class="btn btn-sm btn-delete">Delete</button>
    </div>
  </div>
</template>

<style scoped>
.model-card {
  background-color: #1f2937;
  border: 1px solid #374151;
  border-radius: 10px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
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
  color: #e5e7eb;
}

.provider-badge {
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  color: #ffffff;
}

.status-indicator {
  font-size: 12px;
  white-space: nowrap;
}

.status-indicator.active {
  color: #22c55e;
}

.status-indicator.inactive {
  color: #6b7280;
}

.scores {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.score-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.score-label {
  width: 70px;
  font-size: 12px;
  color: #9ca3af;
  text-align: right;
  flex-shrink: 0;
}

.score-bar-track {
  flex: 1;
  height: 8px;
  background-color: #374151;
  border-radius: 4px;
  overflow: hidden;
}

.score-bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.3s;
}

.score-value {
  width: 20px;
  font-size: 12px;
  color: #d1d5db;
  text-align: center;
  flex-shrink: 0;
}

.card-meta {
  font-size: 12px;
  color: #6b7280;
}

.test-result {
  font-size: 12px;
  color: #9ca3af;
  background-color: #111827;
  padding: 6px 8px;
  border-radius: 4px;
  word-break: break-all;
}

.card-actions {
  display: flex;
  gap: 6px;
  margin-top: 4px;
}

.btn {
  padding: 4px 12px;
  border: 1px solid #4b5563;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
  background-color: #374151;
  color: #e5e7eb;
  transition: background-color 0.2s;
}

.btn:hover:not(:disabled) {
  background-color: #4b5563;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-sm {
  padding: 4px 10px;
}

.btn-test {
  border-color: #3b82f6;
  color: #60a5fa;
}

.btn-delete {
  border-color: #ef4444;
  color: #f87171;
}
</style>
