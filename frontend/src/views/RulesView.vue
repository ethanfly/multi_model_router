<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { GetClassifierConfig, GetDefaultClassifierConfig, SetClassifierConfig } from '../../wailsjs/go/main/App'

const { t } = useI18n()

interface ClassifierConfig {
  complex_keywords: string[]
  simple_keywords: string[]
  multi_step_keywords: string[]
  math_symbols: string[]
  coding_keywords: string[]
  reasoning_keywords: string[]
  complex_threshold: number
  simple_threshold: number
}

const complexKeywords = ref('')
const simpleKeywords = ref('')
const multiStepKeywords = ref('')
const mathSymbols = ref('')
const codingKeywords = ref('')
const reasoningKeywords = ref('')
const complexThreshold = ref(0.3)
const simpleThreshold = ref(-0.2)
const saving = ref(false)
const message = ref('')

const thresholdError = computed(() =>
  simpleThreshold.value >= complexThreshold.value ? t('rules.thresholdInvalid') : '',
)

function applyConfig(cfg: ClassifierConfig) {
  complexKeywords.value = (cfg.complex_keywords || []).join('\n')
  simpleKeywords.value = (cfg.simple_keywords || []).join('\n')
  multiStepKeywords.value = (cfg.multi_step_keywords || []).join('\n')
  mathSymbols.value = (cfg.math_symbols || []).join('\n')
  codingKeywords.value = (cfg.coding_keywords || []).join('\n')
  reasoningKeywords.value = (cfg.reasoning_keywords || []).join('\n')
  complexThreshold.value = cfg.complex_threshold ?? 0.3
  simpleThreshold.value = cfg.simple_threshold ?? -0.2
}

async function loadCurrentConfig() {
  const raw = await GetClassifierConfig()
  if (!raw) {
    return
  }
  applyConfig(JSON.parse(raw) as ClassifierConfig)
}

onMounted(async () => {
  try {
    await loadCurrentConfig()
  } catch {
    // Ignore initial load failures to keep the page usable.
  }
})

function buildConfig(): ClassifierConfig {
  return {
    complex_keywords: complexKeywords.value.split('\n').map((s) => s.trim()).filter(Boolean),
    simple_keywords: simpleKeywords.value.split('\n').map((s) => s.trim()).filter(Boolean),
    multi_step_keywords: multiStepKeywords.value.split('\n').map((s) => s.trim()).filter(Boolean),
    math_symbols: mathSymbols.value.split('\n').map((s) => s.trim()).filter(Boolean),
    coding_keywords: codingKeywords.value.split('\n').map((s) => s.trim()).filter(Boolean),
    reasoning_keywords: reasoningKeywords.value.split('\n').map((s) => s.trim()).filter(Boolean),
    complex_threshold: complexThreshold.value,
    simple_threshold: simpleThreshold.value,
  }
}

async function handleSave() {
  if (thresholdError.value) {
    message.value = thresholdError.value
    return
  }

  saving.value = true
  message.value = ''
  try {
    await SetClassifierConfig(JSON.stringify(buildConfig()))
    message.value = t('rules.saved')
    setTimeout(() => {
      message.value = ''
    }, 2000)
  } catch (err: any) {
    message.value = 'Error: ' + (err.message || err)
  } finally {
    saving.value = false
  }
}

async function handleReset() {
  try {
    const raw = await GetDefaultClassifierConfig()
    if (raw) {
      applyConfig(JSON.parse(raw) as ClassifierConfig)
    }
    message.value = t('rules.resetDone')
    setTimeout(() => {
      message.value = ''
    }, 2000)
  } catch (err: any) {
    message.value = 'Error: ' + (err.message || err)
  }
}
</script>

