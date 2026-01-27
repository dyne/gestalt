import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

const defaultBackend = 'http://localhost:57417'
const backendTarget = process.env.GESTALT_BACKEND_URL || defaultBackend
const devMode = process.env.GESTALT_DEV_MODE === 'true'
const sourcemapOverride = process.env.GESTALT_FRONTEND_SOURCEMAP === 'true'
const enableSourcemap = devMode || sourcemapOverride
let backendUrl = null
let websocketUrl = null

try {
  backendUrl = new URL(backendTarget)
  websocketUrl = new URL(backendTarget)
  websocketUrl.protocol = backendUrl.protocol === 'https:' ? 'wss:' : 'ws:'
} catch {
  backendUrl = new URL(defaultBackend)
  websocketUrl = new URL(defaultBackend)
  websocketUrl.protocol = 'ws:'
}

// https://vite.dev/config/
export default defineConfig({
  define: {
    __GESTALT_VERSION__: JSON.stringify(process.env.VERSION || 'dev'),
  },
  plugins: [svelte()],
  build: {
    sourcemap: enableSourcemap,
  },
  server: {
    proxy: {
      '/api': {
        target: backendUrl.toString(),
        changeOrigin: true,
      },
      '/ws': {
        target: websocketUrl.toString(),
        ws: true,
        changeOrigin: true,
      },
    },
  },
})
