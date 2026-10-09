import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type RankedProduct = {
  product_id: string
  sku: string
  name: string
  quantity: string
  item_total: string
  sales_count: number
}
type Result = { items: RankedProduct[]; from: string; to: string; limit: number }

function brazilDate(daysBack: number): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'America/Sao_Paulo', year: 'numeric', month: '2-digit', day: '2-digit',
  }).format(new Date())
  const date = new Date(parts + 'T12:00:00Z')
  date.setUTCDate(date.getUTCDate() - daysBack)
  return date.toISOString().slice(0, 10)
}

function formatNumber(value: string, digits: number): string {
  const n = Number(value)
  if (!Number.isFinite(n)) return value
  return new Intl.NumberFormat('pt-BR', {
    minimumFractionDigits: digits, maximumFractionDigits: digits,
  }).format(n)
}

export default function TopProductsReport() {
  const [start, setStart] = useState(() => brazilDate(29))
  const [end, setEnd] = useState(() => brazilDate(0))
  const [applied, setApplied] = useState<{ from: string; to: string } | null>(null)
  const [result, setResult] = useState<Result | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const requestSeq = useRef(0)
  const scope = getSessionScope()

  useEffect(() => {
    const id = ++requestSeq.current
    if (!scope || !applied) return
    const originalScope = scope
    setLoading(true)
    setError('')
    setResult(null)
    const params = new URLSearchParams({ ...applied, limit: '20' })
    void apiJson<Result>(`/api/v1/finance/products-ranking?${params.toString()}`).then((data) => {
      if (id !== requestSeq.current || originalScope !== getSessionScope()) return
      setResult(data)
      setLoading(false)
    }).catch((cause: unknown) => {
      if (id !== requestSeq.current || originalScope !== getSessionScope()) return
      setError(errorMessage(cause))
      setLoading(false)
    })
    return () => { ++requestSeq.current }
  }, [applied, scope])

  const valid = start.length === 10 && end.length === 10 &&
    start <= end && (Date.parse(end + 'T00:00:00Z') - Date.parse(start + 'T00:00:00Z')) <= 366 * 86400000

  function consult(event: FormEvent) {
    event.preventDefault()
    if (!valid) return
    setApplied({ from: start, to: end })
  }

  return (
    <section aria-label="Produtos mais vendidos" className="mt-5 rounded-md border p-3">
      <h3 className="text-sm font-semibold">Produtos com maior valor vendido</h3>
      <p className="mt-1 text-xs text-gray-600">
        Ranking dos itens de vendas finalizadas nesta loja. Cancelamentos não entram.
        Não desconta devoluções, reembolsos, taxas, tributos ou despesas; não é lucro.
      </p>
      <form onSubmit={consult} className="mt-3 flex flex-wrap items-end gap-3">
        <label className="text-xs">Data inicial
          <input type="date" required value={start}
            onChange={(e) => setStart(e.target.value)}
            className="mt-1 block rounded-md border px-2 py-2 text-sm" />
        </label>
        <label className="text-xs">Data final
          <input type="date" required value={end}
            onChange={(e) => setEnd(e.target.value)}
            className="mt-1 block rounded-md border px-2 py-2 text-sm" />
        </label>
        <button type="submit" disabled={loading || !valid}
          className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50">
          {loading ? 'Consultando…' : 'Ver ranking'}
        </button>
      </form>
      {!valid ? <p className="mt-2 text-xs text-red-700" role="alert">
        Informe datas válidas, em ordem, com intervalo máximo de 366 dias.
      </p> : null}
      {loading ? <p role="status" className="mt-3 text-xs">Carregando produtos…</p> : null}
      {error ? <p role="alert" className="mt-3 text-xs text-red-700">{error}</p> : null}
      {result && result.items.length === 0 ? (
        <p className="mt-3 text-xs text-gray-600">Nenhuma venda finalizada neste período.</p>
      ) : null}
      {result && result.items.length > 0 ? (
        <div className="mt-3 overflow-auto">
          <table className="min-w-full text-left text-xs">
            <thead className="bg-gray-50 text-gray-600">
              <tr><th className="px-2 py-2">Produto</th><th className="px-2 py-2">Quantidade</th>
                <th className="px-2 py-2">Vendas</th><th className="px-2 py-2">Total dos itens (R$)</th></tr>
            </thead>
            <tbody className="divide-y">
              {result.items.map((item) => (
                <tr key={item.product_id}>
                  <td className="px-2 py-2">{item.name}<span className="block text-gray-500">{item.sku}</span></td>
                  <td className="px-2 py-2">{formatNumber(item.quantity, 3)}</td>
                  <td className="px-2 py-2">{item.sales_count}</td>
                  <td className="px-2 py-2">{formatNumber(item.item_total, 2)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  )
}
