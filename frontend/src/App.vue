<script lang="ts" setup>
</script>

<template>
  <div class="layout">
    <nav class="sidebar">
      <router-link to="/" class="nav-item" active-class="active" title="Chat">
        <span class="nav-icon">&#128172;</span>
      </router-link>
      <router-link to="/dashboard" class="nav-item" active-class="active" title="Dashboard">
        <span class="nav-icon">&#128202;</span>
      </router-link>
      <router-link to="/settings" class="nav-item" active-class="active" title="Settings">
        <span class="nav-icon">&#9881;</span>
      </router-link>
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
  width: 60px;
  min-width: 60px;
  background-color: #1f2937;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 12px;
  gap: 4px;
}

.nav-item {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 8px;
  text-decoration: none;
  color: #9ca3af;
  transition: background-color 0.2s, color 0.2s;
}

.nav-item:hover {
  background-color: #374151;
  color: #e5e7eb;
}

.nav-item.active {
  background-color: #3b82f6;
  color: #ffffff;
}

.nav-icon {
  font-size: 20px;
}

.main-content {
  flex: 1;
  overflow: auto;
  background-color: #111827;
}
</style>
