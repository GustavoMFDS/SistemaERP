import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import App from './App.tsx'
import { BRAND_PAGE_TITLE, BRAND_NAME, BRAND_DESCRIPTION } from './lib/branding'

document.title = BRAND_PAGE_TITLE
const appDescription = document.querySelector('meta[name="description"]')
appDescription?.setAttribute('content', BRAND_NAME + ' — ' + BRAND_DESCRIPTION)

if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/sw.js').catch(() => {
      // Installation is optional; failure must not block POS usage.
    })
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
)
