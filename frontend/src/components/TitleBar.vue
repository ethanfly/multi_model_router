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
        <svg class="app-logo" width="18" height="18" viewBox="0 0 512 512" fill="none" aria-hidden="true">
          <rect width="512" height="512" rx="108" fill="#0f172a"/>
          <path d="M256 92L398 174V338L256 420L114 338V174L256 92Z" fill="#111827" stroke="#334155" stroke-width="24"/>
          <path d="M256 256V116M256 256H396M256 256V396M256 256H116" stroke="#38bdf8" stroke-width="42" stroke-linecap="round"/>
          <circle cx="256" cy="116" r="48" fill="#0f172a" stroke="#22d3ee" stroke-width="24"/>
          <circle cx="396" cy="256" r="48" fill="#0f172a" stroke="#60a5fa" stroke-width="24"/>
          <circle cx="256" cy="396" r="48" fill="#0f172a" stroke="#fbbf24" stroke-width="24"/>
          <circle cx="116" cy="256" r="48" fill="#0f172a" stroke="#34d399" stroke-width="24"/>
          <circle cx="256" cy="256" r="94" fill="#0f172a" stroke="#60a5fa" stroke-width="28"/>
          <path d="M256 184L322 222V290L256 328L190 290V222L256 184Z" fill="#e0f2fe"/>
          <path d="M256 214L296 237V279L256 302L216 279V237L256 214Z" fill="#0f172a"/>
          <circle cx="256" cy="256" r="17" fill="#67e8f9"/>
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
