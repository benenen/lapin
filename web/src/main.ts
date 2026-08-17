import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import 'primeicons/primeicons.css'
import 'katex/dist/katex.min.css'
import '@excalidraw/excalidraw/index.css'

import App from './App.vue'
import { router } from './router'
import './styles.css'

declare global {
  interface Window {
    EXCALIDRAW_ASSET_PATH?: string
  }
}

window.EXCALIDRAW_ASSET_PATH = '/excalidraw-assets/'

createApp(App)
  .use(router)
  .use(PrimeVue, {
    theme: {
      preset: Aura,
      options: { darkModeSelector: '.lapin-dark' },
    },
  })
  .mount('#app')
