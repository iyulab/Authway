import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 3001,
    host: true,
    strictPort: true, // Fail if port is already in use
    proxy: {
      // Proxy OAuth flow endpoints to Auth Backend
      '/logout': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      }
    }
  },
})