<template>
  <div class="rules-page">
    <div class="page-header">
      <h2 class="page-title">{{ $t('rules.title') }}</h2>
      <p class="page-subtitle">{{ $t('rules.subtitle') }}</p>
    </div>

    <div class="rules-grid">
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.complexKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.complexKeywordsDesc') }}</p>
        <textarea v-model="complexKeywords" rows="6" class="rule-textarea"></textarea>
      </div>

      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.simpleKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.simpleKeywordsDesc') }}</p>
        <textarea v-model="simpleKeywords" rows="6" class="rule-textarea"></textarea>
      </div>

      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.codingKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.codingKeywordsDesc') }}</p>
        <textarea v-model="codingKeywords" rows="5" class="rule-textarea"></textarea>
      </div>

      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.reasoningKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.reasoningKeywordsDesc') }}</p>
        <textarea v-model="reasoningKeywords" rows="5" class="rule-textarea"></textarea>
      </div>

      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.multiStepKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.multiStepKeywordsDesc') }}</p>
        <textarea v-model="multiStepKeywords" rows="4" class="rule-textarea"></textarea>
      </div>

      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.mathSymbols') }}</label>
        <p class="rule-desc">{{ $t('rules.mathSymbolsDesc') }}</p>
        <textarea v-model="mathSymbols" rows="4" class="rule-textarea"></textarea>
      </div>

      <div class="rule-card thresholds-card">
        <label class="rule-label">{{ $t('rules.thresholds') }}</label>
        <p class="rule-desc">{{ $t('rules.thresholdHint') }}</p>
        <p v-if="thresholdError" class="rule-warning">{{ thresholdError }}</p>
        <div class="threshold-row">
          <div class="threshold-field">
            <span class="threshold-name">{{ $t('rules.complexThreshold') }}</span>
            <input type="number" v-model.number="complexThreshold" step="0.05" min="0" max="1" />
          </div>
          <div class="threshold-field">
            <span class="threshold-name">{{ $t('rules.simpleThreshold') }}</span>
            <input type="number" v-model.number="simpleThreshold" step="0.05" min="-1" max="1" />
          </div>
        </div>
      </div>
    </div>

    <div v-if="message" class="save-message">{{ message }}</div>

    <div class="actions">
      <button @click="handleSave" :disabled="saving || !!thresholdError" class="btn btn-save">
        {{ saving ? '...' : $t('rules.save') }}
      </button>
      <button @click="handleReset" class="btn btn-reset">{{ $t('rules.reset') }}</button>
    </div>
  </div>
</template>

<style scoped>
.rules-page {
  padding: 28px;
  color: var(--text);
  height: 100%;
  overflow-y: auto;
}

.page-header {
  margin-bottom: 24px;
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

.page-subtitle {
  margin: 8px 0 0;
  font-size: 13px;
  line-height: 1.5;
  color: var(--text-muted);
}

.rules-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.rule-card {
  background: rgba(30, 41, 59, 0.6);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(71, 85, 105, 0.4);
  border-radius: var(--radius);
  padding: 20px;
}

.thresholds-card {
  grid-column: span 2;
}

.rule-label {
  display: block;
  margin-bottom: 4px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
}

.rule-desc {
  margin: 0 0 10px;
  font-size: 12px;
  color: var(--text-muted);
}

.rule-warning {
  margin: 0 0 10px;
  font-size: 12px;
  color: var(--warning);
}

.rule-textarea {
  width: 100%;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 13px;
  line-height: 1.6;
  padding: 10px 12px;
  resize: vertical;
  outline: none;
  transition: border-color 0.2s ease;
}

.rule-textarea:focus {
  border-color: var(--primary);
}

.threshold-row {
  display: flex;
  gap: 20px;
}

.threshold-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.threshold-name {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-muted);
}

.threshold-field input {
  width: 120px;
  padding: 8px 12px;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  font-size: 13px;
  outline: none;
}

.threshold-field input:focus {
  border-color: var(--primary);
}

.save-message {
  margin-top: 16px;
  padding: 8px 14px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.3);
  border-radius: var(--radius-sm);
  color: var(--success);
  font-size: 13px;
}

.actions {
  display: flex;
  gap: 10px;
  margin-top: 20px;
}

.btn {
  padding: 8px 20px;
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

.btn-save {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: white;
  box-shadow: 0 2px 10px rgba(59, 130, 246, 0.25);
}

.btn-save:hover:not(:disabled) {
  box-shadow: 0 4px 16px rgba(59, 130, 246, 0.35);
  transform: translateY(-1px);
}

.btn-reset {
  background: transparent;
  color: var(--text-muted);
  border: 1px dashed var(--border);
}

.btn-reset:hover {
  border-color: var(--text-secondary);
  color: var(--text);
}

@media (max-width: 900px) {
  .rules-grid {
    grid-template-columns: 1fr;
  }

  .thresholds-card {
    grid-column: span 1;
  }
}
</style>
