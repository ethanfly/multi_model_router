<script lang="ts" setup>
</script>

<template>
  <div class="layout">
    <nav class="sidebar">
      <div class="nav-logo">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <path d="M12 2L2 7l10 5 10-5-10-5z" fill="url(#logoGrad)" />
          <path d="M2 17l10 5 10-5" stroke="url(#logoGrad)" stroke-width="2" fill="none" />
          <path d="M2 12l10 5 10-5" stroke="url(#logoGrad)" stroke-width="2" fill="none" />
          <defs>
            <linearGradient id="logoGrad" x1="2" y1="2" x2="22" y2="22">
              <stop offset="0%" stop-color="#3B82F6" />
              <stop offset="100%" stop-color="#06B6D4" />
            </linearGradient>
          </defs>
        </svg>
      </div>
      <div class="nav-items">
        <router-link to="/" class="nav-item" active-class="active" data-tooltip="Chat">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
          </svg>
        </router-link>
        <router-link to="/dashboard" class="nav-item" active-class="active" data-tooltip="Dashboard">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="12" width="4" height="9" rx="1" />
            <rect x="10" y="7" width="4" height="14" rx="1" />
            <rect x="17" y="3" width="4" height="18" rx="1" />
          </svg>
        </router-link>
        <router-link to="/settings" class="nav-item" active-class="active" data-tooltip="Settings">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="3" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
          </svg>
        </router-link>
      </div>
    </nav>
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
}

.sidebar {
  width: 72px;
  min-width: 72px;
  background-color: var(--bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 16px;
  gap: 4px;
  border-right: 1px solid rgba(71, 85, 105, 0.3);
}

.nav-logo {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  margin-bottom: 16px;
}

.nav-items {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: var(--radius-sm);
  text-decoration: none;
  color: var(--text-muted);
  transition: all 0.2s ease;
}

.nav-item:hover {
  background-color: var(--surface-light);
  color: var(--text);
}

/* Tooltip via CSS */
.nav-item::after {
  content: attr(data-tooltip);
  position: absolute;
  left: calc(100% + 12px);
  top: 50%;
  transform: translateY(-50%);
  background-color: var(--surface-light);
  color: var(--text);
  padding: 4px 10px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
  z-index: 100;
  box-shadow: var(--shadow);
}

.nav-item:hover::after {
  opacity: 1;
}

.nav-item.active {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #ffffff;
  box-shadow: 0 2px 12px rgba(59, 130, 246, 0.3);
}

.nav-item.active:hover {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #ffffff;
}

.main-content {
  flex: 1;
  overflow: auto;
  background-color: var(--surface);
  border-top-left-radius: var(--radius);
}
</style>
