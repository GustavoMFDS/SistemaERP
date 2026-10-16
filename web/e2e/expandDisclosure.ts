import { expect, type Page } from '@playwright/test'

// A data-dependent section may mount again after the permission request.
// Retrying the whole action prevents checking a stale <details> node while
// keeping the same user action and never forcing the production panel open.
export async function expandDisclosure(page: Page, title: string): Promise<void> {
  const section = page.locator('details').filter({
    has: page.locator('summary').filter({ hasText: title }),
  }).first()
  await expect(async () => {
    await expect(section).toBeVisible({ timeout: 3000 })
    if ((await section.getAttribute('open', { timeout: 3000 })) === null) {
      await section.locator('summary').click({ timeout: 3000 })
    }
    await expect(section).toHaveAttribute('open', '', { timeout: 3000 })
  }).toPass({ timeout: 15000, intervals: [200, 500, 1000] })
}
