import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Payment = {
  id: string
  sale_id: string
  method: string
  amount: number
  provider?: string | null
  transaction_ref?: string | null
  authorization_code?: string | null
  installments: number
  reconciliation_status: string
  reconciled_amount?: number | null
  reconciled_fee?: number | null
}

type Refund = {
  return_id: string
  sale_id: string
  kind: string
  reason: string
  refund_due: number
  settled_amount: number
  remaining_amount: number
  status: string
}

type ListResponse<T> = { items: T[]; total: number }

const METHOD_LABELS: Record<string, string> = {
  cash: 'Dinheiro',
  pix: 'PIX',
  debit: 'Débito',
  credit: 'Crédito',
  transfer: 'Transferência',
  voucher: 'Voucher',
}

export default function FinancePage() {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [method, setMethod] = useState('')
  const [paymentStatus, setPaymentStatus] = useState('')
  const [payments, setPayments] = useState<Payment[]>([])
  const [refunds, setRefunds] = useState<Refund[]>([])
  const [dashboard, setDashboard] = useState<Record<string, number>>({})
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [selectedPayment, setSelectedPayment] = useState<Payment | null>(null)
  const [receivedAmount, setReceivedAmount] = useState(0)
  const [feeAmount, setFeeAmount] = useState(0)
  const [provider, setProvider] = useState('')
  const [externalRef, setExternalRef] = useState('')
  const [reconcileNotes, setReconcileNotes] = useState('')
  const reconcileKeyRef = useRef('')

  const [selectedRefund, setSelectedRefund] = useState<Refund | null>(null)
  const [refundMethod, setRefundMethod] = useState('pix')
  const [refundAmount, setRefundAmount] = useState(0)
  const [refundProvider, setRefundProvider] = useState('')
  const [refundExternalRef, setRefundExternalRef] = useState('')
  const [refundCashSessionId, setRefundCashSessionId] = useState('')
  const [refundNotes, setRefundNotes] = useState('')
  const refundKeyRef = useRef('')

  async function loadAll(e?: FormEvent) {
    e?.preventDefault()
    setLoading(true)
    setError('')
    try {
      const qs = new URLSearchParams()
      if (from) qs.set('from', from)
      if (to) qs.set('to', to)
      if (method) qs.set('method', method)
      if (paymentStatus) qs.set('status', paymentStatus)
      qs.set('limit', '200')
      qs.set('offset', '0')

      const dashboardQs = new URLSearchParams()
      if (from) dashboardQs.set('from', from)
      if (to) dashboardQs.set('to', to)

      const [paymentData, refundData, dashboardData] = await Promise.all([
        apiJson<ListResponse<Payment>>(`/api/v1/finance/payments?${qs.toString()}`),
        apiJson<ListResponse<Refund>>('/api/v1/finance/refunds?limit=200&offset=0'),
        apiJson<{ totals: Record<string, number> }>(
          `/api/v1/finance/dashboard${dashboardQs.toString() ? `?${dashboardQs.toString()}` : ''}`,
        ),
      ])
      setPayments(paymentData.items)
      setRefunds(refundData.items)
      setDashboard(dashboardData.totals ?? {})
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadAll()
  }, [])

  function choosePayment(payment: Payment) {
    reconcileKeyRef.current = ''
    setSelectedPayment(payment)
    setReceivedAmount(payment.amount)
    setFeeAmount(0)
    setProvider(payment.provider ?? '')
    setExternalRef(payment.transaction_ref ?? '')
    setReconcileNotes('')
  }

  async function reconcilePayment() {
    if (!selectedPayment) return
    setError('')
    try {
      if (!reconcileKeyRef.current) reconcileKeyRef.current = crypto.randomUUID()
      await apiJson(`/api/v1/finance/payments/${selectedPayment.id}/reconcile`, {
        method: 'POST',
        headers: { 'Idempotency-Key': reconcileKeyRef.current },
        body: {
          received_amount: Number(receivedAmount) || 0,
          fee_amount: Number(feeAmount) || 0,
          provider: provider.trim() || null,
          external_ref: externalRef.trim() || null,
          notes: reconcileNotes.trim() || null,
        },
      })
      reconcileKeyRef.current = ''
      setSelectedPayment(null)
      await loadAll()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function chooseRefund(refund: Refund) {
    refundKeyRef.current = ''
    setSelectedRefund(refund)
    setRefundAmount(refund.remaining_amount)
    setRefundMethod('pix')
    setRefundProvider('')
    setRefundExternalRef('')
    setRefundCashSessionId('')
    setRefundNotes('')
  }

  async function settleRefund() {
    if (!selectedRefund) return
    setError('')
    try {
      if (!refundKeyRef.current) refundKeyRef.current = crypto.randomUUID()
      await apiJson(`/api/v1/finance/returns/${selectedRefund.return_id}/refunds`, {
        method: 'POST',
        headers: { 'Idempotency-Key': refundKeyRef.current },
        body: {
          method: refundMethod,
          amount: Number(refundAmount) || 0,
          provider: refundProvider.trim() || null,
          external_ref: refundExternalRef.trim() || null,
          cash_session_id: refundCashSessionId.trim() || null,
          notes: refundNotes.trim() || null,
        },
      })
      refundKeyRef.current = ''
      setSelectedRefund(null)
      await loadAll()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  const dashboardRows = useMemo(
    () => Object.entries(dashboard).sort(([a], [b]) => a.localeCompare(b)),
    [dashboard],
  )

  return (
    <div>
      <h2 className="text-base font-semibold">Financeiro e conciliação</h2>
      <p className="mt-1 text-sm text-gray-600">
        Concilie recebimentos por método e liquide reembolsos pendentes de devoluções.
      </p>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <form onSubmit={loadAll} className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-6">
        <label className="block">
          <span className="text-xs text-gray-600">De</span>
          <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
        </label>
        <label className="block">
          <span className="text-xs text-gray-600">Até</span>
          <input type="date" value={to} onChange={(e) => setTo(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
        </label>
        <label className="block">
          <span className="text-xs text-gray-600">Método</span>
          <select value={method} onChange={(e) => setMethod(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm">
            <option value="">Todos</option>
            {Object.entries(METHOD_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <label className="block">
          <span className="text-xs text-gray-600">Conciliação</span>
          <select value={paymentStatus} onChange={(e) => setPaymentStatus(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm">
            <option value="">Todas</option>
            <option value="pending">Pendente</option>
            <option value="reconciled">Conciliada</option>
            <option value="divergent">Divergente</option>
            <option value="not_applicable">Via caixa</option>
          </select>
        </label>
        <div className="flex items-end md:col-span-2">
          <button disabled={loading} className="rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-60">
            {loading ? 'Carregando…' : 'Atualizar'}
          </button>
        </div>
      </form>

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Resumo do ledger</h3>
        <div className="mt-2 grid grid-cols-2 gap-2 md:grid-cols-4">
          {dashboardRows.map(([key, value]) => (
            <div key={key} className="rounded-md bg-gray-50 p-2 text-xs">
              <div className="text-gray-600">{key}</div>
              <div className="mt-1 font-semibold">R$ {Number(value).toFixed(2)}</div>
            </div>
          ))}
          {dashboardRows.length === 0 ? <div className="text-xs text-gray-500">Sem lançamentos.</div> : null}
        </div>
      </div>

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Pagamentos</h3>
        <div className="mt-2 overflow-auto">
          <table className="min-w-full text-left text-sm">
            <thead className="text-xs text-gray-600">
              <tr>
                <th className="px-2 py-2">Venda</th>
                <th className="px-2 py-2">Método</th>
                <th className="px-2 py-2">Valor</th>
                <th className="px-2 py-2">Referência</th>
                <th className="px-2 py-2">Status</th>
                <th className="px-2 py-2"></th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {payments.map((payment) => (
                <tr key={payment.id}>
                  <td className="px-2 py-2 font-mono text-xs">{payment.sale_id}</td>
                  <td className="px-2 py-2">{METHOD_LABELS[payment.method] ?? payment.method}</td>
                  <td className="px-2 py-2">R$ {payment.amount.toFixed(2)}</td>
                  <td className="px-2 py-2 text-xs">{payment.provider ?? '—'} / {payment.transaction_ref ?? '—'}</td>
                  <td className="px-2 py-2">{payment.reconciliation_status}</td>
                  <td className="px-2 py-2">
                    {payment.method === 'cash' ? (
                      <span className="text-xs text-gray-500">Fechamento do caixa</span>
                    ) : payment.reconciliation_status === 'pending' ? (
                      <button type="button" onClick={() => choosePayment(payment)} className="rounded-md border px-2 py-1 text-xs">
                        Conciliar
                      </button>
                    ) : (
                      <span className="text-xs text-gray-500">Já conciliado</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {selectedPayment ? (
          <div className="mt-3 rounded-md border bg-gray-50 p-3">
            <div className="text-sm font-semibold">Conciliar pagamento</div>
            <div className="mt-2 grid gap-2 md:grid-cols-3">
              <label className="text-xs">Recebido
                <input type="number" step="0.01" min="0" value={receivedAmount} onChange={(e) => setReceivedAmount(Number(e.target.value))} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">Taxa
                <input type="number" step="0.01" min="0" value={feeAmount} onChange={(e) => setFeeAmount(Number(e.target.value))} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">Adquirente/provedor
                <input value={provider} onChange={(e) => setProvider(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">ID externo
                <input value={externalRef} onChange={(e) => setExternalRef(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs md:col-span-2">Observação
                <input value={reconcileNotes} onChange={(e) => setReconcileNotes(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
            </div>
            <div className="mt-2 flex gap-2">
              <button type="button" onClick={() => void reconcilePayment()} className="rounded-md bg-gray-900 px-3 py-2 text-xs text-white">Salvar conciliação</button>
              <button type="button" onClick={() => { reconcileKeyRef.current = ''; setSelectedPayment(null) }} className="rounded-md border px-3 py-2 text-xs">Cancelar</button>
            </div>
          </div>
        ) : null}
      </div>

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Reembolsos de devoluções</h3>
        <div className="mt-2 overflow-auto">
          <table className="min-w-full text-left text-sm">
            <thead className="text-xs text-gray-600">
              <tr>
                <th className="px-2 py-2">Venda</th>
                <th className="px-2 py-2">Devido</th>
                <th className="px-2 py-2">Liquidado</th>
                <th className="px-2 py-2">Restante</th>
                <th className="px-2 py-2">Status</th>
                <th className="px-2 py-2"></th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {refunds.map((refund) => (
                <tr key={refund.return_id}>
                  <td className="px-2 py-2 font-mono text-xs">{refund.sale_id}</td>
                  <td className="px-2 py-2">R$ {refund.refund_due.toFixed(2)}</td>
                  <td className="px-2 py-2">R$ {refund.settled_amount.toFixed(2)}</td>
                  <td className="px-2 py-2">R$ {refund.remaining_amount.toFixed(2)}</td>
                  <td className="px-2 py-2">{refund.status}</td>
                  <td className="px-2 py-2">
                    {refund.remaining_amount > 0 ? (
                      <button type="button" onClick={() => chooseRefund(refund)} className="rounded-md border px-2 py-1 text-xs">Liquidar</button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {selectedRefund ? (
          <div className="mt-3 rounded-md border bg-gray-50 p-3">
            <div className="text-sm font-semibold">Liquidar reembolso</div>
            <div className="mt-2 grid gap-2 md:grid-cols-3">
              <label className="text-xs">Método
                <select value={refundMethod} onChange={(e) => setRefundMethod(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm">
                  {Object.entries(METHOD_LABELS).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
              </label>
              <label className="text-xs">Valor
                <input type="number" step="0.01" min="0.01" max={selectedRefund.remaining_amount} value={refundAmount} onChange={(e) => setRefundAmount(Number(e.target.value))} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">Sessão de caixa
                <input value={refundCashSessionId} onChange={(e) => setRefundCashSessionId(e.target.value)} placeholder={refundMethod === 'cash' ? 'Obrigatória' : 'Opcional'} className="mt-1 w-full rounded-md border px-2 py-2 font-mono text-sm" />
              </label>
              <label className="text-xs">Provedor
                <input value={refundProvider} onChange={(e) => setRefundProvider(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">ID externo
                <input value={refundExternalRef} onChange={(e) => setRefundExternalRef(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">Observação
                <input value={refundNotes} onChange={(e) => setRefundNotes(e.target.value)} className="mt-1 w-full rounded-md border px-2 py-2 text-sm" />
              </label>
            </div>
            <p className="mt-2 text-xs text-gray-600">
              Em dinheiro, informe um caixa aberto. Para PIX/cartão, informar a sessão é opcional e permite abater o reembolso da conciliação daquele fechamento.
            </p>
            <div className="mt-2 flex gap-2">
              <button type="button" onClick={() => void settleRefund()} className="rounded-md bg-gray-900 px-3 py-2 text-xs text-white">Registrar liquidação</button>
              <button type="button" onClick={() => { refundKeyRef.current = ''; setSelectedRefund(null) }} className="rounded-md border px-3 py-2 text-xs">Cancelar</button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
}
