import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { APIError, apiJson, errorMessage } from '../lib/api'

type Sale = {
  id: string
  cash_session_id: string
  status: string
  subtotal: number
  discount_value: number
  total: number
}

type SaleItem = {
  id: string
  sale_id: string
  product_id: string
  qty: number
  unit_price: number
  discount_value: number
  subtotal: number
}

type Product = {
  id: string
  sku: string
  name: string
}

type SaleDetail = {
  sale: Sale
  items: SaleItem[]
  payments: Array<{ id: string; method: string; amount: number }>
}

type SaleReturn = {
  id: string
  sale_id: string
  reason: string
  total_amount: number
  recovered_cost: number
  refunded_amount: number
  created_at: string
}

type CreateReturnResponse = {
  return: SaleReturn
  replayed: boolean
}

type Refund = {
  id: string
  return_id: string
  sale_id: string
  method: string
  amount: number
}

const refundLabels: Record<string, string> = {
  cash: 'Dinheiro',
  pix: 'Pix',
  debit: 'Débito',
  credit: 'Crédito',
  transfer: 'Transferência',
  voucher: 'Voucher',
  store_credit: 'Crédito da loja',
}

export default function ReturnsPage() {
  const [sales, setSales] = useState<Sale[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [selected, setSelected] = useState<SaleDetail | null>(null)
  const [returns, setReturns] = useState<SaleReturn[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const [reason, setReason] = useState('')
  const [returnQty, setReturnQty] = useState<Record<string, number>>({})
  const [restock, setRestock] = useState<Record<string, boolean>>({})
  const returnKeyRef = useRef('')

  const [refundTarget, setRefundTarget] = useState<SaleReturn | null>(null)
  const [refundMethod, setRefundMethod] = useState('cash')
  const [refundAmount, setRefundAmount] = useState(0)
  const [refundReference, setRefundReference] = useState('')
  const refundKeyRef = useRef('')

  const productById = useMemo(() => {
    const map = new Map<string, Product>()
    for (const product of products) map.set(product.id, product)
    return map
  }, [products])

  async function loadBase() {
    setLoading(true)
    setError('')
    try {
      const [saleData, productData] = await Promise.all([
        apiJson<{ items: Sale[]; total: number }>('/api/v1/sales?limit=100&offset=0'),
        apiJson<{ items: Product[]; total: number }>('/api/v1/products?limit=200&offset=0'),
      ])
      setSales(saleData.items)
      setProducts(productData.items)
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadBase()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function selectSale(saleID: string) {
    setError('')
    try {
      const [detail, returnData] = await Promise.all([
        apiJson<SaleDetail>(`/api/v1/sales/${saleID}`),
        apiJson<{ items: SaleReturn[]; total: number }>(
          `/api/v1/sales/${saleID}/returns?limit=100&offset=0`,
        ),
      ])
      setSelected(detail)
      setReturns(returnData.items)
      const qty: Record<string, number> = {}
      const restockState: Record<string, boolean> = {}
      for (const item of detail.items) {
        qty[item.id] = 0
        restockState[item.id] = true
      }
      setReturnQty(qty)
      setRestock(restockState)
      setReason('')
      returnKeyRef.current = ''
      setRefundTarget(null)
      refundKeyRef.current = ''
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function refreshSelected() {
    if (!selected) return
    await selectSale(selected.sale.id)
  }

  async function createReturn(e: FormEvent) {
    e.preventDefault()
    if (!selected) return
    const items = selected.items
      .map((item) => ({
        sale_item_id: item.id,
        qty: Number(returnQty[item.id]) || 0,
        restock: restock[item.id] !== false,
      }))
      .filter((item) => item.qty > 0)
    if (items.length === 0 || reason.trim().length < 3) return

    setError('')
    try {
      if (!returnKeyRef.current) returnKeyRef.current = crypto.randomUUID()
      await apiJson<CreateReturnResponse>(`/api/v1/sales/${selected.sale.id}/returns`, {
        method: 'POST',
        headers: { 'Idempotency-Key': returnKeyRef.current },
        body: { reason: reason.trim(), items },
      })
      returnKeyRef.current = ''
      await loadBase()
      await refreshSelected()
    } catch (e: unknown) {
      if (e instanceof APIError && e.status === 409) returnKeyRef.current = ''
      setError(errorMessage(e))
    }
  }

  function startRefund(value: SaleReturn) {
    const remaining = Math.max(0, value.total_amount - value.refunded_amount)
    setRefundTarget(value)
    setRefundAmount(remaining)
    setRefundMethod('cash')
    setRefundReference('')
    refundKeyRef.current = ''
  }

  async function createRefund(e: FormEvent) {
    e.preventDefault()
    if (!selected || !refundTarget || refundAmount <= 0) return

    setError('')
    try {
      if (!refundKeyRef.current) refundKeyRef.current = crypto.randomUUID()
      await apiJson<{ refund: Refund; replayed: boolean }>(
        `/api/v1/sales/${selected.sale.id}/returns/${refundTarget.id}/refunds`,
        {
          method: 'POST',
          headers: { 'Idempotency-Key': refundKeyRef.current },
          body: {
            method: refundMethod,
            amount: refundAmount,
            external_reference: refundReference.trim() || null,
            notes: null,
          },
        },
      )
      refundKeyRef.current = ''
      setRefundTarget(null)
      await refreshSelected()
    } catch (e: unknown) {
      if (e instanceof APIError && e.status === 409) refundKeyRef.current = ''
      setError(errorMessage(e))
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Vendas e devoluções</h2>
          <p className="text-sm text-gray-600">
            Devolução física e reembolso são registrados separadamente.
          </p>
          <p className="mt-1 text-xs text-gray-500">
            Para troca: registre a devolução; use crédito da loja se aplicável; depois faça a nova
            venda no PDV.
          </p>
        </div>
        <button
          type="button"
          onClick={() => void loadBase()}
          disabled={loading}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50 disabled:opacity-60"
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 overflow-auto rounded-md border">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-gray-50 text-xs text-gray-600">
            <tr>
              <th className="px-3 py-2">Venda</th>
              <th className="px-3 py-2">Status</th>
              <th className="px-3 py-2">Total</th>
              <th className="px-3 py-2">Ação</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {sales.map((sale) => (
              <tr key={sale.id}>
                <td className="px-3 py-2 font-mono text-xs">{sale.id}</td>
                <td className="px-3 py-2">{sale.status}</td>
                <td className="px-3 py-2">R$ {sale.total.toFixed(2)}</td>
                <td className="px-3 py-2">
                  <button
                    type="button"
                    onClick={() => void selectSale(sale.id)}
                    className="text-xs text-blue-700 hover:underline"
                  >
                    Abrir devolução
                  </button>
                </td>
              </tr>
            ))}
            {sales.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-gray-500">
                  Nenhuma venda encontrada.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      {selected ? (
        <section className="mt-5 rounded-md border p-3">
          <h3 className="text-sm font-semibold">Venda {selected.sale.id}</h3>
          <p className="mt-1 text-xs text-gray-600">
            Status: {selected.sale.status} • Total: R$ {selected.sale.total.toFixed(2)}
          </p>

          {selected.sale.status === 'finalized' ? (
            <form onSubmit={createReturn} className="mt-3">
              <div className="overflow-auto rounded-md border">
                <table className="min-w-full text-left text-sm">
                  <thead className="bg-gray-50 text-xs text-gray-600">
                    <tr>
                      <th className="px-3 py-2">Produto</th>
                      <th className="px-3 py-2">Vendido</th>
                      <th className="px-3 py-2">Devolver</th>
                      <th className="px-3 py-2">Volta ao estoque?</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y">
                    {selected.items.map((item) => {
                      const product = productById.get(item.product_id)
                      return (
                        <tr key={item.id}>
                          <td className="px-3 py-2">
                            {product ? `${product.sku} — ${product.name}` : item.product_id}
                          </td>
                          <td className="px-3 py-2">{item.qty.toFixed(3)}</td>
                          <td className="px-3 py-2">
                            <input
                              aria-label={`Quantidade para devolver ${product?.name ?? item.product_id}`}
                              type="number"
                              min="0"
                              max={item.qty}
                              step="0.001"
                              value={String(returnQty[item.id] ?? 0)}
                              onChange={(e) =>
                                setReturnQty((prev) => ({
                                  ...prev,
                                  [item.id]: Number(e.target.value),
                                }))
                              }
                              className="w-28 rounded-md border px-2 py-1 text-sm"
                            />
                          </td>
                          <td className="px-3 py-2">
                            <label className="inline-flex items-center gap-2 text-xs">
                              <input
                                type="checkbox"
                                checked={restock[item.id] !== false}
                                onChange={(e) =>
                                  setRestock((prev) => ({
                                    ...prev,
                                    [item.id]: e.target.checked,
                                  }))
                                }
                              />
                              Repor
                            </label>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>

              <label className="mt-3 block">
                <span className="text-xs text-gray-600">Motivo da devolução</span>
                <input
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  placeholder="Ex.: produto com defeito"
                  className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                />
              </label>
              <div className="mt-3 flex justify-end">
                <button className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white">
                  Registrar devolução
                </button>
              </div>
            </form>
          ) : (
            <p className="mt-3 rounded-md bg-gray-50 p-2 text-sm text-gray-600">
              Esta venda não aceita novas devoluções no estado atual.
            </p>
          )}

          <div className="mt-5">
            <h4 className="text-sm font-semibold">Devoluções registradas</h4>
            <div className="mt-2 space-y-2">
              {returns.map((value) => {
                const remaining = Math.max(0, value.total_amount - value.refunded_amount)
                return (
                  <div key={value.id} className="rounded-md border p-3 text-sm">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div>
                        <div className="font-medium">{value.reason}</div>
                        <div className="mt-1 text-xs text-gray-600">
                          Devolvido: R$ {value.total_amount.toFixed(2)} • Reembolsado/crédito: R 
                          {value.refunded_amount.toFixed(2)} • Pendente: R$ {remaining.toFixed(2)}
                        </div>
                      </div>
                      {remaining > 0 ? (
                        <button
                          type="button"
                          onClick={() => startRefund(value)}
                          className="rounded-md border px-2 py-1 text-xs"
                        >
                          Registrar reembolso/crédito
                        </button>
                      ) : null}
                    </div>
                  </div>
                )
              })}
              {returns.length === 0 ? (
                <div className="rounded-md border p-4 text-center text-sm text-gray-500">
                  Nenhuma devolução registrada.
                </div>
              ) : null}
            </div>
          </div>

          {refundTarget ? (
            <form onSubmit={createRefund} className="mt-4 rounded-md border p-3">
              <h4 className="text-sm font-semibold">Reembolso / crédito</h4>
              <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-3">
                <label className="block">
                  <span className="text-xs text-gray-600">Forma</span>
                  <select
                    value={refundMethod}
                    onChange={(e) => setRefundMethod(e.target.value)}
                    className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                  >
                    {Object.entries(refundLabels).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="block">
                  <span className="text-xs text-gray-600">Valor</span>
                  <input
                    type="number"
                    min="0.01"
                    step="0.01"
                    value={String(refundAmount)}
                    onChange={(e) => setRefundAmount(Number(e.target.value))}
                    className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                  />
                </label>
                <label className="block">
                  <span className="text-xs text-gray-600">Referência externa</span>
                  <input
                    value={refundReference}
                    onChange={(e) => setRefundReference(e.target.value)}
                    placeholder="Opcional"
                    className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                  />
                </label>
              </div>
              <div className="mt-3 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setRefundTarget(null)}
                  className="rounded-md border px-3 py-2 text-sm"
                >
                  Cancelar
                </button>
                <button className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white">
                  Registrar
                </button>
              </div>
            </form>
          ) : null}
        </section>
      ) : null}
    </div>
  )
}
