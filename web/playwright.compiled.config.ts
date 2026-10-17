import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './smoke',
  timeout: 30000,
  workers: 1,
  use: {
    baseURL: 'http://127.0.0.1:18080',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
