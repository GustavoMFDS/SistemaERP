// Public-facing brand only. Keep legacy storage namespaces, database identities,
// API paths, and Go module names stable when changing this value.
const configured = import.meta.env.VITE_APP_BRAND_NAME?.trim()
export const BRAND_NAME = configured && configured.length <= 48
  ? configured
  : 'SistemaEmGo'

export const BRAND_DESCRIPTION = 'Caixa, estoque e gestão da loja'
export const BRAND_PAGE_TITLE = BRAND_NAME + ' — Gestão da Loja'
