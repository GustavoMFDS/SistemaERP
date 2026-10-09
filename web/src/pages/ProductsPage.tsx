import { Fragment, useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { APIError, apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'
import ImportBatchHistory from '../components/ImportBatchHistory'
import ProductPhotos from '../components/ProductPhotos'
import ProductFamily from '../components/ProductFamily'
import { useProductThumbnails } from '../lib/productThumbnails'
import { clearPendingProductImport, fingerprintProducts, readPendingProductImport, savePendingProductImport, type PendingProductImport } from '../lib/productImportRecovery'
import { parseProductCSV, PRODUCT_IMPORT_EXAMPLE, type ProductImportPreview } from '../lib/productImport'

type Product = {
  id: string
  sku: string
  name: string
  unit: string
  category_id?: string | null
  barcode?: string | null
  ncm?: string | null
  cest?: string | null
  description?: string | null
  cost_price: number
  price_cash: number
  promo_price?: number | null
  min_stock: number
  active: boolean
  qty_on_hand: number
}

type ListResponse = { items: Product[]; total: number }

type ProductCreateRequest = {
  category_id?: string | null
  sku: string
  barcode?: string | null
  ncm?: string | null
  cest?: string | null
  name: string
  description?: string | null
  unit: string
  cost_price: number
  price_cash: number
  promo_price?: number | null
  min_stock: number
  active: boolean
}

export default function ProductsPage() {
  const [query, setQuery] = useState('')
  const [onlyLowStock, setOnlyLowStock] = useState(false)
  const [showTechnical, setShowTechnical] = useState(false)
  const [photoProductId, setPhotoProductId] = useState('')
  const [familyProductId, setFamilyProductId] = useState('')
  const manualFormRef = useRef<HTMLDetailsElement>(null)
  const skuInputRef = useRef<HTMLInputElement>(null)
  const [variationSource, setVariationSource] = useState('')
  const [variationParentId, setVariationParentId] = useState('')
  const [variationOptionLabel, setVariationOptionLabel] = useState('')
  const [photoVersion, setPhotoVersion] = useState(0)
  const [items, setItems] = useState<Product[]>([])
  const [barcodeDrafts, setBarcodeDrafts] = useState<Record<string, string>>({})
  const [ncmDrafts, setNcmDrafts] = useState<Record<string, string>>({})
  const [cestDrafts, setCestDrafts] = useState<Record<string, string>>({})
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [canWrite, setCanWrite] = useState(false)
  const [importPreview, setImportPreview] = useState<ProductImportPreview | null>(null)
  const [importing, setImporting] = useState(false)
  const [importResult, setImportResult] = useState('')
  const [importKey, setImportKey] = useState('')
  const [importDigest, setImportDigest] = useState('')
  const [importScope, setImportScope] = useState('')
  const [pendingImport, setPendingImport] = useState<PendingProductImport | null>(null)
  const [historyRefresh, setHistoryRefresh] = useState(0)

  const [sku, setSku] = useState('')
  const [barcode, setBarcode] = useState('')
  const [ncm, setNcm] = useState('')
  const [cest, setCest] = useState('')
  const [name, setName] = useState('')
  const [unit, setUnit] = useState('un')
  const [priceCash, setPriceCash] = useState<number>(0)
  const [minStock, setMinStock] = useState<number>(0)

  const lowStockItems = items.filter((p) => p.active && p.min_stock > 0 && p.qty_on_hand <= p.min_stock)
  const visibleItems = onlyLowStock ? lowStockItems : items
  const thumbnails = useProductThumbnails(visibleItems.map((item) => item.id), photoVersion)

  const canCreate = useMemo(() => sku.trim() && name.trim() && priceCash > 0, [
    sku,
    name,
    priceCash,
  ])

  async function load(nextQuery = query) {
    setError('')
    setLoading(true)
    try {
      const qs = new URLSearchParams()
      if (nextQuery.trim()) qs.set('query', nextQuery.trim())
      const data = await apiJson<ListResponse>(`/api/v1/products?${qs.toString()}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((me) => {
        const writable = me.permissions.includes('product:write')
        setCanWrite(writable)
        setPendingImport(writable ? readPendingProductImport() : null)
      })
      .catch((e: unknown) => setError(errorMessage(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => { void load() }, 300)
    return () => window.clearTimeout(timer)
    // Search by name, SKU or barcode without requiring a second click.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query])

  async function saveCatalogFiscal(product: Product) {
    setError('')
    const nextBarcode = (barcodeDrafts[product.id] ?? product.barcode ?? '').trim()
    const nextNCM = (ncmDrafts[product.id] ?? product.ncm ?? '').trim()
    const nextCEST = (cestDrafts[product.id] ?? product.cest ?? '').trim()
    try {
      const payload: ProductCreateRequest = {
        category_id: product.category_id ?? null,
        sku: product.sku,
        barcode: nextBarcode || null,
        ncm: nextNCM || null,
        cest: nextCEST || null,
        name: product.name,
        description: product.description ?? null,
        unit: product.unit,
        cost_price: product.cost_price,
        price_cash: product.price_cash,
        promo_price: product.promo_price ?? null,
        min_stock: product.min_stock,
        active: product.active,
      }
      await apiJson<{ id: string }>(`/api/v1/products/${product.id}`, {
        method: 'PUT',
        body: payload,
      })
      setBarcodeDrafts((prev) => {
        const next = { ...prev }
        delete next[product.id]
        return next
      })
      setNcmDrafts((prev) => {
        const next = { ...prev }
        delete next[product.id]
        return next
      })
      setCestDrafts((prev) => {
        const next = { ...prev }
        delete next[product.id]
        return next
      })
      await load()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!canCreate) return
    setError('')
    try {
      const payload: ProductCreateRequest = {
        sku: sku.trim(),
        barcode: barcode.trim() || null,
        ncm: ncm.trim() || null,
        cest: cest.trim() || null,
        name: name.trim(),
        unit: unit.trim() || 'un',
        cost_price: 0,
        price_cash: Number(priceCash),
        min_stock: Number(minStock) || 0,
        active: true,
      }
      const created = await apiJson<{ id: string }>(
        variationParentId ? '/api/v1/products/' + encodeURIComponent(variationParentId) + '/variations' : '/api/v1/products', {
          method: 'POST',
          body: variationParentId ? { option_label: variationOptionLabel, product: payload } : payload,
        },
      )
      setSku('')
      setBarcode('')
      setNcm('')
      setCest('')
      setName('')
      setPriceCash(0)
      setMinStock(0)
      setVariationSource('')
      setVariationParentId('')
      setVariationOptionLabel('')
      setQuery(payload.name)
      setPhotoProductId(created.id)
      await load(payload.name)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }


  function prepareNewVariation(product: Product) {
    if (!canWrite) return
    const option = window.prompt('Qual é a cor ou tamanho? Ex.: Azul, Verde, 42')?.trim()
    if (!option) return
    if (option.length > 40) {
      setError('Use no máximo 40 caracteres para a cor ou tamanho.')
      return
    }
    // Reuse the existing per-product stock and sale/return paths, rather than
    // create a second ledger or silently split quantities from the parent.
    setSku('')
    setBarcode('')
    setNcm('')
    setCest('')
    setName(product.name + ' — ' + option)
    setUnit(product.unit)
    setPriceCash(product.price_cash)
    setMinStock(product.min_stock)
    setVariationSource(product.name + ' — ' + option)
    setVariationParentId(product.id)
    setVariationOptionLabel(option)
    setError('')
    if (manualFormRef.current) {
      manualFormRef.current.open = true
      manualFormRef.current.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
    window.requestAnimationFrame(() => skuInputRef.current?.focus())
  }

  async function chooseImportFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    setImportPreview(null)
    setImportKey('')
    setImportDigest('')
    setImportScope('')
    setImportResult('')
    if (!file) return
    if (file.size > 1024 * 1024) {
      setError('Arquivo muito grande. Limite: 1 MB e 500 produtos por lote.')
      return
    }
    try {
      const csv = await file.text()
      const preview = parseProductCSV(csv)
      if (preview.errors.length || !preview.valid.length) {
        setImportPreview(preview)
        setError('')
        return
      }
      const digest = await fingerprintProducts(preview.valid)
      const pending = readPendingProductImport()
      if (pending && pending.digest !== digest) {
        setPendingImport(pending)
        setError('Há uma importação pendente. Consulte o lote no servidor ou escolha o mesmo CSV para continuar com a chave original.')
        return
      }
      setImportPreview(preview)
      setImportKey(pending?.key ?? crypto.randomUUID())
      setImportDigest(digest)
      setImportScope(getSessionScope())
      setPendingImport(pending)
      setError('')
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function downloadCSVExample() {
    const blob = new Blob(['\uFEFF' + PRODUCT_IMPORT_EXAMPLE], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'modelo-produtos-sistemaemgo.csv'
    link.click()
    URL.revokeObjectURL(url)
  }

  async function importProducts() {
    if (!canWrite || importing || !importPreview?.valid.length ||
        importPreview.errors.length || !importKey || !importDigest ||
        !importScope || importScope !== getSessionScope()) return
    if (!window.confirm(`Cadastrar ${importPreview.valid.length} produto(s) nesta loja em um único lote? Se houver qualquer conflito, nenhum será cadastrado.`)) return

    const scope = getSessionScope()
    setImporting(true)
    setError('')
    setImportResult('')
    try {
      const prior = readPendingProductImport()
      const record: PendingProductImport = {
        key: importKey,
        digest: importDigest,
        createdAt: prior?.createdAt ?? Date.now(),
      }
      savePendingProductImport(record)
      setPendingImport(record)
    } catch (e: unknown) {
      setError('Nenhuma importação foi enviada: não foi possível preservar a referência de recuperação. ' + errorMessage(e))
      setImporting(false)
      return
    }
    try {
      const result = await apiJson<{ batch_id: string; item_count: number; replayed: boolean }>(
        '/api/v1/products/import-batches',
        {
          method: 'POST',
          headers: { 'Idempotency-Key': importKey },
          body: {
            items: importPreview.valid.map((row): ProductCreateRequest => ({
              sku: row.sku,
              name: row.name,
              unit: row.unit,
              price_cash: row.price_cash,
              min_stock: row.min_stock,
              barcode: row.barcode,
              ncm: row.ncm,
              cest: row.cest,
              cost_price: 0,
              active: true,
            })),
          },
        },
      )
      if (scope !== getSessionScope()) return
      clearPendingProductImport(importKey)
      setPendingImport(null)
      setHistoryRefresh((version) => version + 1)
      setImportResult(result.replayed
        ? `Lote ${result.batch_id} confirmado anteriormente. Nenhum produto foi duplicado.`
        : `Todos os ${result.item_count} produto(s) foram cadastrados. Lote ${result.batch_id}.`)
      setImportPreview(null)
      setImportKey('')
      setImportDigest('')
      setImportScope('')
      await load()
    } catch (e: unknown) {
      if (scope === getSessionScope()) {
        setError(`Importação não confirmada: ${errorMessage(e)}. A chave original foi preservada. Consulte o lote no servidor; se precisar repetir, use o mesmo CSV sem alterações. Não inicie outra importação antes de reconciliar esta tentativa.`)
      }
    } finally {
      setImporting(false)
    }
  }

  async function checkPendingProductImport() {
    if (!canWrite || importing) return
    const pending = readPendingProductImport()
    if (!pending) {
      setPendingImport(null)
      setError('Nenhuma referência de importação válida foi encontrada nesta conta e loja.')
      return
    }
    const scope = getSessionScope()
    setImporting(true)
    setError('')
    setImportResult('')
    try {
      const result = await apiJson<{ batch_id: string; item_count: number; replayed: boolean }>(
        `/api/v1/products/import-batches/${encodeURIComponent(pending.key)}`,
      )
      if (scope !== getSessionScope()) return
      clearPendingProductImport(pending.key)
      setPendingImport(null)
      setImportPreview(null)
      setImportKey('')
      setImportDigest('')
      setImportScope('')
      setHistoryRefresh((version) => version + 1)
      setImportResult(`O servidor confirmou o lote ${result.batch_id} com ${result.item_count} produto(s). Nenhuma nova gravação é necessária.`)
      await load()
    } catch (e: unknown) {
      if (scope !== getSessionScope()) return
      setError(e instanceof APIError && e.status === 404
        ? 'Este lote ainda não aparece como confirmado no servidor. Uma requisição anterior pode estar em andamento. Selecione o mesmo CSV para reutilizar a chave e tentar novamente.'
        : `Falha ao consultar o lote: ${errorMessage(e)}. Preserve a referência até esclarecer a situação.`)
    } finally {
      setImporting(false)
    }
  }

  function discardPendingProductImport() {
    if (!canWrite || importing || !pendingImport) return
    if (!window.confirm(
      'Descartar a referência salva neste navegador? Isso não desfaz cadastros eventualmente confirmados no servidor. Confira os produtos antes de iniciar outro lote.',
    )) return
    clearPendingProductImport(pendingImport.key)
    setPendingImport(null)
    setImportPreview(null)
    setImportKey('')
    setImportDigest('')
    setImportScope('')
    setImportResult('')
    setError('Referência local descartada. Verifique o catálogo antes de iniciar uma nova importação.')
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-slate-900">Produtos da loja</h2>
          <p className="mt-1 text-sm text-slate-600">{total} resultado(s). Busque, confira preços e veja o saldo disponível.</p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      <div className="mt-5 flex flex-col gap-3 sm:flex-row">
        <input
          aria-label="Buscar produto"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Digite o nome, código do produto ou código de barras"
          className="w-full rounded-md border px-3 py-2 text-sm"
        />

      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center gap-4 rounded-xl bg-slate-50 p-4 text-sm">
        <span className={lowStockItems.length ? 'font-medium text-amber-800' : 'text-gray-600'}>
          {lowStockItems.length} produto(s) com estoque no mínimo ou abaixo, entre os resultados carregados.
        </span>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={onlyLowStock} onChange={(e) => setOnlyLowStock(e.target.checked)} />
          Mostrar somente estoque baixo
        </label>
        <button type="button" onClick={() => setShowTechnical((value) => !value)} className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-xs font-medium hover:bg-slate-100">
          {showTechnical ? 'Ocultar códigos fiscais' : 'Editar códigos e dados fiscais'}
        </button>
      </div>

      <div className="mt-4 overflow-auto rounded-xl border border-slate-200">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-gray-50 text-xs text-gray-600">
            <tr>
              <th className="px-3 py-2">Foto</th>
              <th className="px-3 py-2">Código (SKU)</th>
              {showTechnical ? (<>
              <th className="px-3 py-2">Código de barras</th>
              <th className="px-3 py-2">Classificação (NCM)</th>
              <th className="px-3 py-2">Tributação (CEST)</th>
              </>) : null}
              <th className="px-3 py-2">Nome</th>
              <th className="px-3 py-2">Unidade</th>
              <th className="px-3 py-2">Preço</th>
              <th className="px-3 py-2">Em estoque</th>
              <th className="px-3 py-2">Mínimo</th>
              <th className="px-3 py-2">Ativo</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {visibleItems.map((p) => (
              <Fragment key={p.id}>
              <tr>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2">
                    {thumbnails[p.id] ? (
                      <img src={thumbnails[p.id]} alt={p.name} className="h-12 w-12 rounded-md object-cover" />
                    ) : <span className="flex h-12 w-12 items-center justify-center rounded-md bg-slate-100 text-xs text-slate-500">Sem foto</span>}
                    <button type="button" onClick={() => setPhotoProductId(photoProductId === p.id ? '' : p.id)} className="rounded-md border border-slate-300 px-2 py-1 text-xs hover:bg-slate-50">Fotos</button>
                  </div>
                </td>
                <td className="px-3 py-2 font-mono text-xs">{p.sku}</td>
                {showTechnical ? (<>
                <td className="px-3 py-2">
                  {canWrite ? (
                    <input
                      value={barcodeDrafts[p.id] ?? p.barcode ?? ''}
                      onChange={(e) =>
                        setBarcodeDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                      }
                      placeholder="Sem código"
                      autoComplete="off"
                      className="min-w-52 rounded-md border px-2 py-1 font-mono text-xs"
                    />
                  ) : (
                    <span className="font-mono text-xs">{p.barcode ?? '—'}</span>
                  )}
                </td>
                <td className="px-3 py-2">
                  {canWrite ? (
                    <input
                      value={ncmDrafts[p.id] ?? p.ncm ?? ''}
                      onChange={(e) =>
                        setNcmDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                      }
                      placeholder="8 dígitos"
                      inputMode="numeric"
                      maxLength={8}
                      className="w-28 rounded-md border px-2 py-1 font-mono text-xs"
                    />
                  ) : (
                    <span className="font-mono text-xs">{p.ncm ?? '—'}</span>
                  )}
                </td>
                <td className="px-3 py-2">
                  {canWrite ? (
                    <div className="flex items-center gap-2">
                      <input
                        value={cestDrafts[p.id] ?? p.cest ?? ''}
                        onChange={(e) =>
                          setCestDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                        }
                        placeholder="7 dígitos"
                        inputMode="numeric"
                        maxLength={7}
                        className="w-28 rounded-md border px-2 py-1 font-mono text-xs"
                      />
                      <button
                        type="button"
                        onClick={() => void saveCatalogFiscal(p)}
                        className="rounded-md border px-2 py-1 text-xs hover:bg-gray-50"
                      >
                        Salvar
                      </button>
                    </div>
                  ) : (
                    <span className="font-mono text-xs">{p.cest ?? '—'}</span>
                  )}
                </td>
                </>) : null}
                <td className="px-3 py-2">
                  <div>{p.name}</div>
                  <button type="button" onClick={() => setFamilyProductId(familyProductId === p.id ? "" : p.id)}
                    className="mt-1 mr-3 text-xs font-medium text-blue-700 underline hover:text-blue-900">
                    {familyProductId === p.id ? "Ocultar cores/tamanhos" : "Ver cores/tamanhos"}
                  </button>
                  {canWrite ? <button type="button" onClick={() => prepareNewVariation(p)}
                    className="mt-1 text-xs font-medium text-blue-700 underline hover:text-blue-900">
                    Nova cor/tamanho com estoque próprio
                  </button> : null}
                </td>
                <td className="px-3 py-2">{p.unit}</td>
                <td className="px-3 py-2">{p.price_cash.toFixed(2)}</td>
                <td className="px-3 py-2">
                  {p.qty_on_hand.toFixed(2)}
                  {p.active && p.min_stock > 0 && p.qty_on_hand <= p.min_stock ? (
                    <span className="ml-2 rounded bg-amber-100 px-2 py-1 text-xs text-amber-900">Estoque baixo</span>
                  ) : null}
                </td>
                <td className="px-3 py-2">{p.min_stock.toFixed(2)}</td>
                <td className="px-3 py-2">{p.active ? 'Sim' : 'Não'}</td>
              </tr>
              {familyProductId === p.id ? (
                <tr><td colSpan={showTechnical ? 11 : 8} className="p-3">
                  <ProductFamily productId={p.id} />
                </td></tr>
              ) : null}
              {photoProductId === p.id ? (
                <tr><td colSpan={showTechnical ? 11 : 8} className="p-3">
                  <ProductPhotos productId={p.id} productName={p.name} canWrite={canWrite}
                    onChange={() => setPhotoVersion((value) => value + 1)} />
                </td></tr>
              ) : null}
              </Fragment>
            ))}
            {visibleItems.length === 0 ? (
              <tr><td colSpan={showTechnical ? 11 : 8} className="px-3 py-6 text-center text-sm text-gray-500">
                {onlyLowStock ? 'Nenhum produto abaixo do mínimo entre os resultados carregados.' : 'Nenhum produto encontrado.'}
              </td></tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {canWrite ? (
        <details open={Boolean(pendingImport)} className="mt-6 rounded-2xl border border-slate-200 p-5">
          <summary className="cursor-pointer text-base font-bold">Importar uma planilha de produtos</summary>
          <div className="mt-4">
          <p className="mt-1 text-xs text-gray-600">
            Para cadastrar muitos produtos, baixe o modelo, preencha a planilha e escolha o arquivo CSV.
            Confira os dados antes de confirmar. O estoque não muda nesta etapa.
          </p>
          {pendingImport ? (
            <div role="region" aria-label="Importação de produtos pendente" className="mt-3 rounded-md border border-amber-300 p-3 text-sm">
              <p className="font-semibold">Uma tentativa anterior precisa ser conferida.</p>
              <p className="mt-1 text-xs">Referência: <code>{pendingImport.key}</code></p>
              <p className="mt-1 text-xs text-gray-600">
                A planilha não fica salva aqui. Consulte o servidor. Caso não tenha sido confirmada,
                escolha novamente o mesmo arquivo para reutilizar a chave original.
              </p>
              <div className="mt-2 flex flex-wrap gap-2">
                <button type="button" disabled={importing} onClick={() => void checkPendingProductImport()}
                  className="rounded-md border border-blue-300 px-3 py-2 text-sm disabled:opacity-50">
                  Conferir lote no servidor
                </button>
                <button type="button" disabled={importing} onClick={discardPendingProductImport}
                  className="rounded-md border px-3 py-2 text-sm text-amber-900 disabled:opacity-50">
                  Descartar referência local
                </button>
              </div>
            </div>
          ) : null}
          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button type="button" onClick={downloadCSVExample} className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50">
              Baixar modelo CSV
            </button>
            <label className="text-sm">
              <span className="mr-2">Escolher arquivo CSV</span>
              <input type="file" accept=".csv,text/csv" onChange={(e) => void chooseImportFile(e)} disabled={importing} className="text-xs" />
            </label>
          </div>
          {importPreview ? (
            <div className="mt-3 space-y-2 text-sm">
              <p>Arquivo: {importPreview.lines} linha(s) · {importPreview.valid.length} válida(s) · {importPreview.errors.length} problema(s).</p>
              {importPreview.errors.length > 0 ? (
                <div className="rounded-md border border-amber-200 p-2 text-xs text-amber-900">
                  <p className="font-semibold">Corrija estas linhas antes de importar:</p>
                  <ul className="mt-1 list-disc pl-5">{importPreview.errors.slice(0, 20).map((e, i) => <li key={i}>{e}</li>)}</ul>
                  {importPreview.errors.length > 20 ? <p>Mais {importPreview.errors.length - 20} erro(s).</p> : null}
                </div>
              ) : null}
              <p className="text-xs text-gray-600">Prévia: {importPreview.valid.slice(0, 5).map((row) => `${row.sku} — ${row.name} (R$ ${row.price_cash.toFixed(2)})`).join(' · ')}</p>
              <p className="text-xs text-amber-800">Códigos NCM/CEST precisam ser conferidos com o contador. A importação não habilita emissão fiscal.</p>
              <button type="button" onClick={() => void importProducts()} disabled={importing || importPreview.valid.length === 0 || importPreview.errors.length > 0 || !importKey || importScope !== getSessionScope()}
                className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50">
                {importing ? 'Importando produtos…' : `Confirmar importação de ${importPreview.valid.length} produto(s)`}
              </button>
            </div>
          ) : null}
          {importResult ? <p role="status" className="mt-3 text-sm">{importResult}</p> : null}
          <ImportBatchHistory kind="products" refreshVersion={historyRefresh} />
          </div>
        </details>
      ) : null}

      {canWrite ? (
        <details ref={manualFormRef} className="mt-5 rounded-2xl border border-slate-200 p-5">
          <summary className="cursor-pointer text-base font-bold">Adicionar produto manualmente</summary>
        <form onSubmit={onCreate} className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-8">
          <p className="text-sm text-slate-600 md:col-span-8">Preencha o nome, o código e o preço. Os dados fiscais podem ser preenchidos depois, com ajuda do contador.</p>
          {variationSource ? <p className="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-900 md:col-span-8">
            Nova opção de {variationSource}: ela ficará vinculada ao produto original, mas terá código e estoque independentes. Informe um SKU diferente. O saldo começa zerado; ajuste-o na aba Estoque após cadastrar. Os dados fiscais e código de barras não são copiados automaticamente.
          </p> : null}
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Código do produto (SKU)</span>
            <input
              ref={skuInputRef}
              value={sku}
              onChange={(e) => setSku(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Código de barras</span>
            <input
              value={barcode}
              onChange={(e) => setBarcode(e.target.value)}
              placeholder="EAN / GTIN"
              autoComplete="off"
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Nome do produto</span>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Unidade</span>
            <input
              value={unit}
              onChange={(e) => setUnit(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Preço (à vista)</span>
            <input
              value={String(priceCash)}
              onChange={(e) => setPriceCash(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Avisar quando restarem menos de</span>
            <input
              value={String(minStock)}
              onChange={(e) => setMinStock(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>

          <details className="rounded-xl border border-slate-200 bg-slate-50 p-4 md:col-span-8">
            <summary className="cursor-pointer font-medium text-slate-700">Dados fiscais (se precisar emitir nota)</summary>
            <p className="mt-2 text-xs text-slate-600">O contador pode informar estes códigos. Não invente um código fiscal.</p>
            <div className="mt-3 grid gap-4 sm:grid-cols-2">
          <label className="block">
            <span className="text-xs text-gray-600">Classificação fiscal (NCM)</span>
            <input
              value={ncm}
              onChange={(e) => setNcm(e.target.value)}
              placeholder="8 dígitos"
              inputMode="numeric"
              maxLength={8}
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Código tributário (CEST)</span>
            <input
              value={cest}
              onChange={(e) => setCest(e.target.value)}
              placeholder="7 dígitos"
              inputMode="numeric"
              maxLength={7}
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>

            </div>
          </details>
          <div className="md:col-span-8">
            <button
              disabled={!canCreate}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Cadastrar
            </button>
          </div>
        </form>
        </details>
      ) : null}
    </div>
  )
}
