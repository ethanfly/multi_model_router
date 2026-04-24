<script lang="ts" setup>
import { ref, nextTick, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { SendChat } from '../../wailsjs/go/main/App'
import { core } from '../../wailsjs/go/models'
import { useModelsStore } from '../stores/models'
import MessageBubble from '../components/MessageBubble.vue'

interface ChatMessage {
  role: 'user' | 'assistant' | 'error'
  content: string
  modelName?: string
  complexity?: string
  routeMode?: string
  diagnostics?: string
  diagnosticsJson?: string
  isError?: boolean
}

const modelsStore = useModelsStore()

const messages = ref<ChatMessage[]>([])
const inputText = ref('')
const mode = ref('auto')
const selectedModel = ref('')
const isStreaming = ref(false)
const messagesContainer = ref<HTMLElement | null>(null)

onMounted(async () => {
  await modelsStore.fetchModels()

  EventsOn('chat:chunk', (data: any) => {
    const content = typeof data === 'string' ? data : data.content
    const lastMsg = messages.value[messages.value.length - 1]
    if (lastMsg && lastMsg.role === 'assistant') {
      lastMsg.content += content
      scrollToBottom()
    }
  })

  EventsOn('chat:done', (data: any) => {
    const lastMsg = messages.value[messages.value.length - 1]
    if (lastMsg && lastMsg.role === 'assistant' && data && typeof data !== 'string') {
      lastMsg.modelName = data.model || lastMsg.modelName
      if (data.diagnostics) {
        lastMsg.diagnostics = data.diagnostics
      }
      if (data.diagnosticsJson) {
        lastMsg.diagnosticsJson = data.diagnosticsJson
      }
    }
    isStreaming.value = false
  })

  EventsOn('chat:error', (data: any) => {
    const errMsg = typeof data === 'string' ? data : data.message || String(data)
    messages.value.push({
      role: 'error',
      content: errMsg,
      isError: true,
    })
    isStreaming.value = false
    scrollToBottom()
  })
})

onUnmounted(() => {
  EventsOff('chat:chunk')
  EventsOff('chat:done')
  EventsOff('chat:error')
})

async function sendMessage() {
  const text = inputText.value.trim()
  if (!text || isStreaming.value) return

  messages.value.push({ role: 'user', content: text })
  inputText.value = ''
  isStreaming.value = true

  messages.value.push({ role: 'assistant', content: '' })

  scrollToBottom()

  try {
    const resp = await SendChat(core.ChatRequest.createFrom({
      messages: messages.value
        .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content))
        .map((m) => ({ role: m.role, content: m.content })),
      mode: mode.value,
      modelId: mode.value === 'manual' ? selectedModel.value : '',
    }))
    if (resp && resp.status === 'error') {
      // Remove the empty assistant placeholder
      const lastIdx = messages.value.length - 1
      if (lastIdx >= 0 && messages.value[lastIdx].role === 'assistant' && !messages.value[lastIdx].content) {
        messages.value.splice(lastIdx, 1)
      }
      messages.value.push({
        role: 'error',
        content: resp.error || 'Unknown error',
        isError: true,
      })
      isStreaming.value = false
      scrollToBottom()
    } else if (resp && resp.status === 'success') {
      const lastMsg = messages.value[messages.value.length - 1]
      if (lastMsg && lastMsg.role === 'assistant') {
        lastMsg.modelName = resp.modelName
        lastMsg.complexity = resp.complexity
        lastMsg.routeMode = resp.routeMode
        lastMsg.diagnostics = resp.diagnostics
        lastMsg.diagnosticsJson = resp.diagnosticsJson
      }
    }
  } catch (err: any) {
    messages.value.push({
      role: 'error',
      content: err.message || String(err),
      isError: true,
    })
    isStreaming.value = false
  }

  scrollToBottom()
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  })
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}
</script>

