import { expect, type Page } from '@playwright/test'

// The operator intentionally opens these sections. Keep the production UI
// collapsed by default and interact with it the same way a person would.
export async function expandDisclosure(page: Page, title: string): Promise<void> {
  const section = page.locator('details').filter({
    has: page.locator('summary').filter({ hasText: title }),
  }).first()
  await expect(section).toBeVisible()
  if ((await section.getAttribute('open')) === null) {
    await section.locator('summary').click()
  }
  await expect(section).toHaveAttribute('open', '')
}
