import { defineConfig, type Plugin } from 'vite';
import baseConfig from './vite.config';

const stubApiUrl = process.env.E2E_STUB_API_URL || 'http://127.0.0.1:4174';
const apiProxy = {
  '/api/v1': {
    changeOrigin: false,
    secure: false,
    target: stubApiUrl,
  },
};
const localOnlyFonts: Plugin = {
  name: 'e2e-local-only-fonts',
  enforce: 'pre' as const,
  transform(source: string, id: string) {
    if (!id.endsWith('/src/styles/fonts.css')) return null;
    return source.replace(/@import\s+url\(['"]https:\/\/fonts\.googleapis\.com\/[^;]+;/g, '');
  },
  // Vite's CSS pipeline can inline nested imports without invoking transform
  // on the imported file. Strip remote fonts from final E2E CSS assets too.
  generateBundle: {
    order: 'post',
    handler(_options, bundle) {
      for (const output of Object.values(bundle)) {
        if (output.type === 'asset' && output.fileName.endsWith('.css') && typeof output.source === 'string') {
          output.source = output.source.replace(/@import\s+(?:url\(\s*)?['"]https:\/\/fonts\.googleapis\.com\/[^;]+;/g, '');
        }
      }
    },
  },
};

export default defineConfig({
  ...baseConfig,
  plugins: [localOnlyFonts, ...(baseConfig.plugins || [])],
  server: {
    ...baseConfig.server,
    // Keep browser requests same-origin so CORS preflights cannot bypass
    // Playwright routes. Requests not intercepted by a spec reach the local
    // deny-by-default server and fail the suite during teardown.
    proxy: apiProxy,
  },
  preview: {
    proxy: apiProxy,
  },
});
