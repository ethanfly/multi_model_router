<script lang="ts" setup>
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GetProxyStatus, StartProxy, StopProxy, GetConfig, SetConfig, GetAutoStart, SetAutoStart as SetAutoStartFn } from '../../wailsjs/go/main/App'

const proxyRunning = ref(false)
const proxyPort = ref(9680)
const proxyUrl = ref('')
const proxyLoading = ref(false)
const copySuccess = ref(false)
const autoStartEnabled = ref(false)

const { locale, t } = useI18n()
const currentLang = computed({
  get: () => locale.value,
  set: (val: string) => {
    locale.value = val
    localStorage.setItem('language', val)
  },
})

onMounted(async () => {
  try { await loadProxyStatus() } catch { /* ignore */ }
  try { autoStartEnabled.value = await GetAutoStart() } catch { /* ignore */ }
  try {
    const port = await GetConfig('proxyPort')
    if (port) proxyPort.value = parseInt(port, 10) || 9680
  } catch { /* ignore */ }
})

async function loadProxyStatus() {
  try {
    const status = await GetProxyStatus() as any
    proxyRunning.value = status.running
    proxyPort.value = status.port || 9680
    proxyUrl.value = status.url || ''
  } catch { /* ignore */ }
}

async function toggleProxy() {
  proxyLoading.value = true
  try {
    if (proxyRunning.value) {
      await StopProxy()
    } else {
      await SetConfig('proxyPort', String(proxyPort.value))
      await StartProxy(proxyPort.value)
    }
    await loadProxyStatus()
  } catch (err: any) {
    alert(t('settings.proxyError') + ': ' + (err.message || err))
  } finally {
    proxyLoading.value = false
  }
}

async function copyProxyUrl() {
  if (!proxyUrl.value) return
  try {
    await navigator.clipboard.writeText(proxyUrl.value)
    copySuccess.value = true
    setTimeout(() => { copySuccess.value = false }, 1500)
  } catch { /* ignore */ }
}

async function toggleAutoStart() {
  try {
    const result = await SetAutoStartFn(autoStartEnabled.value)
    if (result !== 'OK') {
      alert(t('settings.autoStartFail') + ': ' + result)
      autoStartEnabled.value = !autoStartEnabled.value
    }
  } catch (err: any) {
    alert(t('settings.autoStartFail') + ': ' + (err.message || err))
    autoStartEnabled.value = !autoStartEnabled.value
  }
}
</script>

<template>
  <div class="settings">
    <h2 class="page-title">{{ $t('settings.title') }}</h2>

    <section class="section">
      <h3 class="section-title">{{ $t('settings.general') }}</h3>
      <div class="general-card">
        <div class="toggle-row">
          <div class="toggle-info">
            <span class="toggle-label">{{ $t('settings.language') }}</span>
            <span class="toggle-desc">{{ $t('settings.languageDesc') }}</span>
          </div>
          <select v-model="currentLang" class="lang-select">
            <option value="en">English</option>
            <option value="zh">中文</option>
          </select>
        </div>
        <div class="toggle-row" style="margin-top: 16px;">
          <div class="toggle-info">
            <span class="toggle-label">{{ $t('settings.autoStart') }}</span>
            <span class="toggle-desc">{{ $t('settings.autoStartDesc') }}</span>
          </div>
          <label class="toggle-switch">
            <input type="checkbox" v-model="autoStartEnabled" @change="toggleAutoStart" />
            <span class="toggle-slider"></span>
          </label>
        </div>
      </div>
    </section>

    <section class="section">
      <h3 class="section-title">{{ $t('settings.proxy') }}</h3>
      <div class="proxy-card">
        <div class="proxy-row">
          <div class="proxy-field">
            <label>{{ $t('settings.port') }}</label>
            <input
              type="number"
              v-model.number="proxyPort"
              :disabled="proxyRunning"
              min="1"
              max="65535"
            />
          </div>
          <button
            @click="toggleProxy"
            :disabled="proxyLoading"
            :class="['btn', proxyRunning ? 'btn-stop' : 'btn-start']"
          >
            {{ proxyLoading ? '...' : proxyRunning ? $t('settings.stopProxy') : $t('settings.startProxy') }}
          </button>
          <div class="proxy-status">
            <span :class="['status-dot', proxyRunning ? 'active' : 'inactive']"></span>
            {{ proxyRunning ? $t('settings.running') : $t('settings.stopped') }}
          </div>
          <button
            v-if="proxyRunning && proxyUrl"
            @click="copyProxyUrl"
            class="btn btn-secondary"
          >
            {{ copySuccess ? $t('settings.copied') : $t('settings.copyUrl') }}
          </button>
        </div>
        <div v-if="proxyUrl" class="proxy-url">
          {{ $t('settings.proxyUrl') }} <code>{{ proxyUrl }}</code>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.settings {
  padding: 28px;
  color: var(--text);
  max-width: 960px;
  margin: 0 auto;
}

