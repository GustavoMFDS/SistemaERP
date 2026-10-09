import { useCallback, useEffect, useState } from 'react'
import type { ChangeEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Photo = {
  id: string
  data_url: string
  thumbnail_url: string
  principal: boolean
  caption: string
}

async function resizedJPEG(file: File, maxDimension: number, maxBytes: number) {
  const bitmap = await createImageBitmap(file)
  try {
    const ratio = Math.min(1, maxDimension / Math.max(bitmap.width, bitmap.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(bitmap.width * ratio))
    canvas.height = Math.max(1, Math.round(bitmap.height * ratio))
    const context = canvas.getContext('2d')
    if (!context) throw new Error('O navegador não conseguiu preparar a imagem.')
    context.fillStyle = '#fff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
    for (const quality of [0.85, 0.73, 0.6, 0.45]) {
      const url = canvas.toDataURL('image/jpeg', quality)
      const encoded = url.split(',')[1] ?? ''
      if (Math.floor(encoded.length * 3 / 4) <= maxBytes) return encoded
    }
    throw new Error('A imagem ficou grande demais. Tente uma foto mais simples.')
  } finally {
    bitmap.close()
  }
}

// Derive a stable UUID from this photo and store scope to make resubmissions
// idempotent even after ambiguous network failures and browser reloads.
async function imageAttemptKey(encodedJPEG: string, scope: string, productID: string): Promise<string> {
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256',
    new TextEncoder().encode(scope + ':' + productID + ':' + encodedJPEG)))
  const bytes = digest.slice(0, 16)
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)].join('-')
}

