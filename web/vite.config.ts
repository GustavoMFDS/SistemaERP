import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig, loadEnv, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const appRoot = fileURLToPath(new URL('.', import.meta.url))
const manifestTemplatePath = fileURLToPath(new URL('./manifest.webmanifest.template.json', import.meta.url))
const defaultBrandName = 'Esterium ERP'

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[character] ?? character)
}

function publicBrandPlugin(brandName: string): Plugin {
  const manifestTemplate = readFileSync(manifestTemplatePath, 'utf8')
  const manifest = manifestTemplate.replaceAll('__APP_BRAND_NAME__', () =>
    JSON.stringify(brandName).slice(1, -1),
  )

  return {
    name: 'esterium-public-brand',
    transformIndexHtml(html) {
      return html.replaceAll('__APP_BRAND_NAME__', () => escapeHtml(brandName))
    },
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (request.url?.split('?')[0] !== '/manifest.webmanifest') {
          next()
          return
        }
        response.statusCode = 200
        response.setHeader('Content-Type', 'application/manifest+json; charset=utf-8')
        response.end(manifest)
      })
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'manifest.webmanifest', source: manifest })
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, appRoot, 'VITE_')
  const brandName = env.VITE_APP_BRAND_NAME?.trim() || defaultBrandName

  return {
    plugins: [react(), tailwindcss(), publicBrandPlugin(brandName)],
  }
})
