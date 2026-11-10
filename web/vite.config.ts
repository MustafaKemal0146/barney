import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'

// Geliştirme: Go sunucusu 127.0.0.1:8777'de, `pnpm dev` :5173'te; /api vekillenir.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') } },
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://127.0.0.1:8777', changeOrigin: false } },
  },
  build: { outDir: 'dist', chunkSizeWarningLimit: 2000 },
  test: { environment: 'jsdom', css: false },
})
