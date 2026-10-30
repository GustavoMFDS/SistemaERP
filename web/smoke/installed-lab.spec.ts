import { expect, test } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

test('installed Windows ERP renders login and home in Chromium', async ({ page }) => {
  const email = process.env.ESTERIUM_LAB_ADMIN_EMAIL ?? ''
  const password = process.env.ESTERIUM_LAB_ADMIN_PASSWORD ?? ''
  const evidenceDir = process.env.ESTERIUM_LAB_EVIDENCE_DIR ?? ''
  expect(process.env.ESTERIUM_INSTALLED_LAB).toBe('1')
  expect(process.env.E2E_IGNORE_HTTPS_ERRORS).toBe('1')
  expect(email).toMatch(/^ci-admin@esterium\.invalid$/)
  expect(password).toMatch(/^CIonly-[0-9a-f]{32}$/)
  expect(evidenceDir).not.toBe('')
  mkdirSync(evidenceDir, { recursive: true })

  const pageErrors: string[] = []
  page.on('pageerror', (error) => pageErrors.push(error.message))

  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Bem-vindo de volta' })).toBeVisible()
  await page.screenshot({ path: join(evidenceDir, 'installed-login.png'), fullPage: true })

  await page.getByLabel('E-mail ou nome de usuário').fill(email)
  await page.getByLabel('Senha', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Entrar', exact: true }).click()
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('navigation', { name: 'Navegação principal' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Resumo da semana' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'O ritmo da loja' })).toBeVisible()
  await page.screenshot({ path: join(evidenceDir, 'installed-home.png'), fullPage: true })

  expect(pageErrors).toEqual([])
})
