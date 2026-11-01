import { test as base, expect, type Page, type TestInfo } from '@playwright/test'

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

export async function attachVisualScreenshot(page: Page, testInfo: TestInfo, name: string) {
  const screenshotPath = testInfo.outputPath('tailwind-v4-' + name + '.png')
  await page.screenshot({ path: screenshotPath, fullPage: true })
  await testInfo.attach('tailwind-v4-' + name, {
    path: screenshotPath,
    contentType: 'image/png',
  })
}
