import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    include: ['test/**/*.test.ts'],
    // Scenarios drive a live identity provider; each step is a network round trip.
    testTimeout: 30_000,
    hookTimeout: 60_000,
    // Scenarios share one provider and create real users and clients — run them serially.
    fileParallelism: false,
  },
})
