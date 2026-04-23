<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { GetClassifierConfig, SetClassifierConfig } from '../../wailsjs/go/main/App'

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

onMounted(async () => {
  try {
    const raw = await GetClassifierConfig()
    if (raw) {
      const cfg: ClassifierConfig = JSON.parse(raw)
      complexKeywords.value = (cfg.complex_keywords || []).join('\n')
      simpleKeywords.value = (cfg.simple_keywords || []).join('\n')
      multiStepKeywords.value = (cfg.multi_step_keywords || []).join('\n')
      mathSymbols.value = (cfg.math_symbols || []).join('\n')
      codingKeywords.value = (cfg.coding_keywords || []).join('\n')
      reasoningKeywords.value = (cfg.reasoning_keywords || []).join('\n')
      complexThreshold.value = cfg.complex_threshold ?? 0.3
      simpleThreshold.value = cfg.simple_threshold ?? -0.2
    }
  } catch { /* ignore */ }
})

function buildConfig(): ClassifierConfig {
  return {
    complex_keywords: complexKeywords.value.split('\n').map(s => s.trim()).filter(Boolean),
    simple_keywords: simpleKeywords.value.split('\n').map(s => s.trim()).filter(Boolean),
    multi_step_keywords: multiStepKeywords.value.split('\n').map(s => s.trim()).filter(Boolean),
    math_symbols: mathSymbols.value.split('\n').map(s => s.trim()).filter(Boolean),
    coding_keywords: codingKeywords.value.split('\n').map(s => s.trim()).filter(Boolean),
    reasoning_keywords: reasoningKeywords.value.split('\n').map(s => s.trim()).filter(Boolean),
    complex_threshold: complexThreshold.value,
    simple_threshold: simpleThreshold.value,
  }
}

async function handleSave() {
  saving.value = true
  message.value = ''
  try {
    const json = JSON.stringify(buildConfig())
    await SetClassifierConfig(json)
    message.value = t('rules.saved')
    setTimeout(() => { message.value = '' }, 2000)
  } catch (err: any) {
    message.value = 'Error: ' + (err.message || err)
  } finally {
    saving.value = false
  }
}

function handleReset() {
  const defaults: ClassifierConfig = {
    complex_keywords: [
      '设计', '架构', '推导', '证明', '优化', '重构', '实现', '构建', '部署', '调试',
      '分析', '评估', '对比', '权衡', '方案', '策略', '模式',
      'design', 'architect', 'derive', 'prove', 'optimize', 'refactor',
      'implement a system', 'build a', 'create a framework',
      'troubleshoot', 'debug', 'migrate', 'integrate',
      'best practice', 'design pattern', 'trade-off', 'compare',
    ],
    simple_keywords: [
      '翻译', '总结', '改写', '你好', '谢谢', '是什么', '什么是', '解释一下',
      'translate', 'summarize', 'rewrite', 'what is', 'define', 'list',
      'hello', 'hi ', 'thanks', 'please explain',
      'how to say', 'meaning of', 'convert',
    ],
    multi_step_keywords: [
      '步骤', '第一步', '首先', '然后', '接下来', '最后', '流程',
      'step', 'first,', 'then,', 'finally,', 'next,', 'after that',
      'workflow', 'pipeline', 'procedure',
    ],
    math_symbols: [
      '∫', '∑', '∂', '∇', '方程', '积分', '微分', '矩阵', '向量',
      'prove', 'theorem', 'lemma', 'corollary',
      '∂²', '∞', '≈', '≠', '≤', '≥', '∀', '∃',
    ],
    coding_keywords: [
      '函数', '类', '接口', '算法', '排序', '递归', '并发', '异步',
      '数据库', '缓存', '索引', '事务', '锁',
      'function', 'class ', 'interface', 'algorithm', 'sort',
      'recursion', 'concurrent', 'async', 'await',
      'database', 'cache', 'index', 'transaction', 'lock',
      'api', 'endpoint', 'middleware', 'handler',
      'test', 'unit test', 'integration test', 'benchmark',
      'docker', 'kubernetes', 'container',
    ],
    reasoning_keywords: [
      '为什么', '原因', '逻辑', '推理', '因果', '假设',
      'why', 'reason', 'because', 'logic', 'inference',
      'hypothesis', 'assumption', 'therefore', 'conclusion',
      'analyze', 'evaluate', 'assess', 'investigate',
      'pros and cons', 'advantages', 'disadvantages',
    ],
    complex_threshold: 0.3,
    simple_threshold: -0.2,
  }
  complexKeywords.value = defaults.complex_keywords.join('\n')
  simpleKeywords.value = defaults.simple_keywords.join('\n')
  multiStepKeywords.value = defaults.multi_step_keywords.join('\n')
  mathSymbols.value = defaults.math_symbols.join('\n')
  codingKeywords.value = defaults.coding_keywords.join('\n')
  reasoningKeywords.value = defaults.reasoning_keywords.join('\n')
  complexThreshold.value = defaults.complex_threshold
  simpleThreshold.value = defaults.simple_threshold
  message.value = t('rules.resetDone')
  setTimeout(() => { message.value = '' }, 2000)
}
</script>

<template>
  <div class="rules-page">
    <div class="page-header">
      <h2 class="page-title">{{ $t('rules.title') }}</h2>
    </div>

    <div class="rules-grid">
      <!-- Complex Keywords -->
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.complexKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.complexKeywordsDesc') }}</p>
        <textarea v-model="complexKeywords" rows="6" class="rule-textarea"></textarea>
      </div>

      <!-- Simple Keywords -->
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.simpleKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.simpleKeywordsDesc') }}</p>
        <textarea v-model="simpleKeywords" rows="6" class="rule-textarea"></textarea>
      </div>

      <!-- Coding Keywords -->
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.codingKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.codingKeywordsDesc') }}</p>
        <textarea v-model="codingKeywords" rows="5" class="rule-textarea"></textarea>
      </div>

      <!-- Reasoning Keywords -->
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.reasoningKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.reasoningKeywordsDesc') }}</p>
        <textarea v-model="reasoningKeywords" rows="5" class="rule-textarea"></textarea>
      </div>

      <!-- Multi-step Keywords -->
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.multiStepKeywords') }}</label>
        <p class="rule-desc">{{ $t('rules.multiStepKeywordsDesc') }}</p>
        <textarea v-model="multiStepKeywords" rows="4" class="rule-textarea"></textarea>
      </div>

      <!-- Math Symbols -->
      <div class="rule-card">
        <label class="rule-label">{{ $t('rules.mathSymbols') }}</label>
        <p class="rule-desc">{{ $t('rules.mathSymbolsDesc') }}</p>
        <textarea v-model="mathSymbols" rows="4" class="rule-textarea"></textarea>
      </div>

      <!-- Thresholds -->
      <div class="rule-card thresholds-card">
        <label class="rule-label">{{ $t('rules.thresholds') }}</label>
        <p class="rule-desc">{{ $t('rules.thresholdHint') }}</p>
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
      <button @click="handleSave" :disabled="saving" class="btn btn-save">
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
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  display: block;
  margin-bottom: 4px;
}

.rule-desc {
  font-size: 12px;
  color: var(--text-muted);
  margin: 0 0 10px 0;
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
  color: var(--text-muted);
  font-weight: 500;
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
