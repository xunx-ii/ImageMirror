import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: '127.0.0.1',
    port: 25173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/v1': {
        target: 'http://localhost:8080',
        timeout: 310_000,
      },
      '/health': 'http://localhost:8080',
    },
  },
})
