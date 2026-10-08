import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { OPENING_STOCK_EXAMPLE, parseOpeningStockCSV, type OpeningStockPreview } from '../lib/openingStockImport'

type Product = {
  id: string
  sku: string
  name: string
  unit: string
  min_stock: number
  qty_on_hand: number
  active: boolean
  price_cash: number
}

type LowStockResponse = { items: Product[] | null; total: number }

type ProductsListResponse = { items: Product[]; total: number }

type AdjustRequest = {
  product_id: string
  delta: number
  reason: string
  type: 'adjustment' | 'loss' | 'damage'
}

export default function InventoryPage() {
  const [low, setLow] = useState<Product[]>([])
  const [openingPreview, setOpeningPreview] = useState<OpeningStockPreview | null>(null)
  const [openingKey, setOpeningKey] = useState('')
  const [openingConfirmed, setOpeningConfirmed] = useState(false)
  const [openingLoading, setOpeningLoading] = useState(false)
  const [openingMessage, setOpeningMessage] = useState('')
  const [lowTotal, setLowTotal] = useState(0)
  const [lowLimit, setLowLimit] = useState(50)
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [canAdjustPermission, setCanAdjustPermission] = useState(false)

  const [productId, setProductId] = useState('')
  const [delta, setDelta] = useState<number>(0)
  const [reason, setReason] = useState('')
  const [type, setType] = useState<AdjustRequest['type']>('adjustment')
  const canAdjust = useMemo(
    () => productId && delta !== 0 && reason.trim().length >= 3,
    [productId, delta, reason],
  )

  const load = useCallback(async () => {
    setError('')
    setLoading(true)
    try {
      const [lowRes, prodRes] = await Promise.all([
        apiJson<LowStockResponse>(`/api/v1/inventory/low-stock?limit=${lowLimit}`),
        apiJson<ProductsListResponse>('/api/v1/products?limit=200&offset=0'),
      ])
      setLow(lowRes.items ?? [])
      setLowTotal(lowRes.total)
      setProducts(prodRes.items ?? [])
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }, [lowLimit])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((me) => setCanAdjustPermission(me.permissions.includes('inventory:adjust')))
      .catch((e: unknown) => setError(errorMessage(e)))
  }, [])

  async function onAdjust(e: FormEvent) {
    e.preventDefault()
    if (!canAdjust) return
    setError('')
    try {
      const payload: AdjustRequest = {
        product_id: productId,
        delta: Number(delta),
        reason: reason.trim(),
        type,
      }
      await apiJson('/api/v1/inventory/adjust', { method: 'POST', body: payload })
      setDelta(0)
      setReason('')
      await load()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function downloadOpeningExample() {
    const blob = new Blob(['\uFEFF' + OPENING_STOCK_EXAMPLE], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'modelo-estoque-inicial.csv'
    link.click()
    URL.revokeObjectURL(url)
  }

  async function chooseOpeningFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    setOpeningPreview(null)
    setOpeningKey('')
    setOpeningConfirmed(false)
    setOpeningMessage('')
    if (!file) return
    if (file.size > 1024 * 1024) {
      setError('O CSV deve ter até 1 MB e no máximo 100 produtos.')
      return
    }
    try {
      const preview = parseOpeningStockCSV(await file.text())
      setOpeningPreview(preview)
      setOpeningKey(crypto.randomUUID())
      setError('')
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function submitOpeningStock() {
    if (!canAdjustPermission || openingLoading || !openingPreview ||
        !openingConfirmed || openingPreview.errors.length || !openingPreview.rows.length || !openingKey) return
    if (!window.confirm(
      `Confirmar saldo inicial de ${openingPreview.rows.length} produto(s)? O lote só funciona em produtos que nunca tiveram movimentação. A operação é atômica e auditada.`,
    )) return
    setOpeningLoading(true)
    setOpeningMessage('')
    setError('')
    try {
      const result = await apiJson<{ batch_id: string; item_count: number; replayed: boolean }>(
        '/api/v1/inventory/opening-stock',
        {
          method: 'POST',
          headers: { 'Idempotency-Key': openingKey },
          body: { items: openingPreview.rows.map(({ sku, quantity }) => ({ sku, quantity })) },
        },
      )
      setOpeningMessage(
        result.replayed
          ? `Lote ${result.batch_id} já havia sido aplicado. Nenhum estoque foi lançado novamente.`
          : `Saldo inicial de ${result.item_count} produto(s) salvo com segurança. Lote ${result.batch_id}.`,
      )
      setOpeningPreview(null)
      setOpeningKey('')
      setOpeningConfirmed(false)
      await load()
    } catch (e: unknown) {
      setError(`Não foi possível confirmar o lote: ${errorMessage(e)}. Nenhuma correção manual deve ser feita antes de conferir o saldo. Se a conexão falhou, tente novamente sem escolher outro arquivo: a chave original será reutilizada.`)
    } finally {
      setOpeningLoading(false)
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Estoque</h2>
          <p className="text-sm text-gray-600">Ajuste manual e alerta de baixo estoque.</p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4">
        <h3 className="text-sm font-semibold">Baixo estoque</h3>
        <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-gray-600">
          <span><strong>{lowTotal}</strong> produto(s) precisam de atenção. Mostrando {low.length}.</span>
          {low.length < lowTotal && lowLimit < 500 ? (
            <button type="button" className="rounded-md border px-3 py-2 hover:bg-gray-50"
              onClick={() => setLowLimit(500)}>Mostrar até 500 produtos</button>
          ) : null}
          {low.length < lowTotal && lowLimit >= 500 ? (
            <span className="text-amber-800">Há mais de 500 itens; procure os demais no cadastro de produtos.</span>
          ) : null}
        </div>
        <div className="mt-2 overflow-auto rounded-md border">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr>
                <th className="px-3 py-2">SKU</th>
                <th className="px-3 py-2">Produto</th>
                <th className="px-3 py-2">Qtd</th>
                <th className="px-3 py-2">Min</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {low.map((p) => (
                <tr key={p.id}>
                  <td className="px-3 py-2 font-mono text-xs">{p.sku}</td>
                  <td className="px-3 py-2">{p.name}</td>
                  <td className="px-3 py-2">{p.qty_on_hand.toFixed(2)}</td>
                  <td className="px-3 py-2">{p.min_stock.toFixed(2)}</td>
                </tr>
              ))}
              {low.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-sm text-gray-500" colSpan={4}>
                    Nenhum item com baixo estoque.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>

      {canAdjustPermission ? (
        <section className="mt-6 rounded-lg border p-4">
          <h3 className="text-base font-semibold">Cadastrar estoque inicial por planilha</h3>
          <p className="mt-1 text-sm text-gray-600">
            Ideal para configurar uma loja nova: informe somente o SKU do produto e a quantidade
            contada na prateleira. A planilha não cria produtos nem altera preços.
          </p>
          <p className="mt-2 rounded-md bg-amber-50 p-2 text-xs text-amber-900">
            Esta opção aceita somente produtos sem saldo anterior e sem qualquer movimentação.
            Para corrigir estoque existente, use o ajuste auditado abaixo. Não repita a
            importação mudando o arquivo após erro de conexão.
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-3">
            <button type="button" onClick={downloadOpeningExample}
              className="rounded-md border px-3 py-2 text-sm">Baixar modelo CSV</button>
            <label className="text-sm">
              <span className="mr-2">Escolher planilha CSV</span>
              <input type="file" accept=".csv,text/csv" disabled={openingLoading}
                onChange={(e) => void chooseOpeningFile(e)} className="text-xs" />
            </label>
          </div>
          {openingPreview ? (
            <div className="mt-3 space-y-3">
              <p className="text-sm">Conferência: {openingPreview.total} linha(s),
                {' '}{openingPreview.rows.length} válida(s), {openingPreview.errors.length} com erro.</p>
              {openingPreview.errors.length > 0 ? (
                <div className="rounded-md border border-red-200 p-3 text-xs text-red-700">
                  <strong>Corrija o arquivo antes de continuar:</strong>
                  <ul className="list-disc pl-5">{openingPreview.errors.slice(0, 20).map((entry, index) =>
                    <li key={index}>{entry}</li>)}</ul>
                </div>
              ) : null}
              <div className="max-h-44 overflow-auto rounded-md border">
                <table className="min-w-full text-left text-xs">
                  <thead className="bg-gray-50"><tr><th className="px-3 py-2">SKU</th><th className="px-3 py-2">Saldo contado</th></tr></thead>
                  <tbody>{openingPreview.rows.map((item) =>
                    <tr key={item.sku}>
                      <td className="px-3 py-2">{item.sku}
                        <span className="block text-gray-500">
                          {products.find((product) => product.sku === item.sku)?.name ?? 'Verifique o SKU na lista de produtos'}
                        </span>
                      </td>
                      <td className="px-3 py-2">{item.quantity.toFixed(3)}</td>
                    </tr>)}</tbody>
                </table>
              </div>
              <label className="flex items-start gap-2 text-sm">
                <input type="checkbox" checked={openingConfirmed} disabled={openingLoading}
                  onChange={(e) => setOpeningConfirmed(e.target.checked)} />
                <span>Conferi os códigos e as quantidades com a contagem física da loja atual.</span>
              </label>
              <button type="button" onClick={() => void submitOpeningStock()}
                disabled={openingLoading || !openingConfirmed || openingPreview.errors.length > 0 || !openingPreview.rows.length}
                className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50">
                {openingLoading ? 'Salvando lote…' : `Confirmar estoque inicial de ${openingPreview.rows.length} produto(s)`}
              </button>
            </div>
          ) : null}
          {openingMessage ? <p role="status" className="mt-3 text-sm text-green-800">{openingMessage}</p> : null}
        </section>
      ) : null}

      {canAdjustPermission ? (
        <div className="mt-6">
          <h3 className="text-sm font-semibold">Ajuste de estoque</h3>
        <form onSubmit={onAdjust} className="mt-2 grid grid-cols-1 gap-3 md:grid-cols-4">
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Produto</span>
            <select
              value={productId}
              onChange={(e) => setProductId(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            >
              <option value="">Selecione…</option>
              {products.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.sku} — {p.name}
                </option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="text-xs text-gray-600">Tipo</span>
            <select
              value={type}
              onChange={(e) => setType(e.target.value as AdjustRequest['type'])}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            >
              <option value="adjustment">Ajuste</option>
              <option value="loss">Perda</option>
              <option value="damage">Avaria</option>
            </select>
          </label>

          <label className="block">
            <span className="text-xs text-gray-600">Delta</span>
            <input
              value={String(delta)}
              onChange={(e) => setDelta(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>

          <label className="block md:col-span-4">
            <span className="text-xs text-gray-600">Motivo</span>
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              placeholder="Ex.: correção de inventário, perda identificada…"
              required
            />
          </label>

          <div className="md:col-span-4">
            <button
              disabled={!canAdjust}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Aplicar ajuste
            </button>
          </div>
        </form>
        </div>
      ) : null}
    </div>
  )
}
