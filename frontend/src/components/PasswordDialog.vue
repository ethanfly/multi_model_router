<script lang="ts" setup>
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  title: string
  hint: string
  requireConfirm?: boolean
}>()

const emit = defineEmits<{
  confirm: [password: string]
  cancel: []
}>()

const password = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const mismatch = ref(false)

function handleSubmit() {
  if (props.requireConfirm && password.value !== confirmPassword.value) {
    mismatch.value = true
    return
  }
  mismatch.value = false
  emit('confirm', password.value)
}

function handleOverlayClick(e: MouseEvent) {
  if ((e.target as HTMLElement).classList.contains('modal-overlay')) {
    emit('cancel')
  }
}
</script>

<template>
  <div class="modal-overlay" @click="handleOverlayClick">
    <div class="modal">
      <h3>{{ title }}</h3>
      <p class="hint">{{ hint }}</p>

      <form @submit.prevent="handleSubmit" class="dialog-form">
        <div class="form-row">
          <div class="input-wrapper">
            <input
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="new-password"
              required
              autofocus
            />
            <button type="button" class="toggle-btn" @click="showPassword = !showPassword">
              {{ showPassword ? $t('modelEditor.cancel') : '***' }}
            </button>
          </div>
        </div>

        <div v-if="requireConfirm" class="form-row">
          <label>{{ $t('settings.confirmPassword') }}</label>
          <input
            v-model="confirmPassword"
            :type="showPassword ? 'text' : 'password'"
            autocomplete="new-password"
            required
          />
          <p v-if="mismatch" class="field-warning">{{ $t('settings.passwordMismatch') }}</p>
        </div>

        <div class="form-actions">
          <button type="button" @click="emit('cancel')" class="btn btn-cancel">{{ $t('modelEditor.cancel') }}</button>
          <button type="submit" class="btn btn-save">{{ $t('modelEditor.save') }}</button>
        </div>
      </form>
    </div>
  </div>
</template>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background-color: rgba(0, 0, 0, 0.6);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  animation: overlay-fade 0.2s ease;
}

@keyframes overlay-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

.modal {
  background-color: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 28px;
  width: 400px;
  max-height: 90vh;
  overflow-y: auto;
  color: var(--text);
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.4);
  animation: modal-enter 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

@keyframes modal-enter {
  from {
    opacity: 0;
    transform: translateY(12px) scale(0.97);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.modal h3 {
  margin: 0 0 12px 0;
  font-size: 20px;
  font-weight: 700;
  background: linear-gradient(135deg, var(--text), var(--text-secondary));
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hint {
  margin: 0 0 18px 0;
  font-size: 13px;
  color: var(--text-muted);
  line-height: 1.5;
}

.dialog-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-row label {
  margin-bottom: 2px;
  font-size: 12px;
  color: var(--text-muted);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.input-wrapper {
  display: flex;
  align-items: center;
  gap: 0;
}

.input-wrapper input {
  flex: 1;
  background-color: var(--surface);
  color: var(--text);
  border: 1px solid rgba(71, 85, 105, 0.5);
  border-radius: var(--radius-sm) 0 0 var(--radius-sm);
  padding: 10px 14px;
  font-size: 14px;
  font-family: inherit;
  height: 40px;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.input-wrapper input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2);
}

.toggle-btn {
  height: 40px;
  padding: 0 12px;
  background-color: var(--surface-light);
  border: 1px solid rgba(71, 85, 105, 0.5);
  border-left: none;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  color: var(--text-muted);
  font-size: 12px;
  cursor: pointer;
  white-space: nowrap;
}

.toggle-btn:hover {
  color: var(--text);
  background-color: var(--border);
}

.form-row input[type="password"],
.form-row input[type="text"] {
  background-color: var(--surface);
  color: var(--text);
  border: 1px solid rgba(71, 85, 105, 0.5);
  border-radius: var(--radius-sm);
  padding: 10px 14px;
  font-size: 14px;
  font-family: inherit;
  height: 40px;
  outline: none;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.form-row input:focus {
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2);
}

.form-row input::placeholder {
  color: var(--text-muted);
  opacity: 0.7;
}

.form-row input::-ms-reveal,
.form-row input::-ms-clear {
  display: none;
}

.field-warning {
  margin: 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--warning);
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 12px;
  border-top: 1px solid rgba(71, 85, 105, 0.4);
}

.btn {
  padding: 10px 24px;
  border: none;
  border-radius: var(--radius-sm);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-cancel {
  background-color: var(--surface-light);
  color: var(--text-secondary);
}

.btn-cancel:hover {
  background-color: var(--border);
  color: var(--text);
}

.btn-save {
  background: linear-gradient(135deg, var(--primary), var(--accent));
  color: #ffffff;
  box-shadow: 0 2px 12px rgba(59, 130, 246, 0.25);
}

.btn-save:hover {
  box-shadow: 0 4px 20px rgba(59, 130, 246, 0.4);
  transform: translateY(-1px);
}

.btn-save:active {
  transform: translateY(0);
}
</style>
