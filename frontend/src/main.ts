import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import './style.css'
import App from './App.vue'
import { registerAuthBridge, registerRouter } from './services/api'
import { useAuthStore } from './stores/auth'

async function start() {
  const app = createApp(App)
  app.use(createPinia())

  // Wire the late-bound dependencies the Axios interceptors need.
  const auth = useAuthStore()
  registerAuthBridge({
    getAccessToken: () => auth.accessToken,
    setAccessToken: (token) => auth.setAccessToken(token),
    refreshAccessToken: () => auth.refreshAccessToken(),
    reset: () => auth.reset(),
  })
  registerRouter(router)

  // Recover the session before the router's initial navigation resolves so
  // the first paint reflects the real auth state and we don't flash /login
  // on hard reload.
  await auth.bootstrap()

  app.use(router)
  await router.isReady()

  app.mount('#app')
}

void start()
