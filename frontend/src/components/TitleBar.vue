<script lang="ts" setup>
import { ref, onMounted } from 'vue'
import {
  MinimizeWindow,
  ToggleMaximizeWindow,
  HideWindow,
  IsWindowMaximized,
} from '../../wailsjs/go/main/App'

const isMaximized = ref(false)

onMounted(async () => {
  try { isMaximized.value = await IsWindowMaximized() } catch { /* ignore */ }
})

async function handleMinimize() {
  try { await MinimizeWindow() } catch { /* ignore */ }
}

async function handleMaximize() {
  try {
    await ToggleMaximizeWindow()
    isMaximized.value = await IsWindowMaximized()
  } catch { /* ignore */ }
}

async function handleClose() {
  try { await HideWindow() } catch { /* ignore */ }
}
</script>

<template>
  <div class="title-bar">
    <div class="drag-region">
      <div class="app-info">
        <svg class="app-logo" width="18" height="18" viewBox="0 0 512 512" fill="none">
          <rect width="512" height="512" rx="96" fill="#1e293b"/>
          <circle cx="256" cy="256" r="32" fill="none" stroke="#3b82f6" stroke-width="4"/>
          <circle cx="148" cy="148" r="24" fill="none" stroke="#3b82f6" stroke-width="2.5"/>
          <circle cx="364" cy="148" r="24" fill="none" stroke="#06b6d4" stroke-width="2.5"/>
          <circle cx="148" cy="364" r="24" fill="none" stroke="#8b5cf6" stroke-width="2.5"/>
          <circle cx="364" cy="364" r="24" fill="none" stroke="#f59e0b" stroke-width="2.5"/>
          <line x1="256" y1="256" x2="148" y2="148" stroke="#3b82f6" stroke-width="2" opacity="0.5"/>
          <line x1="256" y1="256" x2="364" y2="148" stroke="#06b6d4" stroke-width="2" opacity="0.5"/>
          <line x1="256" y1="256" x2="148" y2="364" stroke="#8b5cf6" stroke-width="2" opacity="0.5"/>
          <line x1="256" y1="256" x2="364" y2="364" stroke="#f59e0b" stroke-width="2" opacity="0.5"/>
        </svg>
        <span class="app-title">{{ $t('app.title') }}</span>
      </div>
    </div>
    <div class="window-controls" @mousedown.stop>
      <button class="ctrl-btn minimize" @click="handleMinimize" :title="$t('titlebar.minimize')">
        <svg width="10" height="10" viewBox="0 0 10 10">
          <line x1="0" y1="5" x2="10" y2="5" stroke="currentColor" stroke-width="1"/>
        </svg>
      </button>
      <button class="ctrl-btn maximize" @click="handleMaximize" :title="isMaximized ? $t('titlebar.restore') : $t('titlebar.maximize')">
        <svg v-if="!isMaximized" width="10" height="10" viewBox="0 0 10 10">
          <rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor" stroke-width="1"/>
        </svg>
        <svg v-else width="10" height="10" viewBox="0 0 10 10">
          <rect x="2.5" y="0" width="7" height="7" fill="none" stroke="currentColor" stroke-width="1"/>
          <rect x="0.5" y="2.5" width="7" height="7" fill="var(--bg, #0f172a)" stroke="currentColor" stroke-width="1"/>
        </svg>
      </button>
      <button class="ctrl-btn close" @click="handleClose" :title="$t('titlebar.hideToTray')">
        <svg width="10" height="10" viewBox="0 0 10 10">
          <line x1="0" y1="0" x2="10" y2="10" stroke="currentColor" stroke-width="1.2"/>
          <line x1="10" y1="0" x2="0" y2="10" stroke="currentColor" stroke-width="1.2"/>
        </svg>
      </button>
    </div>
  </div>
</template>

<style scoped>
.title-bar {
  display: flex;
  align-items: center;
  height: 36px;
  background: rgba(15, 23, 42, 0.95);
  border-bottom: 1px solid rgba(71, 85, 105, 0.3);
  user-select: none;
  flex-shrink: 0;
  --wails-draggable: drag;
}

.drag-region {
  flex: 1;
  height: 100%;
  display: flex;
  align-items: center;
  padding-left: 12px;
}

.app-info {
  display: flex;
  align-items: center;
  gap: 8px;
  pointer-events: none;
}

.app-logo {
  flex-shrink: 0;
}

.app-title {
  font-size: 12px;
  font-weight: 500;
  color: var(--text-muted, #94a3b8);
  letter-spacing: 0.02em;
}

.window-controls {
  display: flex;
  height: 100%;
  --wails-draggable: no-drag;
}

.ctrl-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 100%;
  background: transparent;
  border: none;
  color: var(--text-muted, #94a3b8);
  cursor: pointer;
  transition: all 0.15s ease;
}

.ctrl-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: var(--text, #e2e8f0);
}

.ctrl-btn.close:hover {
  background: #e81123;
  color: #ffffff;
}

.ctrl-btn svg {
  display: block;
}
</style>
