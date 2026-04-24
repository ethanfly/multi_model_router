<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { GetDashboardStats, GetDashboardLogs } from '../../wailsjs/go/main/App'
import RouteDiagnosticsCard from '../components/RouteDiagnosticsCard.vue'

interface ModelUsage {
  modelId: string
  count: number
  percentage: number
}

interface LogEntry {
  time: string
  model: string
  complexity: string
  source: string
  tokens: number
  latency: number
  diagnostics: string
  diagnosticsJson: string
}

interface DashboardStats {
  todayRequests: number
  todayTokensIn: number
  todayTokensOut: number
  avgLatency: number
  modelUsage: ModelUsage[]
  complexityDist: Record<string, number>
}

const { t } = useI18n()

const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const error = ref('')

// Pagination state
const logs = ref<LogEntry[]>([])
const logPage = ref(1)
const logPageSize = 15
const logTotal = ref(0)
const logLoading = ref(false)
const logError = ref('')
const totalPages = computed(() => Math.max(1, Math.ceil(logTotal.value / logPageSize)))

onMounted(async () => {
  await Promise.all([loadStats(), loadLogs()])
})

async function loadStats() {
  loading.value = true
  error.value = ''
  try {
    const raw: any = await GetDashboardStats()
    if (!raw) {
      stats.value = null
      return
    }
    const mu: ModelUsage[] = (raw.model_usage || []).map((m: any) => ({
      modelId: m.modelId || '',
      count: m.count || 0,
      percentage: m.percentage || 0,
    }))
    stats.value = {
      todayRequests: raw.total_requests || 0,
      todayTokensIn: raw.total_tokens_in || 0,
      todayTokensOut: raw.total_tokens_out || 0,
      avgLatency: raw.avg_latency || 0,
      modelUsage: mu,
      complexityDist: raw.complexity_dist || {},
    }
  } catch (err: any) {
    error.value = err.message || 'Failed to load dashboard stats'
  } finally {
    loading.value = false
  }
}

async function loadLogs() {
  logLoading.value = true
  logError.value = ''
  try {
    const raw: any = await GetDashboardLogs(logPage.value, logPageSize)
    logTotal.value = raw.total || 0
    const rawLogs = Array.isArray(raw?.logs)
      ? raw.logs
      : raw?.logs && typeof raw.logs === 'object'
        ? Object.values(raw.logs)
        : []
    const mapped: LogEntry[] = rawLogs.map((l: any) => ({
      time: formatLogTime(l.createdAt ?? l.CreatedAt),
      model: l.modelId ?? l.ModelID ?? '',
      complexity: l.complexity ?? l.Complexity ?? '',
      source: l.source ?? l.Source ?? '',
      tokens: Number(l.tokensIn ?? l.TokensIn ?? 0) + Number(l.tokensOut ?? l.TokensOut ?? 0),
      latency: Number(l.latencyMs ?? l.LatencyMs ?? 0),
      diagnostics: l.diagnostics ?? l.Diagnostics ?? '',
      diagnosticsJson: l.diagnosticsJson ?? l.DiagnosticsJSON ?? '',
    }))
    logs.value = mapped
  } catch (err: any) {
    console.error('Failed to load dashboard logs', err)
    logError.value = err?.message || 'Failed to load logs'
    logs.value = []
  } finally {
    logLoading.value = false
  }
}

function prevPage() {
  if (logPage.value > 1) {
    logPage.value--
    loadLogs()
  }
}

function nextPage() {
  if (logPage.value < totalPages.value) {
    logPage.value++
    loadLogs()
  }
}

function complexityColor(c: string): string {
  switch (c) {
    case 'simple': return 'var(--success)'
    case 'medium': return 'var(--warning)'
    case 'complex': return 'var(--error)'
    default: return 'var(--text-muted)'
  }
}

function formatLogTime(value: unknown): string {
  if (!value) {
    return ''
  }
  const date = new Date(String(value))
  if (Number.isNaN(date.getTime())) {
    return String(value)
  }
  return date.toLocaleString()
}
</script>

