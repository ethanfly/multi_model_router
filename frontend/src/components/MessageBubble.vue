<script lang="ts" setup>
defineProps<{
  message: {
    role: 'user' | 'assistant' | 'error'
    content: string
    modelName?: string
    complexity?: string
    routeMode?: string
    isError?: boolean
  }
}>()
</script>

<template>
  <div :class="['bubble', message.role, { error: message.isError }]">
    <div v-if="message.role === 'assistant'" class="bubble-meta">
      <span v-if="message.modelName" class="badge model-badge">{{ message.modelName }}</span>
      <span v-if="message.complexity" class="badge complexity-badge">{{ message.complexity }}</span>
      <span v-if="message.routeMode" class="badge route-badge">{{ message.routeMode }}</span>
    </div>
    <div class="bubble-content">{{ message.content }}</div>
  </div>
</template>

<style scoped>
.bubble {
  max-width: 70%;
  padding: 12px 16px;
  border-radius: var(--radius);
  word-wrap: break-word;
  white-space: pre-wrap;
  line-height: 1.6;
  font-size: 14px;
  animation: bubble-enter 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes bubble-enter {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* User: gradient bg, right-aligned */
.bubble.user {
  align-self: flex-end;
  background: linear-gradient(135deg, var(--primary), var(--secondary));
  color: #ffffff;
  border-bottom-right-radius: 4px;
  box-shadow: 0 2px 12px rgba(59, 130, 246, 0.2);
}

/* Assistant: surface bg with subtle border, left-aligned */
.bubble.assistant {
  align-self: flex-start;
  background-color: var(--bg);
  color: var(--text);
  border-bottom-left-radius: 4px;
  border: 1px solid rgba(71, 85, 105, 0.4);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

/* Error: error color border + tinted bg */
.bubble.error {
  align-self: center;
  background-color: rgba(239, 68, 68, 0.08);
  color: var(--error);
  border: 1px solid rgba(239, 68, 68, 0.3);
  border-radius: var(--radius-sm);
}

.bubble-meta {
  display: flex;
  gap: 6px;
  margin-bottom: 8px;
  flex-wrap: wrap;
}

.badge {
  display: inline-block;
  padding: 2px 10px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.3px;
}

.model-badge {
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.2), rgba(6, 182, 212, 0.2));
  color: var(--primary);
  border: 1px solid rgba(59, 130, 246, 0.2);
}

.complexity-badge {
  background: rgba(245, 158, 11, 0.12);
  color: var(--warning);
  border: 1px solid rgba(245, 158, 11, 0.2);
}

.route-badge {
  background: rgba(16, 185, 129, 0.12);
  color: var(--success);
  border: 1px solid rgba(16, 185, 129, 0.2);
}

.bubble-content {
  min-height: 1em;
}
</style>
