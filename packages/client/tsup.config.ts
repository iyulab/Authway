import { defineConfig } from 'tsup'

export default defineConfig({
  entry: [
    'src/index.ts',
    'src/popup-callback.ts',
  ],
  format: ['cjs', 'esm'],
  // Explicit extensions so each format matches what package.json "exports"
  // advertises (.cjs for require, .mjs for import). Declarations come out as
  // .d.ts (CJS) and .d.mts (ESM) on their own.
  outExtension({ format }) {
    return { js: format === 'cjs' ? '.cjs' : '.mjs' }
  },
  dts: true,
  clean: true,
  sourcemap: true,
  minify: true,
  treeshake: true,
  splitting: false,
  external: [],
})