<template>
  <div class="dashboard">
    <h2 class="page-title">{{ $t('dashboard.title') }}</h2>

    <div v-if="loading" class="loading">{{ $t('dashboard.loading') }}</div>
    <div v-else-if="error" class="error-msg">{{ error }}</div>

    <template v-else-if="stats">
      <div class="stat-cards">
        <div class="stat-card requests-card">
          <div class="stat-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
            </svg>
          </div>
          <div class="stat-value">{{ stats.todayRequests }}</div>
          <div class="stat-label">{{ $t('dashboard.todayRequests') }}</div>
        </div>
        <div class="stat-card tokens-card">
          <div class="stat-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
              <polyline points="14 2 14 8 20 8" />
              <line x1="16" y1="13" x2="8" y2="13" />
              <line x1="16" y1="17" x2="8" y2="17" />
            </svg>
          </div>
          <div class="stat-value">{{ (stats.todayTokensIn + stats.todayTokensOut).toLocaleString() }}</div>
          <div class="stat-label">{{ $t('dashboard.todayTokens') }}</div>
        </div>
        <div class="stat-card latency-card">
          <div class="stat-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </div>
          <div class="stat-value">{{ stats.avgLatency?.toFixed(0) ?? 0 }}ms</div>
          <div class="stat-label">{{ $t('dashboard.avgLatency') }}</div>
        </div>
      </div>

      <div class="charts-row">
        <div class="chart-section">
          <h3>{{ $t('dashboard.modelUsage') }}</h3>
          <div v-if="stats.modelUsage && stats.modelUsage.length" class="bar-chart">
            <div v-for="mu in stats.modelUsage" :key="mu.modelId" class="bar-row">
              <span class="bar-label">{{ mu.modelId }}</span>
              <div class="bar-track">
                <div class="bar-fill bar-fill-primary" :style="{ width: mu.percentage + '%' }"></div>
              </div>
              <span class="bar-pct">{{ mu.percentage.toFixed(1) }}%</span>
            </div>
          </div>
          <div v-else class="no-data">{{ $t('dashboard.noUsageData') }}</div>
        </div>

        <div class="chart-section">
          <h3>{{ $t('dashboard.complexityDist') }}</h3>
          <div v-if="stats.complexityDist" class="bar-chart">
            <div v-for="(val, key) in stats.complexityDist" :key="key" class="bar-row">
              <span class="bar-label">{{ key }}</span>
              <div class="bar-track">
                <div
                  class="bar-fill"
                  :style="{ width: val + '%', background: complexityColor(key as string) }"
                ></div>
              </div>
              <span class="bar-pct">{{ (val as number)?.toFixed(1) ?? '0.0' }}%</span>
            </div>
          </div>
          <div v-else class="no-data">{{ $t('dashboard.noData') }}</div>
        </div>
      </div>

      <div class="logs-section">
        <div class="logs-header">
          <h3>{{ $t('dashboard.recentLogs') }}</h3>
          <div v-if="logTotal > 0" class="pagination">
            <button class="page-btn" :disabled="logPage <= 1 || logLoading" @click="prevPage">{{ $t('dashboard.prevPage') }}</button>
            <span class="page-info">{{ logPage }} / {{ totalPages }}</span>
            <button class="page-btn" :disabled="logPage >= totalPages || logLoading" @click="nextPage">{{ $t('dashboard.nextPage') }}</button>
          </div>
        </div>
        <div class="table-wrap">
          <table v-if="logs.length">
            <thead>
              <tr>
                <th>{{ $t('dashboard.colTime') }}</th>
                <th>{{ $t('dashboard.colModel') }}</th>
                <th>{{ $t('dashboard.colComplexity') }}</th>
                <th>{{ $t('dashboard.colSource') }}</th>
                <th>{{ $t('dashboard.colTokens') }}</th>
                <th>{{ $t('dashboard.colLatency') }}</th>
                <th>Decision</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(log, i) in logs" :key="i">
                <td>{{ log.time }}</td>
                <td>{{ log.model }}</td>
                <td>
                  <span class="complexity-dot" :style="{ backgroundColor: complexityColor(log.complexity) }"></span>
                  {{ log.complexity }}
                </td>
                <td>{{ log.source }}</td>
                <td>{{ log.tokens }}</td>
                <td>{{ log.latency }}ms</td>
                <td class="diagnostics-cell">
                  <RouteDiagnosticsCard
                    v-if="log.diagnostics || log.diagnosticsJson"
                    :summary="log.diagnostics"
                    :diagnostics-json="log.diagnosticsJson"
                    title="Decision"
                  />
                  <span v-else>—</span>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else-if="logError" class="no-data">{{ logError }}</div>
          <div v-else class="no-data">{{ $t('dashboard.noLogs') }}</div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.dashboard {
  padding: 28px;
  color: var(--text);
  height: 100%;
  overflow-y: auto;
}

.page-title {
  margin: 0 0 24px 0;
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--text), var(--text-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.loading, .error-msg, .no-data {
  color: var(--text-muted);
  padding: 16px;
  text-align: center;
}

.error-msg {
  color: var(--error);
}

/* Gradient stat cards */
.stat-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  position: relative;
  border-radius: var(--radius);
  padding: 24px 20px;
  text-align: center;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.stat-card::before {
  content: '';
  position: absolute;
  inset: 0;
  opacity: 0.12;
  border-radius: var(--radius);
}

.requests-card {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.15), rgba(59, 130, 246, 0.05));
  border-color: rgba(59, 130, 246, 0.2);
}