<template>
  <div class="chat-view">
    <div class="top-bar">
      <div class="mode-selector">
        <label :class="['mode-pill', { active: mode === 'auto' }]">
          <input type="radio" value="auto" v-model="mode" />
          <span>{{ $t('chat.modeAuto') }}</span>
        </label>
        <label :class="['mode-pill', { active: mode === 'manual' }]">
          <input type="radio" value="manual" v-model="mode" />
          <span>{{ $t('chat.modeManual') }}</span>
        </label>
        <label :class="['mode-pill', { active: mode === 'race' }]">
          <input type="radio" value="race" v-model="mode" />
          <span>{{ $t('chat.modeRace') }}</span>
        </label>
      </div>
      <select v-if="mode === 'manual'" v-model="selectedModel" class="model-select">
        <option value="" disabled>{{ $t('chat.selectModel') }}</option>
        <option v-for="m in modelsStore.models" :key="m.id" :value="m.id">
          {{ m.name }} ({{ m.provider }})
        </option>
      </select>
    </div>

    <div class="messages" ref="messagesContainer">
      <div v-if="messages.length === 0" class="empty-state">
        <div class="empty-icon">
          <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
          </svg>
        </div>
        <p>{{ $t('chat.emptyState') }}</p>
      </div>
      <MessageBubble
        v-for="(msg, i) in messages"
        :key="i"
        :message="msg"
      />
      <div v-if="isStreaming" class="streaming-indicator">
        <span class="dot"></span>
        <span class="dot"></span>
        <span class="dot"></span>
      </div>
    </div>

    <div class="input-bar">
      <textarea
        v-model="inputText"
        @keydown="handleKeydown"
        :placeholder="$t('chat.placeholder')"
        rows="1"
        :disabled="isStreaming"
      ></textarea>
      <button @click="sendMessage" :disabled="isStreaming || !inputText.trim()" class="send-btn">
        <svg v-if="!isStreaming" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="22" y1="2" x2="11" y2="13" />
          <polygon points="22 2 15 22 11 13 2 9 22 2" />
        </svg>
        <span v-else class="btn-dots">
          <span class="dot"></span>
          <span class="dot"></span>
          <span class="dot"></span>
        </span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.chat-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  color: var(--text);
}

/* Glass top bar */
.top-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 20px;
  background: rgba(30, 41, 59, 0.7);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border-bottom: 1px solid rgba(71, 85, 105, 0.4);
  border-radius: var(--radius) var(--radius) 0 0;
}

/* Segmented control pills */
.mode-selector {
  display: flex;
  background: var(--bg);
  border-radius: 20px;
  padding: 3px;
  border: 1px solid var(--border);
}

.mode-pill {
  position: relative;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  border-radius: 17px;
  transition: all 0.2s ease;
}

.mode-pill input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.mode-pill span {
  display: block;
  padding: 6px 16px;
  border-radius: 17px;
  color: var(--text-muted);
  transition: all 0.2s ease;
}

.mode-pill:hover span {
  color: var(--text-secondary);
}

.mode-pill.active span {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #ffffff;
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.3);
}

.model-select {
  padding: 6px 12px;
  font-size: 13px;
  min-width: 160px;
}


/* Messages area with subtle gradient */
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.3) 0%, rgba(30, 41, 59, 0.1) 100%);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  gap: 12px;
}

.empty-icon {
  opacity: 0.4;
}

.empty-state p {
  font-size: 15px;
}

/* Streaming indicator */
.streaming-indicator {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 8px 14px;
  align-self: flex-start;
}

.streaming-indicator .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--primary);
  animation: pulse-dot 1.4s ease-in-out infinite;
}

.streaming-indicator .dot:nth-child(2) {
  animation-delay: 0.2s;
}

.streaming-indicator .dot:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes pulse-dot {
  0%, 80%, 100% {
    opacity: 0.3;
    transform: scale(0.8);
  }
  40% {
    opacity: 1;
    transform: scale(1.2);
  }
}

/* Elevated input bar */
.input-bar {
  display: flex;
  gap: 10px;
  padding: 16px 20px;
  background: var(--surface);
  border-top: 1px solid rgba(71, 85, 105, 0.3);
}

.input-bar textarea {
  flex: 1;
  background-color: var(--bg);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 12px 16px;
  font-size: 14px;
  resize: none;
  outline: none;
  font-family: inherit;
  min-height: 44px;
  max-height: 120px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
  line-height: 1.5;
}

.input-bar textarea:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

.input-bar textarea:disabled {
  opacity: 0.5;
}

.input-bar textarea::placeholder {
  color: var(--text-muted);
}

/* Gradient send button */
.send-btn {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: white;
  border: none;
  border-radius: var(--radius);
  padding: 0 20px;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
  white-space: nowrap;
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 48px;
  box-shadow: 0 2px 12px rgba(59, 130, 246, 0.25);
}

.send-btn:hover:not(:disabled) {
  box-shadow: 0 4px 20px rgba(59, 130, 246, 0.4);
  transform: translateY(-1px);
}

.send-btn:active:not(:disabled) {
  transform: translateY(0);
}

.send-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  transform: none;
}

.send-btn .btn-dots {
  display: flex;
  gap: 3px;
  align-items: center;
}

.send-btn .btn-dots .dot {
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: white;
  animation: pulse-dot 1.4s ease-in-out infinite;
}

.send-btn .btn-dots .dot:nth-child(2) {
  animation-delay: 0.2s;
}

.send-btn .btn-dots .dot:nth-child(3) {
  animation-delay: 0.4s;
}
</style>
