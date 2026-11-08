const configuredBrandName = import.meta.env.VITE_APP_BRAND_NAME?.trim()

export const APP_BRAND_NAME = configuredBrandName || 'Esterium ERP'