.page-title {
  margin: 0 0 28px 0;
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--text), var(--text-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.section {
  margin-bottom: 36px;
}

/* Section title with gradient underline */
.section-title {
  margin: 0 0 16px 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-secondary);
  padding-bottom: 10px;
  position: relative;
}

.section-title::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 0;
  width: 48px;
  height: 3px;
  border-radius: 2px;
  background: linear-gradient(90deg, var(--primary), var(--accent));
}

/* Glass proxy card */
.proxy-card {
  background: rgba(30, 41, 59, 0.6);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(71, 85, 105, 0.4);
  border-radius: var(--radius);
  padding: 20px;
}

/* Buttons */
.btn {
  padding: 8px 18px;
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

.btn-start {
  background: linear-gradient(135deg, var(--success), #059669);
  color: white;
  box-shadow: 0 2px 10px rgba(16, 185, 129, 0.25);
}

.btn-start:hover:not(:disabled) {
  box-shadow: 0 4px 16px rgba(16, 185, 129, 0.35);
  transform: translateY(-1px);
}

.btn-stop {
  background: linear-gradient(135deg, var(--error), #dc2626);
  color: white;
  box-shadow: 0 2px 10px rgba(239, 68, 68, 0.25);
}

.btn-stop:hover:not(:disabled) {
  box-shadow: 0 4px 16px rgba(239, 68, 68, 0.35);
  transform: translateY(-1px);
}

.btn-secondary {
  background-color: var(--surface-light);
  color: var(--text-secondary);
  border: 1px solid var(--border);
}

.btn-secondary:hover:not(:disabled) {
  background-color: var(--border);
  color: var(--text);
}

.proxy-row {
  display: flex;
  align-items: flex-end;
  gap: 14px;
  flex-wrap: wrap;
}

.proxy-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.proxy-field label {
  font-size: 11px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  font-weight: 600;
}

.proxy-field input {
  width: 110px;
  padding: 8px 12px;
}

.proxy-field input:disabled {
  opacity: 0.5;
}

.proxy-status {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  padding-bottom: 6px;
  color: var(--text-secondary);
}

.status-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  transition: all 0.3s ease;
}

.status-dot.active {
  background-color: var(--success);
  box-shadow: 0 0 8px var(--success);
  animation: status-pulse 2s ease-in-out infinite;
}

.status-dot.inactive {
  background-color: var(--text-muted);
}

@keyframes status-pulse {
  0%, 100% { box-shadow: 0 0 4px var(--success); }
  50% { box-shadow: 0 0 12px var(--success); }
}

.proxy-url {
  margin-top: 14px;
  font-size: 13px;
  color: var(--text-muted);
}

.proxy-url code {
  background-color: var(--bg);
  padding: 4px 10px;
  border-radius: 6px;
  color: var(--primary);
  border: 1px solid rgba(71, 85, 105, 0.3);
}

/* General settings card */
.general-card {
  background: rgba(30, 41, 59, 0.6);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(71, 85, 105, 0.4);
  border-radius: var(--radius);
  padding: 20px;
}

.toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.toggle-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.toggle-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--text);
}

.toggle-desc {
  font-size: 12px;
  color: var(--text-muted);
}

.toggle-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  cursor: pointer;
}

.toggle-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--border);
  border-radius: 24px;
  transition: all 0.3s ease;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  border-radius: 50%;
  transition: all 0.3s ease;
}

.toggle-switch input:checked + .toggle-slider {
  background: linear-gradient(135deg, var(--primary), var(--accent));
}

.toggle-switch input:checked + .toggle-slider::before {
  transform: translateX(20px);
}

.lang-select {
  padding: 6px 12px;
  font-size: 13px;
  min-width: 100px;
}
</style>
