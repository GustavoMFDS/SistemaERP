import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Supplier = {
  id: string
  name: string
  document?: string | null
  email?: string | null
  phone?: string | null
  contact_name?: string | null
  active: boolean
}

type Product = {
  id: string
  sku: string
  name: string
  cost_price: number
  active: boolean
}

type Purchase = {
  id: string
  supplier_id: string
  supplier_name: string
  status: 'ordered' | 'partially_received' | 'received' | 'cancelled'
  invoice_number?: string | null
  payment_due_date?: string | null
  total: number
  ordered_at: string
  received_at?: string | null
}

type PurchaseItem = {
  id: string
  purchase_id: string
  product_id: string
  product_sku: string
  product_name: string
  qty_ordered: number
  qty_received: number
  unit_cost: number
  line_total: number
}

type PurchaseDetail = {
  purchase: Purchase
  items: PurchaseItem[]
  receipts: Array<{ id: string; received_at: string }>
}

type DraftLine = {
  product_id: string
  qty: number
  unit_cost: number
}

const statusLabel: Record<Purchase['status'], string> = {
  ordered: 'Aguardando recebimento',
  partially_received: 'Recebida parcialmente',
  received: 'Recebida',
  cancelled: 'Cancelada',
}

export default function PurchasesPage() {
  const [suppliers, setSuppliers] = useState<Supplier[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [purchases, setPurchases] = useState<Purchase[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const [supplierName, setSupplierName] = useState('')
  const [supplierDocument, setSupplierDocument] = useState('')
  const [supplierPhone, setSupplierPhone] = useState('')

  const [supplierId, setSupplierId] = useState('')
  const [invoiceNumber, setInvoiceNumber] = useState('')
  const [paymentDueDate, setPaymentDueDate] = useState('')
  const [purchaseNotes, setPurchaseNotes] = useState('')
  const [lineProductId, setLineProductId] = useState('')
  const [lineQty, setLineQty] = useState(1)
  const [lineCost, setLineCost] = useState(0)
  const [lines, setLines] = useState<DraftLine[]>([])

  const [selected, setSelected] = useState<PurchaseDetail | null>(null)
  const [receiveQty, setReceiveQty] = useState<Record<string, number>>({})
  const [receiveNotes, setReceiveNotes] = useState('')

  const productById = useMemo(() => {
    const map = new Map<string, Product>()
    for (const product of products) map.set(product.id, product)
    return map
  }, [products])

  const purchaseTotal = useMemo(
    () => lines.reduce((sum, line) => sum + line.qty * line.unit_cost, 0),
    [lines],
  )

  async function loadBase() {
    setLoading(true)
    setError('')
    try {
      const [supplierData, productData, purchaseData] = await Promise.all([
        apiJson<{ items: Supplier[]; total: number }>('/api/v1/suppliers?limit=200&offset=0'),
        apiJson<{ items: Product[]; total: number }>('/api/v1/products?limit=200&offset=0'),
        apiJson<{ items: Purchase[]; total: number }>('/api/v1/purchases?limit=200&offset=0'),
      ])
      setSuppliers(supplierData.items)
      setProducts(productData.items.filter((product) => product.active))
      setPurchases(purchaseData.items)
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadBase()
  }, [])

  async function createSupplier(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      await apiJson<{ id: string }>('/api/v1/suppliers', {
        method: 'POST',
        body: {
          name: supplierName.trim(),
          document: supplierDocument.trim() || null,
          email: null,
          phone: supplierPhone.trim() || null,
          contact_name: null,
          notes: null,
          active: true,
        },
      })
      setSupplierName('')
      setSupplierDocument('')
      setSupplierPhone('')
      await loadBase()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function addLine() {
    const product = productById.get(lineProductId)
    if (!product || lineQty <= 0 || lineCost <= 0) return
    setLines((prev) => {
      const idx = prev.findIndex((line) => line.product_id === product.id)
      if (idx >= 0) {
        return prev.map((line, i) =>
          i === idx
            ? { ...line, qty: line.qty + lineQty, unit_cost: lineCost }
            : line,
        )
      }
      return [...prev, { product_id: product.id, qty: lineQty, unit_cost: lineCost }]
    })
    setLineProductId('')
    setLineQty(1)
    setLineCost(0)
  }

  async function createPurchase(e: FormEvent) {
    e.preventDefault()
    if (!supplierId || lines.length === 0) return
    setError('')
    try {
      await apiJson<{ id: string }>('/api/v1/purchases', {
        method: 'POST',
        body: {
          supplier_id: supplierId,
          invoice_number: invoiceNumber.trim() || null,
          payment_due_date: paymentDueDate || null,
          notes: purchaseNotes.trim() || null,
          items: lines,
        },
      })
      setSupplierId('')
      setInvoiceNumber('')
      setPaymentDueDate('')
      setPurchaseNotes('')
      setLines([])
      await loadBase()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function openPurchase(id: string) {
    setError('')
    try {
      const detail = await apiJson<PurchaseDetail>(`/api/v1/purchases/${id}`)
      setSelected(detail)
      const initial: Record<string, number> = {}
      for (const item of detail.items) initial[item.id] = 0
      setReceiveQty(initial)
      setReceiveNotes('')
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function fillRemaining() {
    if (!selected) return
    const next: Record<string, number> = {}
    for (const item of selected.items) {
      next[item.id] = Math.max(0, item.qty_ordered - item.qty_received)
    }
    setReceiveQty(next)
  }

  async function receivePurchase(e: FormEvent) {
    e.preventDefault()
    if (!selected) return
    const items = selected.items
      .map((item) => ({
        purchase_item_id: item.id,
        qty: Number(receiveQty[item.id]) || 0,
      }))
      .filter((item) => item.qty > 0)
    if (items.length === 0) return

    setError('')
    try {
      await apiJson<{ receipt_id: string; status: Purchase['status'] }>(
        `/api/v1/purchases/${selected.purchase.id}/receive`,
        {
          method: 'POST',
          body: { items, notes: receiveNotes.trim() || null },
        },
      )
      await loadBase()
      await openPurchase(selected.purchase.id)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function cancelPurchase(id: string) {
    if (!window.confirm('Cancelar esta compra? Isso só é permitido antes de qualquer recebimento.')) {
      return
    }
    setError('')
    try {
      await apiJson(`/api/v1/purchases/${id}/cancel`, { method: 'POST' })
      if (selected?.purchase.id === id) setSelected(null)
      await loadBase()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Compras e fornecedores</h2>
          <p className="text-sm text-gray-600">
            Registre pedidos e dê entrada no estoque somente quando a mercadoria chegar.
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

      <section className="mt-5 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Novo fornecedor</h3>
        <form onSubmit={createSupplier} className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-4">
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Nome</span>
            <input
              aria-label="Nome do fornecedor"
              value={supplierName}
              onChange={(e) => setSupplierName(e.target.value)}
              required
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">CNPJ/CPF</span>
            <input
              value={supplierDocument}
              onChange={(e) => setSupplierDocument(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Telefone</span>
            <input
              value={supplierPhone}
              onChange={(e) => setSupplierPhone(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <div className="md:col-span-4">
            <button className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white">
              Cadastrar fornecedor
            </button>
          </div>
        </form>
      </section>

      <section className="mt-5 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Nova compra</h3>
        <form onSubmit={createPurchase}>
          <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-4">
            <label className="block md:col-span-2">
              <span className="text-xs text-gray-600">Fornecedor</span>
              <select
                aria-label="Fornecedor da compra"
                value={supplierId}
                onChange={(e) => setSupplierId(e.target.value)}
                required
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              >
                <option value="">Selecione…</option>
                {suppliers
                  .filter((supplier) => supplier.active)
                  .map((supplier) => (
                    <option key={supplier.id} value={supplier.id}>
                      {supplier.name}
                    </option>
                  ))}
              </select>
            </label>
            <label className="block">
              <span className="text-xs text-gray-600">Documento/NF fornecedor</span>
              <input
                value={invoiceNumber}
                onChange={(e) => setInvoiceNumber(e.target.value)}
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              />
            </label>
            <label className="block">
              <span className="text-xs text-gray-600">Vencimento</span>
              <input
                type="date"
                value={paymentDueDate}
                onChange={(e) => setPaymentDueDate(e.target.value)}
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              />
            </label>
          </div>

          <div className="mt-3 grid grid-cols-1 gap-2 md:grid-cols-6">
            <label className="block md:col-span-3">
              <span className="text-xs text-gray-600">Produto</span>
              <select
                aria-label="Produto da compra"
                value={lineProductId}
                onChange={(e) => {
                  const id = e.target.value
                  setLineProductId(id)
                  const product = productById.get(id)
                  if (product) setLineCost(Number(product.cost_price) || 0)
                }}
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              >
                <option value="">Selecione…</option>
                {products.map((product) => (
                  <option key={product.id} value={product.id}>
                    {product.sku} — {product.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="block">
              <span className="text-xs text-gray-600">Qtd pedida</span>
              <input
                type="number"
                step="0.001"
                min="0.001"
                value={String(lineQty)}
                onChange={(e) => setLineQty(Number(e.target.value))}
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              />
            </label>
            <label className="block">
              <span className="text-xs text-gray-600">Custo unitário</span>
              <input
                type="number"
                step="0.01"
                min="0.01"
                value={String(lineCost)}
                onChange={(e) => setLineCost(Number(e.target.value))}
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              />
            </label>
            <button
              type="button"
              onClick={addLine}
              className="mt-5 rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
            >
              Incluir item
            </button>
          </div>

          {lines.length > 0 ? (
            <div className="mt-3 overflow-auto rounded-md border">
              <table className="min-w-full text-left text-sm">
                <thead className="bg-gray-50 text-xs text-gray-600">
                  <tr>
                    <th className="px-3 py-2">Produto</th>
                    <th className="px-3 py-2">Qtd</th>
                    <th className="px-3 py-2">Custo</th>
                    <th className="px-3 py-2">Total</th>
                    <th className="px-3 py-2"></th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {lines.map((line) => {
                    const product = productById.get(line.product_id)
                    return (
                      <tr key={line.product_id}>
                        <td className="px-3 py-2">{product?.name ?? line.product_id}</td>
                        <td className="px-3 py-2">{line.qty.toFixed(3)}</td>
                        <td className="px-3 py-2">R$ {line.unit_cost.toFixed(2)}</td>
                        <td className="px-3 py-2">R$ {(line.qty * line.unit_cost).toFixed(2)}</td>
                        <td className="px-3 py-2">
                          <button
                            type="button"
                            onClick={() =>
                              setLines((prev) =>
                                prev.filter((item) => item.product_id !== line.product_id),
                              )
                            }
                            className="text-xs text-red-700 hover:underline"
                          >
                            Remover
                          </button>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          ) : null}

          <label className="mt-3 block">
            <span className="text-xs text-gray-600">Observações</span>
            <textarea
              value={purchaseNotes}
              onChange={(e) => setPurchaseNotes(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>

          <div className="mt-3 flex items-center justify-between gap-3">
            <div className="text-sm font-semibold">Total: R$ {purchaseTotal.toFixed(2)}</div>
            <button
              disabled={!supplierId || lines.length === 0}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Criar compra
            </button>
          </div>
        </form>
      </section>

      <section className="mt-5">
        <h3 className="text-sm font-semibold">Compras</h3>
        <div className="mt-2 overflow-auto rounded-md border">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr>
                <th className="px-3 py-2">Fornecedor</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Documento</th>
                <th className="px-3 py-2">Total</th>
                <th className="px-3 py-2">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {purchases.map((purchase) => (
                <tr key={purchase.id}>
                  <td className="px-3 py-2">{purchase.supplier_name}</td>
                  <td className="px-3 py-2">{statusLabel[purchase.status]}</td>
                  <td className="px-3 py-2">{purchase.invoice_number || '—'}</td>
                  <td className="px-3 py-2">R$ {purchase.total.toFixed(2)}</td>
                  <td className="px-3 py-2">
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => void openPurchase(purchase.id)}
                        className="text-xs text-blue-700 hover:underline"
                      >
                        Detalhes/receber
                      </button>
                      {purchase.status === 'ordered' ? (
                        <button
                          type="button"
                          onClick={() => void cancelPurchase(purchase.id)}
                          className="text-xs text-red-700 hover:underline"
                        >
                          Cancelar
                        </button>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
              {purchases.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-3 py-6 text-center text-gray-500">
                    Nenhuma compra cadastrada.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </section>

      {selected ? (
        <section className="mt-5 rounded-md border p-3">
          <div className="flex items-start justify-between gap-2">
            <div>
              <h3 className="text-sm font-semibold">Recebimento da compra</h3>
              <p className="text-xs text-gray-600">
                {selected.purchase.supplier_name} • {statusLabel[selected.purchase.status]}
              </p>
            </div>
            <button type="button" onClick={() => setSelected(null)} className="text-xs underline">
              Fechar
            </button>
          </div>

          <form onSubmit={receivePurchase} className="mt-3">
            <div className="overflow-auto rounded-md border">
              <table className="min-w-full text-left text-sm">
                <thead className="bg-gray-50 text-xs text-gray-600">
                  <tr>
                    <th className="px-3 py-2">Produto</th>
                    <th className="px-3 py-2">Pedido</th>
                    <th className="px-3 py-2">Recebido</th>
                    <th className="px-3 py-2">Receber agora</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {selected.items.map((item) => {
                    const remaining = Math.max(0, item.qty_ordered - item.qty_received)
                    return (
                      <tr key={item.id}>
                        <td className="px-3 py-2">
                          {item.product_sku} — {item.product_name}
                        </td>
                        <td className="px-3 py-2">{item.qty_ordered.toFixed(3)}</td>
                        <td className="px-3 py-2">{item.qty_received.toFixed(3)}</td>
                        <td className="px-3 py-2">
                          <input
                            aria-label={`Receber ${item.product_name}`}
                            type="number"
                            step="0.001"
                            min="0"
                            max={remaining}
                            disabled={remaining <= 0}
                            value={String(receiveQty[item.id] ?? 0)}
                            onChange={(e) =>
                              setReceiveQty((prev) => ({
                                ...prev,
                                [item.id]: Number(e.target.value),
                              }))
                            }
                            className="w-32 rounded-md border px-2 py-1 text-sm"
                          />
                          <span className="ml-2 text-xs text-gray-500">resta {remaining.toFixed(3)}</span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {selected.purchase.status !== 'received' &&
            selected.purchase.status !== 'cancelled' ? (
              <>
                <label className="mt-3 block">
                  <span className="text-xs text-gray-600">Observação do recebimento</span>
                  <input
                    value={receiveNotes}
                    onChange={(e) => setReceiveNotes(e.target.value)}
                    className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                  />
                </label>
                <div className="mt-3 flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={fillRemaining}
                    className="rounded-md border px-3 py-2 text-sm"
                  >
                    Preencher restante
                  </button>
                  <button className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white">
                    Confirmar recebimento
                  </button>
                </div>
              </>
            ) : null}
          </form>

          {selected.receipts.length > 0 ? (
            <p className="mt-3 text-xs text-gray-500">
              Recebimentos registrados: {selected.receipts.length}
            </p>
          ) : null}
        </section>
      ) : null}
    </div>
  )
}
