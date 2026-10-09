import { useEffect, useMemo, useRef, useState } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Sale = {
  id: string
  status: string
  total: number
}

type SaleItem = {
  id: string
  product_id: string
  qty: number
  unit_price: number
  discount_value: number
  subtotal: number
}

type SaleDetail = {
  sale: Sale
  items: SaleItem[]
}

type ReturnRecord = {
  id: string
  sale_id: string
  kind: 'return' | 'exchange'
  reason: string
  refund_due: number
  created_at: string
}

type ReturnsList = {
  items: ReturnRecord[]
  total: number
}

type RecentSales = { items: Array<{ id: string; total: number; status: string }> }

type ReturnCreateResponse = {
  id: string
  refund_due: number
  refund_status: string
  replayed: boolean
}

export default function ReturnsPage() {
  const [saleId, setSaleId] = useState('')
  const [recentSales, setRecentSales] = useState<RecentSales['items']>([])
  const [detail, setDetail] = useState<SaleDetail | null>(null)
  const [kind, setKind] = useState<'return' | 'exchange'>('return')
  const [reason, setReason] = useState('')
  const [qtyByItem, setQtyByItem] = useState<Record<string, number>>({})
  const [restockByItem, setRestockByItem] = useState<Record<string, boolean>>({})
  const [returnedByItem, setReturnedByItem] = useState<Record<string, number>>({})
  const [returns, setReturns] = useState<ReturnRecord[]>([])
  const [result, setResult] = useState<ReturnCreateResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const returnKeyRef = useRef('')

  async function loadReturns() {
    try {
      const data = await apiJson<ReturnsList>('/api/v1/returns?limit=50&offset=0')
      setReturns(data.items)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  useEffect(() => {
    void loadReturns()
    void apiJson<RecentSales>('/api/v1/sales?limit=30&offset=0')
      .then((data) => setRecentSales(data.items ?? []))
      .catch(() => { /* The manual sale lookup remains available. */ })
  }, [])

  async function loadSale() {
    const id = saleId.trim()
    if (!id) return
    setLoading(true)
    setError('')
    try {
      const data = await apiJson<SaleDetail>(`/api/v1/sales/${encodeURIComponent(id)}`)
      const history = await apiJson<ReturnsList>(
        `/api/v1/returns?sale_id=${encodeURIComponent(id)}&limit=200&offset=0`,
      )
      const returned: Record<string, number> = {}
      for (const record of history.items) {
        const detail = await apiJson<{
          items: Array<{ sale_item_id: string; qty: number }>
        }>(`/api/v1/returns/${record.id}`)
        for (const item of detail.items) {
          returned[item.sale_item_id] = (returned[item.sale_item_id] ?? 0) + item.qty
        }
      }

      returnKeyRef.current = ''
      setDetail(data)
      setReturnedByItem(returned)
      const qty: Record<string, number> = {}
      const restock: Record<string, boolean> = {}
      for (const item of data.items) {
        qty[item.id] = 0
        restock[item.id] = true
      }
      setQtyByItem(qty)
      setRestockByItem(restock)
    } catch (e: unknown) {
      setDetail(null)
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  const selected = useMemo(
    () =>
      detail?.items
        .map((item) => ({
          sale_item_id: item.id,
          qty: Number(qtyByItem[item.id]) || 0,
          restock: restockByItem[item.id] ?? true,
        }))
        .filter((item) => item.qty > 0) ?? [],
    [detail, qtyByItem, restockByItem],
  )

  async function submitReturn() {
    if (!detail || selected.length === 0 || reason.trim().length < 3 || saving) return
    setSaving(true)
    setError('')
    setResult(null)
    try {
      if (!returnKeyRef.current) returnKeyRef.current = crypto.randomUUID()
      const response = await apiJson<ReturnCreateResponse>(
        `/api/v1/sales/${detail.sale.id}/returns`,
        {
          method: 'POST',
          headers: { 'Idempotency-Key': returnKeyRef.current },
          body: {
            kind,
            reason: reason.trim(),
            items: selected,
          },
        },
      )
      returnKeyRef.current = ''
      setResult(response)
      setReason('')
      setQtyByItem((prev) =>
        Object.fromEntries(Object.keys(prev).map((key) => [key, 0])),
      )
      await loadReturns()
      await loadSale()
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      <h2 className="text-2xl font-bold tracking-tight">Devoluções e trocas</h2>
      <p className="mt-1 text-sm text-gray-600">
        Procure a venda, escolha os itens que voltaram e registre o motivo.
        Se houver reembolso, ele será conferido separadamente no Financeiro.
      </p>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-5 rounded-2xl border border-slate-200 p-5">
        <label className="block text-sm font-medium">
          <span>Selecione uma venda recente</span>
          <select
            aria-label="Vendas recentes"
            className="mt-2 w-full rounded-lg border px-3 py-3 text-sm"
            value={recentSales.some((sale) => sale.id === saleId) ? saleId : ''}
            onChange={(e) => {setSaleId(e.target.value);setDetail(null);setResult(null)}}
          >
            <option value="">Escolha uma venda ou digite o número abaixo</option>
            {recentSales.map((sale) => (
              <option key={sale.id} value={sale.id}>
                Venda {sale.id.slice(0, 8)} — R$ {sale.total.toFixed(2)} — {sale.status === 'finalized' ? 'Concluída' : sale.status}
              </option>
            ))}
          </select>
        </label>
        <label className="mt-4 block">
          <span className="text-xs text-gray-600">Ou procure pelo código completo da venda</span>
          <div className="mt-1 flex gap-2">
            <input
              value={saleId}
              onChange={(e) => setSaleId(e.target.value)}
              className="min-w-0 flex-1 rounded-md border px-3 py-2 font-mono text-sm"
              placeholder="Cole o código do comprovante, se não estiver na lista"
            />
            <button
              type="button"
              onClick={() => void loadSale()}
              disabled={loading}
              className="rounded-md border px-3 py-2 text-sm"
            >
              {loading ? 'Carregando…' : 'Buscar'}
            </button>
          </div>
        </label>

        {detail ? (
          <div className="mt-4">
            <div className="text-sm">
              Venda <span className="font-mono text-xs">{detail.sale.id}</span> • status{' '}
              <strong>{detail.sale.status}</strong> • total R$ {detail.sale.total.toFixed(2)}
            </div>

            <div className="mt-3 overflow-auto rounded-md border">
              <table className="min-w-full text-left text-sm">
                <thead className="bg-gray-50 text-xs text-gray-600">
                  <tr>
                    <th className="px-3 py-2">Produto</th>
                    <th className="px-3 py-2">Vendido</th>
                    <th className="px-3 py-2">Restante</th>
                    <th className="px-3 py-2">Devolver</th>
                    <th className="px-3 py-2">Volta ao estoque</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {detail.items.map((item) => (
                    <tr key={item.id}>
                      <td className="px-3 py-2 text-xs" title={item.product_id}>Produto {item.product_id.slice(0, 8)}…</td>
                      <td className="px-3 py-2">{item.qty.toFixed(3)}</td>
                      <td className="px-3 py-2">
                        {Math.max(0, item.qty - (returnedByItem[item.id] ?? 0)).toFixed(3)}
                      </td>
                      <td className="px-3 py-2">
                        <input
                          type="number"
                          min="0"
                          max={Math.max(0, item.qty - (returnedByItem[item.id] ?? 0))}
                          step="0.001"
                          value={qtyByItem[item.id] ?? 0}
                          onChange={(e) => {
                            const remaining = Math.max(
                              0,
                              item.qty - (returnedByItem[item.id] ?? 0),
                            )
                            const raw = Number(e.target.value)
                            const normalized = Number.isFinite(raw)
                              ? Math.min(remaining, Math.max(0, Math.round(raw * 1000) / 1000))
                              : 0
                            setQtyByItem((prev) => ({
                              ...prev,
                              [item.id]: normalized,
                            }))
                          }}
                          className="w-28 rounded-md border px-2 py-1"
                        />
                      </td>
                      <td className="px-3 py-2">
                        <label className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            checked={restockByItem[item.id] ?? true}
                            onChange={(e) =>
                              setRestockByItem((prev) => ({
                                ...prev,
                                [item.id]: e.target.checked,
                              }))
                            }
                          />
                          <span className="text-xs">Item vendável</span>
                        </label>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="mt-3 grid gap-3 md:grid-cols-2">
              <label className="block">
                <span className="text-xs text-gray-600">Operação</span>
                <select
                  value={kind}
                  onChange={(e) => setKind(e.target.value as 'return' | 'exchange')}
                  className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                >
                  <option value="return">Devolução</option>
                  <option value="exchange">Troca</option>
                </select>
              </label>
              <label className="block">
                <span className="text-xs text-gray-600">Motivo</span>
                <input
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  maxLength={250}
                  className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                  placeholder="Ex.: tamanho incorreto"
                />
              </label>
            </div>

            {kind === 'exchange' ? (
              <p className="mt-2 text-xs text-gray-600">
                Na troca, registre esta devolução e depois faça a nova venda normalmente no PDV.
              </p>
            ) : null}

            <button
              type="button"
              onClick={() => void submitReturn()}
              disabled={saving || selected.length === 0 || reason.trim().length < 3}
              className="mt-3 rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              {saving ? 'Registrando…' : kind === 'exchange' ? 'Registrar troca' : 'Registrar devolução'}
            </button>
          </div>
        ) : null}

        {result ? (
          <div className="mt-3 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
            Registro <span className="font-mono text-xs">{result.id}</span> criado. Reembolso devido:
            {' '}R$ {result.refund_due.toFixed(2)} • status financeiro: pendente.
          </div>
        ) : null}
      </div>

      <div className="mt-6 rounded-2xl border border-slate-200 p-5">
        <h3 className="text-base font-semibold">Últimas devoluções e trocas</h3>
        <div className="mt-2 overflow-auto">
          <table className="min-w-full text-left text-sm">
            <thead className="text-xs text-gray-600">
              <tr>
                <th className="px-2 py-2">Venda</th>
                <th className="px-2 py-2">Tipo</th>
                <th className="px-2 py-2">Motivo</th>
                <th className="px-2 py-2">Reembolso devido</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {returns.map((item) => (
                <tr key={item.id}>
                  <td className="px-2 py-2 font-mono text-xs" title={item.sale_id}>{item.sale_id.slice(0, 8)}…</td>
                  <td className="px-2 py-2">{item.kind === 'exchange' ? 'Troca' : 'Devolução'}</td>
                  <td className="px-2 py-2">{item.reason}</td>
                  <td className="px-2 py-2">R$ {item.refund_due.toFixed(2)}</td>
                </tr>
              ))}
              {returns.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-2 py-5 text-center text-gray-500">
                    Nenhum registro.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