export default function ProductPhotos({ productId, productName, canWrite, onChange }: {
  productId: string
  productName: string
  canWrite: boolean
  onChange?: () => void
}) {
  const [photos, setPhotos] = useState<Photo[]>([])
  const [captions, setCaptions] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    const scope = getSessionScope()
    setLoading(true)
    try {
      const result = await apiJson<{ items: Photo[] }>(
        '/api/v1/products/' + encodeURIComponent(productId) + '/images',
      )
      if (scope !== getSessionScope()) return
      setPhotos(result.items ?? [])
      setCaptions(Object.fromEntries((result.items ?? []).map((photo) => [photo.id, photo.caption ?? ''])))
      setError('')
    } catch (e: unknown) {
      if (scope === getSessionScope()) setError(errorMessage(e))
    } finally {
      if (scope === getSessionScope()) setLoading(false)
    }
  }, [productId])

  useEffect(() => { void load() }, [load])

  async function choosePhotos(event: ChangeEvent<HTMLInputElement>) {
    const selected = Array.from(event.target.files ?? [])
    event.target.value = ''
    if (!canWrite || busy || selected.length === 0) return
    if (selected.length > 5 - photos.length) {
      setError('Você pode adicionar até cinco fotos por produto.')
      return
    }
    if (selected.some((f) => !['image/jpeg', 'image/png', 'image/webp'].includes(f.type) || f.size > 10 * 1024 * 1024)) {
      setError('Escolha arquivos JPG, PNG ou WebP com até 10 MB cada.')
      return
    }
    const scope = getSessionScope()
    setBusy(true)
    setError('')
    try {
      for (const file of selected) {
        // The browser converts the uploaded photo into a bounded JPEG and thumbnail.
        const image_base64 = await resizedJPEG(file, 900, 256 * 1024)
        const thumbnail_base64 = await resizedJPEG(file, 120, 12 * 1024)
        if (scope !== getSessionScope()) return
        const uploadKey = await imageAttemptKey(image_base64, scope, productId)
        await apiJson<{ id: string }>('/api/v1/products/' + encodeURIComponent(productId) + '/images', {
          method: 'POST',
          headers: { 'Idempotency-Key': uploadKey },
          body: { image_base64, thumbnail_base64 },
        })
      }
      if (scope === getSessionScope()) {
        await load()
        onChange?.()
      }
    } catch (e: unknown) {
      if (scope === getSessionScope()) {
        setError('Não foi possível confirmar todas as fotos: ' + errorMessage(e) + '. Confira a galeria antes de enviar novamente.')
        await load()
        onChange?.()
      }
    } finally {
      if (scope === getSessionScope()) setBusy(false)
    }
  }

  async function saveCaption(id: string) {
    if (!canWrite || busy) return
    const scope = getSessionScope()
    setBusy(true)
    setError('')
    try {
      await apiJson('/api/v1/products/' + encodeURIComponent(productId) + '/images/' + encodeURIComponent(id) + '/caption', {
        method: 'PATCH',
        body: { caption: (captions[id] ?? '').trim() },
      })
      if (scope === getSessionScope()) await load()
    } catch (e: unknown) {
      if (scope === getSessionScope()) setError(errorMessage(e))
    } finally {
      if (scope === getSessionScope()) setBusy(false)
    }
  }

  async function deletePhoto(id: string) {
    if (!canWrite || busy || !window.confirm('Excluir esta foto do produto?')) return
    const scope = getSessionScope()
    setBusy(true)
    setError('')
    try {
      await apiJson('/api/v1/products/' + encodeURIComponent(productId) + '/images/' + encodeURIComponent(id), {
        method: 'DELETE',
      })
      if (scope === getSessionScope()) {
        await load()
        onChange?.()
      }
    } catch (e: unknown) {
      if (scope === getSessionScope()) setError(errorMessage(e))
    } finally {
      if (scope === getSessionScope()) setBusy(false)
    }
  }

  return (
    <section aria-label={'Fotos de ' + productName} className="space-y-3 rounded-xl bg-slate-50 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="font-semibold text-slate-800">Fotos — {productName}</h3>
          <p className="text-xs text-slate-600">Opcional. Identifique cada foto por cor ou modelo. Isso não divide o estoque. A primeira imagem é a principal.</p>
        </div>
        {canWrite && photos.length < 5 ? (
          <label className="cursor-pointer rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm hover:bg-slate-100">
            {busy ? 'Enviando…' : 'Adicionar fotos'}
            <input type="file" accept="image/jpeg,image/png,image/webp" multiple disabled={busy}
              onChange={(event) => void choosePhotos(event)} className="sr-only" />
          </label>
        ) : null}
      </div>
      {error ? <p role="alert" className="text-sm text-red-700">{error}</p> : null}
      {loading ? <p className="text-sm text-slate-600">Carregando fotos…</p> : null}
      {!loading && photos.length === 0 ? <p className="text-sm text-slate-600">Este produto ainda não tem fotos.</p> : null}
      <div className="flex flex-wrap gap-3">
        {photos.map((photo) => (
          <div key={photo.id} className="w-36 rounded-lg border border-slate-200 bg-white p-2">
            <a href={photo.data_url} target="_blank" rel="noopener noreferrer"
              aria-label={'Ver foto de ' + productName}>
              <img src={photo.thumbnail_url} alt={'Foto de ' + productName}
                className="h-28 w-full rounded-md object-contain" />
            </a>
            <p className="mt-1 text-center text-xs text-slate-600">{photo.principal ? 'Foto principal' : 'Foto adicional'}</p>
            {canWrite ? (
              <div className="mt-2 space-y-1">
                <label className="block text-xs text-slate-600" htmlFor={'photo-caption-' + photo.id}>
                  Cor ou modelo
                </label>
                <input id={'photo-caption-' + photo.id} type="text" maxLength={40}
                  value={captions[photo.id] ?? photo.caption ?? ''}
                  placeholder="Ex.: Azul" disabled={busy}
                  onChange={(e) => setCaptions((old) => ({ ...old, [photo.id]: e.target.value }))}
                  className="w-full rounded-md border border-slate-300 p-1 text-xs" />
                <button type="button" disabled={busy || (captions[photo.id] ?? '').trim() === photo.caption}
                  onClick={() => void saveCaption(photo.id)}
                  className="w-full rounded-md border border-slate-300 px-2 py-1 text-xs disabled:opacity-50">Salvar nome</button>
              </div>
            ) : photo.caption ? <p className="mt-1 text-center text-xs">{photo.caption}</p> : null}
            {canWrite ? <button type="button" disabled={busy} onClick={() => void deletePhoto(photo.id)}
              className="mt-1 w-full rounded-md border border-red-200 px-2 py-1 text-xs text-red-700 disabled:opacity-50">
              Excluir
            </button> : null}
          </div>
        ))}
      </div>
    </section>
  )
}
