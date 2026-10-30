import { readFileSync } from 'node:fs';
import { defineConfig, devices } from '@playwright/test';

interface SuiteManifest {
  ciCritical: string[];
  mockedUiDeferred: string[];
  fullStack: string[];
}

const suites = JSON.parse(
  readFileSync(new URL('./e2e/suites.json', import.meta.url), 'utf8'),
) as SuiteManifest;
const appPort = process.env.E2E_APP_PORT || '4173';
const stubApiPort = process.env.E2E_STUB_API_PORT || '4174';
const appUrl = `http://127.0.0.1:${appPort}`;
const stubApiUrl = `http://127.0.0.1:${stubApiPort}`;
const browserChannel = process.env.E2E_BROWSER_CHANNEL as 'chrome' | undefined;
const selectedSpecs = process.env.E2E_INCLUDE_DEFERRED === '1'
  ? [...suites.ciCritical, ...suites.mockedUiDeferred]
  : suites.ciCritical;

export default defineConfig({
  testDir: './e2e',
  // This is an allowlist on purpose: an unclassified full-stack spec must never
  // start calling external infrastructure from a pull-request workflow.
  testMatch: selectedSpecs.map((file) => `**/${file}`),
  fullyParallel: true,
  forbidOnly: true,
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? 'github' : 'list',
  outputDir: 'test-results/deterministic-ui',
  globalSetup: './e2e/fixtures/verify-isolated-stack.ts',
  globalTeardown: './e2e/fixtures/assert-isolated-stack.ts',
  use: {
    baseURL: appUrl,
    // Loopback is the only network boundary allowed. Any accidental HTTP(S)
    // request to another host is sent to the containment server and recorded.
    proxy: {
      server: stubApiUrl,
      bypass: '127.0.0.1,localhost',
    },
    screenshot: 'only-on-failure',
    serviceWorkers: 'block',
    trace: 'retain-on-failure',
    video: 'off',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], ...(browserChannel ? { channel: browserChannel } : {}) },
    },
  ],
  webServer: [
    {
      command: 'node scripts/start-isolated-e2e-app.mjs',
      env: {
        E2E_STUB_API_URL: stubApiUrl,
        VITE_API_URL: '/api/v1',
        VITE_API_DEBUG: 'false',
        VITE_MEDIA_URL: stubApiUrl,
        VITE_SUPABASE_URL: stubApiUrl,
      },
      reuseExistingServer: false,
      timeout: 120_000,
      url: appUrl,
    },
    {
      command: 'node e2e/fixtures/isolated-api-server.mjs',
      env: {
        E2E_SEED: process.env.E2E_SEED || 'suitee-ci-v1',
        E2E_STUB_API_PORT: stubApiPort,
      },
      reuseExistingServer: false,
      timeout: 30_000,
      url: `${stubApiUrl}/__e2e/seed`,
    },
  ],
});
