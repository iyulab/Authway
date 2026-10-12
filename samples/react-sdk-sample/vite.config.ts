import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // The origin registered for the client; a different port is a different origin.
  server: { port: 5173, strictPort: true },
})
