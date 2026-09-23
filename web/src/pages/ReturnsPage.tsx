import { useEffect, useMemo, useState } from 'react'
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

type ReturnCreateResponse = {
  id: string
  refund_due: number
  refund_status: string
  replayed: boolean
}

export default function ReturnsPage() {
  const [saleId, setSaleId] = useState('')
  const [detail, setDetail] = useState<SaleDetail | null>(null)
  const [kind, setKind] = useState<'return' | 'exchange'>('return')
  const [reason, setReason] = useState('')
  const [qtyByItem, setQtyByItem] = useState<Record<string, number>>({})
  const [restockByItem, setRestockByItem] = useState<Record<string, boolean>>({})
  const [returns, setReturns] = useState<ReturnRecord[]>([])
  const [result, setResult] = useState<ReturnCreateResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

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
  }, [])

  async function loadSale() {
    const id = saleId.trim()
    if (!id) return
    setLoading(true)
    setError('')
    setResult(null)
    try {
      const data = await apiJson<SaleDetail>(`/api/v1/sales/${encodeURIComponent(id)}`)
      setDetail(data)
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
      const response = await apiJson<ReturnCreateResponse>(
        `/api/v1/sales/${detail.sale.id}/returns`,
        {
          method: 'POST',
          headers: { 'Idempotency-Key': crypto.randomUUID() },
          body: {
            kind,
            reason: reason.trim(),
            items: selected,
          },
        },
      )
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
      <h2 className="text-base font-semibold">Devoluções e trocas</h2>
      <p className="mt-1 text-sm text-gray-600">
        Registre itens devolvidos. O sistema calcula o valor devido, mas a liquidação do reembolso
        é feita separadamente no fluxo financeiro.
      </p>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 rounded-md border p-3">
        <label className="block">
          <span className="text-xs text-gray-600">ID da venda</span>
          <div className="mt-1 flex gap-2">
            <input
              value={saleId}
              onChange={(e) => setSaleId(e.target.value)}
              className="min-w-0 flex-1 rounded-md border px-3 py-2 font-mono text-sm"
              placeholder="Cole o ID da venda"
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
                    <th className="px-3 py-2">Devolver</th>
                    <th className="px-3 py-2">Volta ao estoque</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {detail.items.map((item) => (
                    <tr key={item.id}>
                      <td className="px-3 py-2 font-mono text-xs">{item.product_id}</td>
                      <td className="px-3 py-2">{item.qty.toFixed(3)}</td>
                      <td className="px-3 py-2">
                        <input
                          type="number"
                          min="0"
                          max={item.qty}
                          step="0.001"
                          value={qtyByItem[item.id] ?? 0}
                          onChange={(e) =>
                            setQtyByItem((prev) => ({
                              ...prev,
                              [item.id]: Number(e.target.value) || 0,
                            }))
                          }
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

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Últimas devoluções/trocas</h3>
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
                  <td className="px-2 py-2 font-mono text-xs">{item.sale_id}</td>
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
