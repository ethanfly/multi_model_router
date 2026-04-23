<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import { GetDashboardStats } from '../../wailsjs/go/main/App'

interface ModelUsage {
  modelName: string
  count: number
  percentage: number
}

interface ComplexityDistribution {
  simple: number
  medium: number
  complex: number
}

interface LogEntry {
  time: string
  model: string
  complexity: string
  source: string
  tokens: number
  latency: number
}

interface DashboardStats {
  todayRequests: number
  todayTokens: number
  avgLatency: number
  modelUsage: ModelUsage[]
  complexityDistribution: ComplexityDistribution
  recentLogs: LogEntry[]
}

const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const error = ref('')

onMounted(async () => {
  await loadStats()
})

async function loadStats() {
  loading.value = true
  error.value = ''
  try {
    stats.value = await GetDashboardStats() as DashboardStats
  } catch (err: any) {
    error.value = err.message || 'Failed to load dashboard stats'
  } finally {
    loading.value = false
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
</script>

<template>
  <div class="dashboard">
    <h2 class="page-title">Dashboard</h2>

    <div v-if="loading" class="loading">Loading...</div>
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
          <div class="stat-label">Today's Requests</div>
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
          <div class="stat-value">{{ stats.todayTokens.toLocaleString() }}</div>
          <div class="stat-label">Today's Tokens</div>
        </div>
        <div class="stat-card latency-card">
          <div class="stat-icon">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10" />
              <polyline points="12 6 12 12 16 14" />
            </svg>
          </div>
          <div class="stat-value">{{ stats.avgLatency.toFixed(0) }}ms</div>
          <div class="stat-label">Avg Latency</div>
        </div>
      </div>

      <div class="charts-row">
        <div class="chart-section">
          <h3>Model Usage</h3>
          <div v-if="stats.modelUsage && stats.modelUsage.length" class="bar-chart">
            <div v-for="mu in stats.modelUsage" :key="mu.modelName" class="bar-row">
              <span class="bar-label">{{ mu.modelName }}</span>
              <div class="bar-track">
                <div class="bar-fill bar-fill-primary" :style="{ width: mu.percentage + '%' }"></div>
              </div>
              <span class="bar-pct">{{ mu.percentage.toFixed(1) }}%</span>
            </div>
          </div>
          <div v-else class="no-data">No usage data yet</div>
        </div>

        <div class="chart-section">
          <h3>Complexity Distribution</h3>
          <div v-if="stats.complexityDistribution" class="bar-chart">
            <div v-for="(val, key) in stats.complexityDistribution" :key="key" class="bar-row">
              <span class="bar-label">{{ key }}</span>
              <div class="bar-track">
                <div
                  class="bar-fill"
                  :style="{ width: val + '%', background: complexityColor(key as string) }"
                ></div>
              </div>
              <span class="bar-pct">{{ val.toFixed(1) }}%</span>
            </div>
          </div>
          <div v-else class="no-data">No data yet</div>
        </div>
      </div>

      <div class="logs-section">
        <h3>Recent Logs</h3>
        <div class="table-wrap">
          <table v-if="stats.recentLogs && stats.recentLogs.length">
            <thead>
              <tr>
                <th>Time</th>
                <th>Model</th>
                <th>Complexity</th>
                <th>Source</th>
                <th>Tokens</th>
                <th>Latency</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(log, i) in stats.recentLogs" :key="i">
                <td>{{ log.time }}</td>
                <td>{{ log.model }}</td>
                <td>
                  <span class="complexity-dot" :style="{ backgroundColor: complexityColor(log.complexity) }"></span>
                  {{ log.complexity }}
                </td>
                <td>{{ log.source }}</td>
                <td>{{ log.tokens }}</td>
                <td>{{ log.latency }}ms</td>
              </tr>
            </tbody>
          </table>
          <div v-else class="no-data">No logs yet</div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.dashboard {
  padding: 28px;
  color: var(--text);
  max-width: 960px;
  margin: 0 auto;
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
  margin: 0 0 16px 0;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-secondary);
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
