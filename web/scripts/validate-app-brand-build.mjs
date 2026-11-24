import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const distDir = resolve(webDir, 'dist')
const brandName = process.env.EXPECTED_APP_BRAND_NAME?.trim() || 'Esterium ERP'
const html = readFileSync(resolve(distDir, 'index.html'), 'utf8')
const manifest = JSON.parse(readFileSync(resolve(distDir, 'manifest.webmanifest'), 'utf8'))

assert.ok(html.includes('<title>' + brandName + ' — Gestão Comercial</title>'), 'built document title must use the public brand')
assert.ok(html.includes('content="' + brandName + ' — PDV, estoque, financeiro e gestão comercial"'), 'built description must use the public brand')
assert.equal(manifest.name, brandName + ' — Gestão Comercial')
assert.equal(manifest.short_name, brandName)
assert.ok(manifest.description.startsWith(brandName + ' — '), 'PWA description must use the public brand')
assert.ok(!html.includes('__APP_BRAND_NAME__'), 'built HTML must not contain the brand template token')
console.log('Built HTML metadata and PWA manifest use brand: ' + brandName)
