import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import TopProductsReport from '../components/TopProductsReport'

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

type ReconciliationInitial = {
  id: string
  payment_id: string
  expected_amount: number
  received_amount: number
  fee_amount: number
  net_amount: number
  difference_amount: number
  status: string
  provider?: string | null
  external_ref?: string | null
  notes?: string | null
  created_at: string
}

type ReconciliationAdjustment = {
  id: string
  payment_id: string
  previous_received_amount: number
  previous_fee_amount: number
  new_received_amount: number
  new_fee_amount: number
  difference_amount: number
  status: string
  notes?: string | null
  created_at: string
}

type ReconciliationHistory = {
  initial: ReconciliationInitial
  adjustments: ReconciliationAdjustment[]
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
  const [reconciling, setReconciling] = useState(false)
  const reconcileKeyRef = useRef('')

  const [selectedAdjustment, setSelectedAdjustment] = useState<Payment | null>(null)
  const [adjustReceivedAmount, setAdjustReceivedAmount] = useState(0)
  const [adjustFeeAmount, setAdjustFeeAmount] = useState(0)
  const [adjustNotes, setAdjustNotes] = useState('')
  const [adjusting, setAdjusting] = useState(false)
  const adjustKeyRef = useRef('')

  const [historyPayment, setHistoryPayment] = useState<Payment | null>(null)
  const [reconciliationHistory, setReconciliationHistory] = useState<ReconciliationHistory | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)

  const [selectedRefund, setSelectedRefund] = useState<Refund | null>(null)
  const [refundMethod, setRefundMethod] = useState('pix')
  const [refundAmount, setRefundAmount] = useState(0)
  const [refundProvider, setRefundProvider] = useState('')
  const [refundExternalRef, setRefundExternalRef] = useState('')
  const [refundCashSessionId, setRefundCashSessionId] = useState('')
  const [refundNotes, setRefundNotes] = useState('')
  const [settlingRefund, setSettlingRefund] = useState(false)
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
    adjustKeyRef.current = ''
    setSelectedAdjustment(null)
    setSelectedPayment(payment)
    setReceivedAmount(payment.amount)
    setFeeAmount(0)
    setProvider(payment.provider ?? '')
    setExternalRef('')
    setReconcileNotes('')
  }

  async function reconcilePayment() {
    if (!selectedPayment || reconciling) return
    const received = Number(receivedAmount)
    const fee = Number(feeAmount)
    if (!Number.isFinite(received) || received < 0 || !Number.isFinite(fee) || fee < 0 || fee > received) {
      setError('Informe valores válidos: a taxa não pode superar o valor recebido.')
      return
    }
    if (Boolean(provider.trim()) !== Boolean(externalRef.trim())) {
      setError('Provedor e ID externo devem ser informados juntos.')
      return
    }
    setError('')
    setReconciling(true)
    try {
      if (!reconcileKeyRef.current) reconcileKeyRef.current = crypto.randomUUID()
      await apiJson(`/api/v1/finance/payments/${selectedPayment.id}/reconcile`, {
        method: 'POST',
        headers: { 'Idempotency-Key': reconcileKeyRef.current },
        body: {
          received_amount: Math.round(received * 100) / 100,
          fee_amount: Math.round(fee * 100) / 100,
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
    } finally {
      setReconciling(false)
    }
  }

  async function showReconciliationHistory(payment: Payment) {
    setHistoryPayment(payment)
    setReconciliationHistory(null)
    setHistoryLoading(true)
    setError('')
    try {
      const data = await apiJson<ReconciliationHistory>(
        `/api/v1/finance/payments/${payment.id}/reconciliation-history`,
      )
      setReconciliationHistory(data)
    } catch (e: unknown) {
      setError(errorMessage(e))
      setHistoryPayment(null)
    } finally {
      setHistoryLoading(false)
    }
  }

  function chooseAdjustment(payment: Payment) {
    adjustKeyRef.current = ''
    reconcileKeyRef.current = ''
    setSelectedPayment(null)
    setSelectedAdjustment(payment)
    setAdjustReceivedAmount(payment.reconciled_amount ?? payment.amount)
    setAdjustFeeAmount(payment.reconciled_fee ?? 0)
    setAdjustNotes('')
  }

  async function adjustPaymentReconciliation() {
    if (!selectedAdjustment || adjusting) return
    const received = Number(adjustReceivedAmount)
    const fee = Number(adjustFeeAmount)
    const notes = adjustNotes.trim()
    if (!Number.isFinite(received) || received < 0 || !Number.isFinite(fee) || fee < 0 || fee > received) {
      setError('Informe valores válidos: a taxa não pode superar o valor recebido.')
      return
    }
    if (notes.length < 3) {
      setError('Informe uma justificativa para o ajuste da conciliação.')
      return
    }
    setError('')
    setAdjusting(true)
    try {
      if (!adjustKeyRef.current) adjustKeyRef.current = crypto.randomUUID()
      await apiJson(`/api/v1/finance/payments/${selectedAdjustment.id}/reconciliation-adjustments`, {
        method: 'POST',
        headers: { 'Idempotency-Key': adjustKeyRef.current },
        body: {
          received_amount: Math.round(received * 100) / 100,
          fee_amount: Math.round(fee * 100) / 100,
          notes,
        },
      })
      const adjustedPayment = selectedAdjustment
      adjustKeyRef.current = ''
      setSelectedAdjustment(null)
      await loadAll()
      if (historyPayment?.id === adjustedPayment.id) {
        await showReconciliationHistory(adjustedPayment)
      }
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setAdjusting(false)
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
    if (!selectedRefund || settlingRefund) return
    const amount = Number(refundAmount)
    if (
      !Number.isFinite(amount) ||
      amount <= 0 ||
      amount > Number(selectedRefund.remaining_amount)
    ) {
      setError('Informe um valor de reembolso maior que zero e dentro do saldo restante.')
      return
    }
    if (refundMethod === 'cash' && !refundCashSessionId.trim()) {
      setError('Reembolso em dinheiro exige uma sessão de caixa aberta.')
      return
    }
    if (refundMethod === 'cash' && (refundProvider.trim() || refundExternalRef.trim())) {
      setError('Reembolso em dinheiro não usa provedor nem ID externo.')
      return
    }
    if (Boolean(refundProvider.trim()) !== Boolean(refundExternalRef.trim())) {
      setError('Provedor e ID externo devem ser informados juntos.')
      return
    }
    setError('')
    setSettlingRefund(true)
    try {
      if (!refundKeyRef.current) refundKeyRef.current = crypto.randomUUID()
      await apiJson(`/api/v1/finance/returns/${selectedRefund.return_id}/refunds`, {
        method: 'POST',
        headers: { 'Idempotency-Key': refundKeyRef.current },
        body: {
          method: refundMethod,
          amount: Math.round(amount * 100) / 100,
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
    } finally {
      setSettlingRefund(false)
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

      <TopProductsReport />

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
                      <div className="flex gap-1">
                        <button type="button" onClick={() => chooseAdjustment(payment)} className="rounded-md border px-2 py-1 text-xs">
                          Ajustar
                        </button>
                        <button type="button" onClick={() => void showReconciliationHistory(payment)} className="rounded-md border px-2 py-1 text-xs">
                          Histórico
                        </button>
                      </div>
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
              <button type="button" disabled={reconciling} onClick={() => void reconcilePayment()} className="rounded-md bg-gray-900 px-3 py-2 text-xs text-white disabled:opacity-60">{reconciling ? 'Salvando…' : 'Salvar conciliação'}</button>
              <button type="button" onClick={() => { reconcileKeyRef.current = ''; setSelectedPayment(null) }} className="rounded-md border px-3 py-2 text-xs">Cancelar</button>
            </div>
          </div>
        ) : null}
        {selectedAdjustment ? (
          <div className="mt-3 rounded-md border bg-gray-50 p-3">
            <div className="text-sm font-semibold">Ajustar conciliação</div>
            <p className="mt-1 text-xs text-gray-600">
              O ajuste não apaga a conciliação original; ele registra uma correção auditável do valor recebido/taxa atuais.
            </p>
            <div className="mt-2 grid gap-2 md:grid-cols-3">
              <label className="text-xs">Recebido corrigido
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={adjustReceivedAmount}
                  onChange={(e) => setAdjustReceivedAmount(Number(e.target.value))}
                  className="mt-1 w-full rounded-md border px-2 py-2 text-sm"
                />
              </label>
              <label className="text-xs">Taxa corrigida
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  value={adjustFeeAmount}
                  onChange={(e) => setAdjustFeeAmount(Number(e.target.value))}
                  className="mt-1 w-full rounded-md border px-2 py-2 text-sm"
                />
              </label>
              <label className="text-xs">Justificativa
                <input
                  value={adjustNotes}
                  onChange={(e) => setAdjustNotes(e.target.value)}
                  placeholder="Motivo da correção"
                  className="mt-1 w-full rounded-md border px-2 py-2 text-sm"
                />
              </label>
            </div>
            <div className="mt-2 flex gap-2">
              <button
                type="button"
                disabled={adjusting}
                onClick={() => void adjustPaymentReconciliation()}
                className="rounded-md bg-gray-900 px-3 py-2 text-xs text-white disabled:opacity-60"
              >
                {adjusting ? 'Ajustando…' : 'Salvar ajuste'}
              </button>
              <button
                type="button"
                disabled={adjusting}
                onClick={() => { adjustKeyRef.current = ''; setSelectedAdjustment(null) }}
                className="rounded-md border px-3 py-2 text-xs disabled:opacity-60"
              >
                Cancelar
              </button>
            </div>
          </div>
        ) : null}

        {historyPayment ? (
          <div className="mt-3 rounded-md border bg-white p-3">
            <div className="flex items-center justify-between gap-2">
              <div>
                <div className="text-sm font-semibold">Histórico da conciliação</div>
                <div className="font-mono text-xs text-gray-500">{historyPayment.id}</div>
              </div>
              <button
                type="button"
                onClick={() => { setHistoryPayment(null); setReconciliationHistory(null) }}
                className="rounded-md border px-2 py-1 text-xs"
              >
                Fechar
              </button>
            </div>
            {historyLoading ? (
              <div className="mt-2 text-xs text-gray-500">Carregando histórico…</div>
            ) : reconciliationHistory ? (
              <div className="mt-3 space-y-3">
                <div className="rounded-md bg-gray-50 p-2 text-xs">
                  <div className="font-semibold">Conciliação inicial • {reconciliationHistory.initial.status}</div>
                  <div className="mt-1">
                    Esperado R$ {reconciliationHistory.initial.expected_amount.toFixed(2)} • recebido R$ {reconciliationHistory.initial.received_amount.toFixed(2)} • taxa R$ {reconciliationHistory.initial.fee_amount.toFixed(2)} • diferença R$ {reconciliationHistory.initial.difference_amount.toFixed(2)}
                  </div>
                  <div className="mt-1 text-gray-600">
                    {reconciliationHistory.initial.provider ?? 'sem provedor'} / {reconciliationHistory.initial.external_ref ?? 'sem referência externa'} • {new Date(reconciliationHistory.initial.created_at).toLocaleString()}
                  </div>
                  {reconciliationHistory.initial.notes ? <div className="mt-1">{reconciliationHistory.initial.notes}</div> : null}
                </div>
                {reconciliationHistory.adjustments.length === 0 ? (
                  <div className="text-xs text-gray-500">Nenhum ajuste posterior.</div>
                ) : (
                  <div className="space-y-2">
                    {reconciliationHistory.adjustments.map((adjustment, index) => (
                      <div key={adjustment.id} className="rounded-md border p-2 text-xs">
                        <div className="font-semibold">Ajuste {index + 1} • {adjustment.status}</div>
                        <div className="mt-1">
                          Recebido R$ {adjustment.previous_received_amount.toFixed(2)} → R$ {adjustment.new_received_amount.toFixed(2)}
                        </div>
                        <div>
                          Taxa R$ {adjustment.previous_fee_amount.toFixed(2)} → R$ {adjustment.new_fee_amount.toFixed(2)} • diferença R$ {adjustment.difference_amount.toFixed(2)}
                        </div>
                        <div className="mt-1 text-gray-600">{new Date(adjustment.created_at).toLocaleString()}</div>
                        {adjustment.notes ? <div className="mt-1">{adjustment.notes}</div> : null}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ) : null}
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
              <button type="button" disabled={settlingRefund} onClick={() => void settleRefund()} className="rounded-md bg-gray-900 px-3 py-2 text-xs text-white disabled:opacity-60">{settlingRefund ? 'Registrando…' : 'Registrar liquidação'}</button>
              <button type="button" onClick={() => { refundKeyRef.current = ''; setSelectedRefund(null) }} className="rounded-md border px-3 py-2 text-xs">Cancelar</button>
            </div>
          </div>
        ) : null}
      </div>
    </div>
  )
}
