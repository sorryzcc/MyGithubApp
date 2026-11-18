import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 把 /api 代理到 Go REST 服务，避免浏览器跨域限制。
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
