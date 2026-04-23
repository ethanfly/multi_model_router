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
    case 'simple': return '#22c55e'
    case 'medium': return '#eab308'
    case 'complex': return '#ef4444'
    default: return '#6b7280'
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
        <div class="stat-card">
          <div class="stat-value">{{ stats.todayRequests }}</div>
          <div class="stat-label">Today's Requests</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ stats.todayTokens.toLocaleString() }}</div>
          <div class="stat-label">Today's Tokens</div>
        </div>
        <div class="stat-card">
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
                <div class="bar-fill" :style="{ width: mu.percentage + '%' }"></div>
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
                  :style="{ width: val + '%', backgroundColor: complexityColor(key as string) }"
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
  padding: 24px;
  color: #e5e7eb;
  max-width: 960px;
  margin: 0 auto;
}

.page-title {
  margin: 0 0 20px 0;
  font-size: 22px;
  font-weight: 600;
}

.loading, .error-msg, .no-data {
  color: #9ca3af;
  padding: 16px;
  text-align: center;
}

.error-msg {
  color: #ef4444;
}

.stat-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

.stat-card {
  background-color: #1f2937;
  border-radius: 10px;
  padding: 20px;
  text-align: center;
  border: 1px solid #374151;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: #3b82f6;
}

.stat-label {
  font-size: 13px;
  color: #9ca3af;
  margin-top: 4px;
}

.charts-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  margin-bottom: 24px;
}

.chart-section {
  background-color: #1f2937;
  border-radius: 10px;
  padding: 16px;
  border: 1px solid #374151;
}

.chart-section h3 {
  margin: 0 0 12px 0;
  font-size: 15px;
  font-weight: 600;
}

.bar-chart {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bar-label {
  width: 80px;
  font-size: 12px;
  text-align: right;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bar-track {
  flex: 1;
  height: 16px;
  background-color: #374151;
  border-radius: 4px;
  overflow: hidden;
}

.bar-fill {
  height: 100%;
  background-color: #3b82f6;
  border-radius: 4px;
  transition: width 0.3s;
}

.bar-pct {
  width: 50px;
  font-size: 12px;
  text-align: left;
  flex-shrink: 0;
}

.logs-section {
  background-color: #1f2937;
  border-radius: 10px;
  padding: 16px;
  border: 1px solid #374151;
}

.logs-section h3 {
  margin: 0 0 12px 0;
  font-size: 15px;
  font-weight: 600;
}

.table-wrap {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

th, td {
  padding: 8px 12px;
  text-align: left;
  border-bottom: 1px solid #374151;
}

th {
  color: #9ca3af;
  font-weight: 600;
}

td {
  color: #d1d5db;
}

.complexity-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;
  vertical-align: middle;
}
</style>
