/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    // Serve the test document from the API's origin. The console calls the
    // API with credentials, and jsdom's XMLHttpRequest enforces CORS for
    // cross-origin calls (a preflight the mocked API does not answer). Same
    // origin also matches the single-domain deployment the console targets.
    environmentOptions: { jsdom: { url: 'http://localhost:8080' } },
    setupFiles: ['./src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      exclude: [
        'node_modules/',
        'src/test/',
        '**/*.d.ts',
        '**/*.config.*',
        'dist/',
        'coverage/'
      ],
      thresholds: {
        global: {
          branches: 100,
          functions: 100,
          lines: 100,
          statements: 100
        }
      }
    }
  },
  resolve: {
    alias: {
      '@': '/src'
    }
  }
})