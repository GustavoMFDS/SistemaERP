import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './smoke',
  testMatch: 'installed-lab.spec.ts',
  timeout: 45000,
  workers: 1,
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'https://localhost:8443',
    ignoreHTTPSErrors: process.env.E2E_IGNORE_HTTPS_ERRORS === '1',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
})
