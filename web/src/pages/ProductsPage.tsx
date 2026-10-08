import { useEffect, useMemo, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
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
  const [importFailures, setImportFailures] = useState<string[]>([])

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

  const canCreate = useMemo(() => sku.trim() && name.trim() && priceCash > 0, [
    sku,
    name,
    priceCash,
  ])

  async function load() {
    setError('')
    setLoading(true)
    try {
      const qs = new URLSearchParams()
      if (query.trim()) qs.set('query', query.trim())
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
    void load()
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((me) => setCanWrite(me.permissions.includes('product:write')))
      .catch((e: unknown) => setError(errorMessage(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

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
      await apiJson<{ id: string }>('/api/v1/products', {
        method: 'POST',
        body: payload,
      })
      setSku('')
      setBarcode('')
      setNcm('')
      setCest('')
      setName('')
      setPriceCash(0)
      setMinStock(0)
      await load()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function chooseImportFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    setImportPreview(null)
    setImportResult('')
    setImportFailures([])
    if (!file) return
    if (file.size > 1024 * 1024) {
      setError('Arquivo muito grande. Limite: 1 MB e 500 produtos por lote.')
      return
    }
    try {
      const csv = await file.text()
      setImportPreview(parseProductCSV(csv))
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
    if (!canWrite || importing || !importPreview?.valid.length) return
    if (!window.confirm(`Cadastrar ${importPreview.valid.length} produto(s) na loja atual? Os produtos já cadastrados podem gerar conflito, e o processo não é revertido automaticamente.`)) return
    setImporting(true)
    setError('')
    setImportFailures([])
    let created = 0
    const failures: string[] = []
    for (const row of importPreview.valid) {
      const payload: ProductCreateRequest = {
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
      }
      try {
        await apiJson<{ id: string }>('/api/v1/products', { method: 'POST', body: payload })
        created += 1
      } catch (e: unknown) {
        failures.push(`Linha ${row.line} (SKU ${row.sku}): ${errorMessage(e)}`)
        // Never keep sending after access is lost; avoid misleading partial import.
        if (e instanceof Error && /não autenticad|unauthoriz|sessão expirada/i.test(e.message)) break
      }
    }
    setImportResult(`${created} produto(s) cadastrado(s); ${failures.length} falha(s). Confira os detalhes antes de repetir a importação.`)
    setImportFailures(failures)
    setImportPreview(null)
    setImporting(false)
    await load()
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Produtos</h2>
          <p className="text-sm text-gray-600">Total: {total}</p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      <div className="mt-4 flex gap-2">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Buscar por nome, SKU ou código"
          className="w-full rounded-md border px-3 py-2 text-sm"
        />
        <button
          onClick={() => void load()}
          className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white"
        >
          Buscar
        </button>
      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-3 flex flex-wrap items-center gap-3 text-sm">
        <span className={lowStockItems.length ? 'font-medium text-amber-800' : 'text-gray-600'}>
          {lowStockItems.length} produto(s) com estoque no mínimo ou abaixo, entre os resultados carregados.
        </span>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={onlyLowStock} onChange={(e) => setOnlyLowStock(e.target.checked)} />
          Mostrar somente estoque baixo
        </label>
      </div>

      <div className="mt-4 overflow-auto rounded-md border">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-gray-50 text-xs text-gray-600">
            <tr>
              <th className="px-3 py-2">SKU</th>
              <th className="px-3 py-2">Código de barras</th>
              <th className="px-3 py-2">NCM</th>
              <th className="px-3 py-2">CEST</th>
              <th className="px-3 py-2">Nome</th>
              <th className="px-3 py-2">Un</th>
              <th className="px-3 py-2">Preço</th>
              <th className="px-3 py-2">Qtd</th>
              <th className="px-3 py-2">Min</th>
              <th className="px-3 py-2">Ativo</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {visibleItems.map((p) => (
              <tr key={p.id}>
                <td className="px-3 py-2 font-mono text-xs">{p.sku}</td>
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
                <td className="px-3 py-2">{p.name}</td>
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
            ))}
            {visibleItems.length === 0 ? (
              <tr><td colSpan={10} className="px-3 py-6 text-center text-sm text-gray-500">
                {onlyLowStock ? 'Nenhum produto abaixo do mínimo entre os resultados carregados.' : 'Nenhum produto encontrado.'}
              </td></tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {canWrite ? (
        <section className="mt-6 rounded-md border p-4">
          <h3 className="text-sm font-semibold">Importar produtos de uma planilha</h3>
          <p className="mt-1 text-xs text-gray-600">
            Baixe o modelo, preencha no Excel ou LibreOffice e salve como CSV. Os produtos serão
            cadastrados somente na loja em que você está conectado. Não altera o estoque atual.
          </p>
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
              <button type="button" onClick={() => void importProducts()} disabled={importing || importPreview.valid.length === 0 || importPreview.errors.length > 0}
                className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50">
                {importing ? 'Importando produtos…' : `Confirmar importação de ${importPreview.valid.length} produto(s)`}
              </button>
            </div>
          ) : null}
          {importResult ? <p role="status" className="mt-3 text-sm">{importResult}</p> : null}
          {importFailures.length ? (
            <ul className="mt-2 list-disc pl-5 text-xs text-red-700">{importFailures.slice(0, 30).map((err, i) => <li key={i}>{err}</li>)}</ul>
          ) : null}
        </section>
      ) : null}

      {canWrite ? (
        <div className="mt-6">
          <h3 className="text-sm font-semibold">Cadastrar produto</h3>
        <form onSubmit={onCreate} className="mt-2 grid grid-cols-1 gap-3 md:grid-cols-8">
          <label className="block md:col-span-1">
            <span className="text-xs text-gray-600">SKU</span>
            <input
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
          <label className="block">
            <span className="text-xs text-gray-600">NCM</span>
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
            <span className="text-xs text-gray-600">CEST</span>
            <input
              value={cest}
              onChange={(e) => setCest(e.target.value)}
              placeholder="7 dígitos"
              inputMode="numeric"
              maxLength={7}
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Nome</span>
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
            <span className="text-xs text-gray-600">Estoque mín.</span>
            <input
              value={String(minStock)}
              onChange={(e) => setMinStock(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>

          <div className="md:col-span-8">
            <button
              disabled={!canCreate}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Cadastrar
            </button>
          </div>
        </form>
        </div>
      ) : null}
    </div>
  )
}
