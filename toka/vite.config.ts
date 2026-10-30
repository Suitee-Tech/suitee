/// <reference types="vitest" />
// vitest/config re-exports vite's defineConfig but extends UserConfig with
// a `test` field — using this entry point is the supported way to keep
// type-checking happy when the same file configures Vite + Vitest.
import { defineConfig } from 'vitest/config'
import path from 'path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { ViteImageOptimizer } from 'vite-plugin-image-optimizer'

const buildMinifier = process.env.BUILD_MINIFY === 'terser' ? 'terser' : 'esbuild'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    ViteImageOptimizer({
      png: { quality: 80 },
      jpg: { quality: 80 },
      jpeg: { quality: 80 },
      webp: { lossless: false, quality: 80 },
      logStats: true,
    }),
  ],
  server: {
    proxy: {
      '/api/v1/ai': {
        target: 'http://localhost:8092',
        changeOrigin: true,
        secure: false,
      },
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
      },
      // Keep legacy uploads on the same local path used by nginx.conf:
      // Vite -> gateway -> toca-comm (/uploads).
      '/uploads': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        secure: false,
      },
    },
  },
  resolve: {
    alias: {
      // Alias @ to the src directory
      '@': path.resolve(__dirname, './src'),
    },
    // dedupe: ['yjs', 'konva'],
  },
  build: {
    // Ensure clean builds
    manifest: true,
    // Let the bundler isolate dependencies of lazy routes. Grouping all editor
    // and PDF modules manually also pulled their shared helpers into the entry.
    // Sometimes build cache is corrupted
    minify: buildMinifier,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/setupTests.ts',
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['e2e/**', 'node_modules/**'],
  },
  assetsInclude: ['**/*.svg', '**/*.csv'],
})
