import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    // During `wails3 dev`, Wails proxies this port.
    port: 5173,
    strictPort: true,
  },
})
