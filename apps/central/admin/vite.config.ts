import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  // The site root by default; a path such as /console/ when the console shares
  // an origin with the login UI and the API (deploy/onprem).
  base: process.env.ADMIN_BASE_PATH || '/',
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    port: 3000,
    host: true,
    strictPort: true, // Fail if port is already in use
  },
})
