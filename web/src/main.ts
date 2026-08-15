import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import Aura from '@primevue/themes/aura'
import 'primeicons/primeicons.css'
import 'katex/dist/katex.min.css'
import 'tldraw/tldraw.css'

import App from './App.vue'
import './styles.css'

createApp(App)
  .use(PrimeVue, {
    theme: {
      preset: Aura,
      options: { darkModeSelector: '.lapin-dark' },
    },
  })
  .mount('#app')
