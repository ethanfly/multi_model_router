import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import './style.css'

const app = createApp(App)

// Vue error handler - logs errors instead of swallowing them
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Error]', info, err)
}

app.use(createPinia()).use(router).use(i18n).mount('#app')