.requests-card::before {
  background: linear-gradient(135deg, var(--primary), var(--accent));
}

.requests-card .stat-icon {
  color: var(--primary);
}

.requests-card .stat-value {
  color: var(--primary);
}

.tokens-card {
  background: linear-gradient(135deg, rgba(6, 182, 212, 0.15), rgba(6, 182, 212, 0.05));
  border-color: rgba(6, 182, 212, 0.2);
}

.tokens-card::before {
  background: linear-gradient(135deg, var(--accent), var(--success));
}

.tokens-card .stat-icon {
  color: var(--accent);
}

.tokens-card .stat-value {
  color: var(--accent);
}

.latency-card {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.15), rgba(139, 92, 246, 0.05));
  border-color: rgba(139, 92, 246, 0.2);
}

.latency-card::before {
  background: linear-gradient(135deg, var(--secondary), var(--primary));
}

.latency-card .stat-icon {
  color: var(--secondary);
}

.latency-card .stat-value {
  color: var(--secondary);
}

.stat-icon {
  margin-bottom: 8px;
  opacity: 0.8;
}

.stat-value {
  font-size: 32px;
  font-weight: 800;
  line-height: 1.1;
}

.stat-label {
  font-size: 13px;
  color: var(--text-muted);
  margin-top: 6px;
  font-weight: 500;
}

/* Charts */
.charts-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 24px;
}

.chart-section {
  background-color: var(--bg);
  border-radius: var(--radius);
  padding: 20px;
  border: 1px solid var(--border);
}

.chart-section h3 {
  margin: 0 0 16px 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-secondary);
}

.bar-chart {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.bar-label {
  width: 80px;
  font-size: 12px;
  text-align: right;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-secondary);
}

.bar-track {
  flex: 1;
  height: 14px;
  background-color: var(--surface-light);
  border-radius: 7px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  border-radius: 7px;
  transition: width 0.6s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
}

.bar-fill-primary {
  background: linear-gradient(90deg, var(--primary), var(--accent));
}

.bar-fill::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.15), transparent);
  animation: shimmer 2s infinite;
}

@keyframes shimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

.bar-pct {
  width: 50px;
  font-size: 12px;
  text-align: left;
  flex-shrink: 0;
  color: var(--text-secondary);
  font-weight: 500;
}

/* Logs table */
.logs-section {
  background-color: var(--bg);
  border-radius: var(--radius);
  padding: 20px;
  border: 1px solid var(--border);
}

.logs-section h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-secondary);
}

.logs-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.pagination {
  display: flex;
  align-items: center;
  gap: 10px;
}

.page-btn {
  padding: 4px 12px;
  font-size: 12px;
  font-weight: 500;
  border: 1px solid var(--border);
  background: var(--bg);
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: all 0.15s ease;
}

.page-btn:hover:not(:disabled) {
  background: var(--surface-light);
  color: var(--text);
  border-color: var(--primary);
}

.page-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}

.page-info {
  font-size: 12px;
  color: var(--text-muted);
  min-width: 60px;
  text-align: center;
}

.table-wrap {
  overflow-x: auto;
  border-radius: var(--radius-sm);
}

table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  font-size: 13px;
}

th, td {
  padding: 10px 14px;
  text-align: left;
}

thead tr {
  background: var(--surface);
}

th {
  color: var(--text-muted);
  font-weight: 600;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 1px solid var(--border);
}

th:first-child {
  border-radius: var(--radius-sm) 0 0 0;
}

th:last-child {
  border-radius: 0 var(--radius-sm) 0 0;
}

tbody tr {
  transition: background-color 0.15s ease;
}

tbody tr:hover {
  background-color: rgba(51, 65, 85, 0.3);
}

td {
  color: var(--text-secondary);
  border-bottom: 1px solid rgba(71, 85, 105, 0.3);
}

.diagnostics-cell {
  min-width: 320px;
  max-width: 420px;
  white-space: normal;
  line-height: 1.45;
  color: var(--text-muted);
  font-size: 12px;
}

.complexity-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 6px;
  vertical-align: middle;
  box-shadow: 0 0 6px currentColor;
}

@media (max-width: 700px) {
  .stat-cards {
    grid-template-columns: 1fr;
  }
  .charts-row {
    grid-template-columns: 1fr;
  }
}
</style>
