import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      // 开发期直连本机 API，避免 CORS
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
})
