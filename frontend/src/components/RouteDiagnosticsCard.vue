<script lang="ts" setup>
import { computed } from 'vue'

interface CandidateDiagnostic {
  name: string
  modelId: string
  provider: string
  eligible: boolean
  score?: number
  decision?: string
  reason?: string
  recentRpm?: number
  recentTpm?: number
  maxRpm?: number
  maxTpm?: number
}

interface RouteDiagnostics {
  mode?: string
  classificationInput?: string
  classificationMethod?: string
  complexity?: string
  taskType?: string
  estimatedTokens?: number
  selectedModel?: string
  fallbackUsed?: boolean
  summary?: string
  candidates?: CandidateDiagnostic[]
}

const props = withDefaults(defineProps<{
  summary?: string
  diagnosticsJson?: string
  title?: string
}>(), {
  summary: '',
  diagnosticsJson: '',
  title: 'Routing Decision',
})

const diagnostics = computed<RouteDiagnostics | null>(() => {
  if (!props.diagnosticsJson) {
    return null
  }
  try {
    return JSON.parse(props.diagnosticsJson) as RouteDiagnostics
  } catch {
    return null
  }
})

const candidateCount = computed(() => diagnostics.value?.candidates?.length || 0)

function formatScore(score?: number): string {
  if (typeof score !== 'number' || Number.isNaN(score)) {
    return 'n/a'
  }
  return score.toFixed(2)
}
</script>

<template>
  <div v-if="summary || diagnostics" class="route-diagnostics">
    <details class="diagnostics-details">
      <summary class="diagnostics-summary-row">
        <span class="diagnostics-title">{{ title }}</span>
        <span class="diagnostics-summary-text">{{ summary || diagnostics?.summary || 'No summary' }}</span>
      </summary>

      <div class="diagnostics-body">
        <div v-if="diagnostics" class="diagnostics-pills">
          <span v-if="diagnostics.mode" class="diag-pill">{{ diagnostics.mode }}</span>
          <span v-if="diagnostics.complexity" class="diag-pill">{{ diagnostics.complexity }}</span>
          <span v-if="diagnostics.taskType" class="diag-pill">{{ diagnostics.taskType }}</span>
          <span v-if="diagnostics.selectedModel" class="diag-pill">picked: {{ diagnostics.selectedModel }}</span>
          <span v-if="diagnostics.estimatedTokens" class="diag-pill">est: {{ diagnostics.estimatedTokens }}</span>
          <span v-if="diagnostics.fallbackUsed" class="diag-pill warning">fallback</span>
          <span v-if="candidateCount" class="diag-pill muted">{{ candidateCount }} candidates</span>
        </div>

        <div v-if="diagnostics?.classificationMethod || diagnostics?.classificationInput" class="diagnostics-section">
          <div v-if="diagnostics.classificationMethod" class="section-label">
            Classifier: {{ diagnostics.classificationMethod }}
          </div>
          <div v-if="diagnostics.classificationInput" class="classification-input">
            {{ diagnostics.classificationInput }}
          </div>
        </div>

        <div v-if="diagnostics?.candidates?.length" class="diagnostics-section">
          <div class="section-label">Candidates</div>
          <div class="candidate-list">
            <div
              v-for="candidate in diagnostics.candidates"
              :key="`${candidate.name}-${candidate.modelId}`"
              :class="['candidate-card', { chosen: candidate.decision === 'selected', skipped: !candidate.eligible }]"
            >
              <div class="candidate-header">
                <div class="candidate-name">{{ candidate.name || candidate.modelId }}</div>
                <div class="candidate-meta">
                  <span class="candidate-badge">{{ candidate.provider || 'unknown' }}</span>
                  <span class="candidate-badge">{{ formatScore(candidate.score) }}</span>
                </div>
              </div>
              <div class="candidate-decision">
                {{ candidate.decision || (candidate.eligible ? 'eligible' : 'skipped') }}
              </div>
              <div v-if="candidate.reason" class="candidate-reason">{{ candidate.reason }}</div>
              <div class="candidate-limits">
                RPM {{ candidate.recentRpm || 0 }}/{{ candidate.maxRpm || 0 }}
                <span class="candidate-sep">|</span>
                TPM {{ candidate.recentTpm || 0 }}/{{ candidate.maxTpm || 0 }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </details>
  </div>
</template>

<style scoped>
.route-diagnostics {
  margin-top: 10px;
}

.diagnostics-details {
  border: 1px solid rgba(71, 85, 105, 0.35);
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.32);
  overflow: hidden;
}

.diagnostics-summary-row {
  list-style: none;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
}

.diagnostics-summary-row::-webkit-details-marker {
  display: none;
}

.diagnostics-title {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.diagnostics-summary-text {
  font-size: 12px;
  line-height: 1.5;
  color: var(--text-secondary);
}

.diagnostics-body {
  padding: 0 12px 12px;
}

.diagnostics-pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}

.diag-pill {
  padding: 3px 8px;
  border-radius: 999px;
  font-size: 11px;
  line-height: 1.2;
  background: rgba(59, 130, 246, 0.16);
  color: var(--primary);
  border: 1px solid rgba(59, 130, 246, 0.18);
}

.diag-pill.warning {
  background: rgba(245, 158, 11, 0.14);
  color: var(--warning);
  border-color: rgba(245, 158, 11, 0.2);
}

.diag-pill.muted {
  background: rgba(148, 163, 184, 0.12);
  color: var(--text-muted);
  border-color: rgba(148, 163, 184, 0.18);
}

.diagnostics-section + .diagnostics-section {
  margin-top: 12px;
}

.section-label {
  margin-bottom: 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--text-muted);
}

.classification-input {
  padding: 10px;
  border-radius: 10px;
  background: rgba(15, 23, 42, 0.55);
  color: var(--text-secondary);
  font-size: 12px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-word;
}

.candidate-list {
  display: grid;
  gap: 8px;
}

.candidate-card {
  padding: 10px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.45);
  border: 1px solid rgba(71, 85, 105, 0.28);
}

.candidate-card.chosen {
  border-color: rgba(16, 185, 129, 0.35);
  background: rgba(16, 185, 129, 0.08);
}

.candidate-card.skipped {
  border-color: rgba(148, 163, 184, 0.2);
}

.candidate-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.candidate-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}

.candidate-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.candidate-badge {
  padding: 2px 7px;
  border-radius: 999px;
  background: rgba(148, 163, 184, 0.12);
  color: var(--text-muted);
  font-size: 11px;
}

.candidate-decision {
  margin-top: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.candidate-reason {
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-muted);
}

.candidate-limits {
  margin-top: 6px;
  font-size: 11px;
  color: var(--text-muted);
}

.candidate-sep {
  margin: 0 6px;
  opacity: 0.5;
}
</style>
