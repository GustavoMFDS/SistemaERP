import { test as base, expect } from '@playwright/test'

/**
 * Most E2E scenarios exercise normal store tasks, not the optional fiscal
 * onboarding prompt. Dismiss that real prompt as an operator would.
 * The dedicated fiscal-setup-reminder.spec.ts imports the raw Playwright
 * runner to assert the dialog is shown, hidden, and repeated correctly.
 */
export const test = base.extend({
  page: async ({ page }, runScenario) => {
    await page.addLocatorHandler(
      page.getByRole('dialog', { name: 'Sua configuração fiscal ainda não está concluída' }),
      async (dialog) => {
        await dialog.getByRole('button', { name: 'Agora não' }).click()
      },
    )
    await runScenario(page)
  },
})

export { expect }
