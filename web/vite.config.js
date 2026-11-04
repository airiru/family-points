import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    // 显式绑 IPv4：默认只监听 [::1]，某些工具/代理只认 127.0.0.1 会连不上
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      // 开发期直连本机 API，避免 CORS
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
})
