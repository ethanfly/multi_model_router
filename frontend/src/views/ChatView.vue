<script lang="ts" setup>
import { ref, nextTick, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { SendChat } from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { useModelsStore } from '../stores/models'
import MessageBubble from '../components/MessageBubble.vue'

interface ChatMessage {
  role: 'user' | 'assistant' | 'error'
  content: string
  modelName?: string
  complexity?: string
  routeMode?: string
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

  EventsOn('chat:done', () => {
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
    await SendChat(main.ChatRequest.createFrom({
      messages: messages.value
        .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content))
        .map((m) => ({ role: m.role, content: m.content })),
      mode: mode.value,
      modelId: mode.value === 'manual' ? selectedModel.value : '',
    }))
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
        <label class="mode-option">
          <input type="radio" value="auto" v-model="mode" />
          <span>Auto</span>
        </label>
        <label class="mode-option">
          <input type="radio" value="manual" v-model="mode" />
          <span>Manual</span>
        </label>
        <label class="mode-option">
          <input type="radio" value="race" v-model="mode" />
          <span>Race</span>
        </label>
      </div>
      <select v-if="mode === 'manual'" v-model="selectedModel" class="model-select">
        <option value="" disabled>Select a model...</option>
        <option v-for="m in modelsStore.models" :key="m.id" :value="m.id">
          {{ m.name }} ({{ m.provider }})
        </option>
      </select>
    </div>

    <div class="messages" ref="messagesContainer">
      <div v-if="messages.length === 0" class="empty-state">
        <p>Send a message to start chatting</p>
      </div>
      <MessageBubble
        v-for="(msg, i) in messages"
        :key="i"
        :message="msg"
      />
    </div>

    <div class="input-bar">
      <textarea
        v-model="inputText"
        @keydown="handleKeydown"
        placeholder="Type a message... (Enter to send, Shift+Enter for new line)"
        rows="1"
        :disabled="isStreaming"
      ></textarea>
      <button @click="sendMessage" :disabled="isStreaming || !inputText.trim()" class="send-btn">
        {{ isStreaming ? '...' : 'Send' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.chat-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  color: #e5e7eb;
}

.top-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  background-color: #1f2937;
  border-bottom: 1px solid #374151;
}

.mode-selector {
  display: flex;
  gap: 8px;
}

.mode-option {
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  font-size: 14px;
}

.mode-option input {
  accent-color: #3b82f6;
}

.model-select {
  background-color: #374151;
  color: #e5e7eb;
  border: 1px solid #4b5563;
  border-radius: 6px;
  padding: 4px 8px;
  font-size: 14px;
  outline: none;
}

.model-select:focus {
  border-color: #3b82f6;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #6b7280;
  font-size: 16px;
}

.input-bar {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  background-color: #1f2937;
  border-top: 1px solid #374151;
}

.input-bar textarea {
  flex: 1;
  background-color: #374151;
  color: #e5e7eb;
  border: 1px solid #4b5563;
  border-radius: 8px;
  padding: 8px 12px;
  font-size: 14px;
  resize: none;
  outline: none;
  font-family: inherit;
  min-height: 38px;
  max-height: 120px;
}

.input-bar textarea:focus {
  border-color: #3b82f6;
}

.input-bar textarea:disabled {
  opacity: 0.6;
}

.send-btn {
  background-color: #3b82f6;
  color: white;
  border: none;
  border-radius: 8px;
  padding: 8px 20px;
  font-size: 14px;
  cursor: pointer;
  transition: background-color 0.2s;
  white-space: nowrap;
}

.send-btn:hover:not(:disabled) {
  background-color: #2563eb;
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
