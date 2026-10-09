import { useEffect, useMemo, useState } from 'react'
import { apiJson } from './api'
import { getSessionScope } from './auth'

// Fetch only small thumbnails, never originals, and clear on tenant/account change.
export function useProductThumbnails(productIDs: string[], refresh = 0): Record<string, string> {
  const key = productIDs.join(',')
  const [images, setImages] = useState<Record<string, string>>({})
  const ids = useMemo(() => key.split(',').filter(Boolean), [key])

  useEffect(() => {
    let cancelled = false
    const scope = getSessionScope()
    setImages({})
    if (ids.length === 0) return
    async function load() {
      try {
        const merged: Record<string, string> = {}
        for (let index = 0; index < ids.length; index += 70) {
          const params = new URLSearchParams()
          for (const id of ids.slice(index, index + 200)) params.append('id', id)
          const response = await apiJson<{ items: Record<string, string> }>(
            '/api/v1/products/images/previews?' + params.toString(),
          )
          Object.assign(merged, response.items)
        }
        if (!cancelled && scope === getSessionScope()) setImages(merged)
      } catch {
        // Photo lookup should never stop checkout, stock counts or barcode scanning.
        if (!cancelled && scope === getSessionScope()) setImages({})
      }
    }
    void load()
    return () => { cancelled = true }
  }, [ids, refresh])

  return images
}
