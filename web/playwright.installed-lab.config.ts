import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './smoke',
  testMatch: 'installed-lab.spec.ts',
  timeout: 120_000,
  expect: { timeout: 15_000 },
  workers: 1,
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'https://esterium.ci.invalid:8443',
    ignoreHTTPSErrors: false,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    {
      name: 'firefox',
      use: {
        ...devices['Desktop Firefox'],
        launchOptions: {
          firefoxUserPrefs: { 'security.enterprise_roots.enabled': true },
        },
      },
    },
    {
      name: 'chromium-mobile',
      grep: /installed mobile/,
      use: {
        browserName: 'chromium',
        viewport: { width: 390, height: 844 },
        deviceScaleFactor: 2,
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
})

