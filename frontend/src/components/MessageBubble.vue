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
  padding: 10px 14px;
  border-radius: 12px;
  word-wrap: break-word;
  white-space: pre-wrap;
  line-height: 1.5;
  font-size: 14px;
}

.bubble.user {
  align-self: flex-end;
  background-color: #3b82f6;
  color: #ffffff;
  border-bottom-right-radius: 4px;
}

.bubble.assistant {
  align-self: flex-start;
  background-color: #f3f4f6;
  color: #1f2937;
  border-bottom-left-radius: 4px;
}

.bubble.error {
  align-self: center;
  background-color: #fef2f2;
  color: #dc2626;
  border: 1px solid #dc2626;
  border-radius: 8px;
}

.bubble-meta {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
}

.model-badge {
  background-color: #dbeafe;
  color: #1d4ed8;
}

.complexity-badge {
  background-color: #fef3c7;
  color: #92400e;
}

.route-badge {
  background-color: #d1fae5;
  color: #065f46;
}

.bubble-content {
  min-height: 1em;
}
</style>
