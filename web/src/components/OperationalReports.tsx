import { useCallback, useEffect, useState } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Report = {
  from: string
  to: string
  hourly: { hour: number; count: number; total: number }[]
  margins: { sku: string; name: string; quantity: number; revenue_items: number; estimated_margin: number }[]
  unsold: { sku: string; name: string }[]
  unsold_total: number
  cash_discrepancies: number
  overdue_accounts: number
}
const money = (value: number) => new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(value)
const localDate = (days: number) => {
  const d = new Date()
  d.setDate(d.getDate() + days)
  return d.toLocaleDateString('en-CA', { timeZone: 'America/Sao_Paulo' })
}
export default function OperationalReports() {
  const [from, setFrom] = useState(() => localDate(-13))
  const [to, setTo] = useState(() => localDate(0))
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async (a: string, b: string) => {
    setBusy(true)
    setError('')
    setReport(null)
    try {
      const result = await apiJson<Report>('/api/v1/finance/operational-reports?from=' +
        encodeURIComponent(a) + '&to=' + encodeURIComponent(b))
      setReport(result)
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }, [])
  useEffect(() => { void refresh(localDate(-13), localDate(0)) }, [refresh]) // initial snapshot, manual refresh afterwards

  const hourly = Array.from({ length: 24 }, (_, hour) =>
    report?.hourly.find((h) => h.hour === hour) ?? { hour, count: 0, total: 0 })
  const maxCount = Math.max(1, ...hourly.map((x) => x.count))

  return (
    <section aria-label="Relatórios operacionais" className="mt-6 space-y-4 rounded-2xl border border-slate-200 bg-white p-5">
      <h3 className="text-lg font-bold text-slate-900">Indicadores para administrar a loja</h3>
      <p className="text-sm text-slate-600">Vendas por horário, itens que mais geraram margem estimada, produtos sem vendas no período e pendências operacionais.</p>
      <form onSubmit={(e) => { e.preventDefault(); void refresh(from, to) }} className="flex flex-wrap items-end gap-3">
        <label className="text-xs text-slate-700">De
          <input type="date" required value={from} onChange={(e) => setFrom(e.target.value)} className="mt-1 block rounded-lg border px-3 py-2 text-sm" />
        </label>
        <label className="text-xs text-slate-700">Até
          <input type="date" required value={to} onChange={(e) => setTo(e.target.value)} className="mt-1 block rounded-lg border px-3 py-2 text-sm" />
        </label>
        <button disabled={busy} className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
          {busy ? 'Consultando…' : 'Consultar (até 93 dias)'}
        </button>
      </form>
      {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
      {report && (
        <>
          <div className="grid gap-3 sm:grid-cols-3">
            {[
              { title: 'Produtos sem venda no período', value: report.unsold_total, tip: 'Produtos ativos sem venda finalizada nesse intervalo' },
              { title: 'Fechamentos com diferença', value: report.cash_discrepancies, tip: 'Sessões fechadas com diferença de caixa no intervalo' },
              { title: 'Contas vencidas hoje', value: report.overdue_accounts, tip: 'Contas abertas vencidas, independente do período escolhido' },
            ].map((x) => (
              <div key={x.title} className="rounded-xl bg-slate-50 p-4">
                <p className="text-xs font-medium text-slate-600">{x.title}</p>
                <p className="mt-2 text-2xl font-bold">{x.value}</p>
                <p className="mt-1 text-xs text-slate-500">{x.tip}</p>
              </div>
            ))}
          </div>
          <div>
            <h4 className="font-semibold">Horários com vendas</h4>
            <p className="text-xs text-slate-500">Quantidades de vendas finalizadas; exclui vendas canceladas, sem abater devoluções.</p>
            <div className="mt-3 grid grid-cols-6 gap-2 sm:grid-cols-12">
              {hourly.map((h) => (
                <div key={h.hour} className="min-w-0 text-center" title={h.hour + 'h: ' + h.count + ' venda(s), ' + money(h.total)}>
                  <div className="flex h-14 items-end justify-center rounded bg-slate-100">
                    <div className="w-full rounded-t bg-blue-600" style={{ height: (h.count ? Math.max(8, 100 * h.count / maxCount) : 0) + '%' }} />
                  </div>
                  <span className="text-[10px] text-slate-600">{String(h.hour).padStart(2, '0')}h</span>
                </div>
              ))}
            </div>
          </div>
          <div className="overflow-x-auto">
            <h4 className="font-semibold">Margem bruta estimada por produto</h4>
            <p className="mt-1 text-xs text-amber-800">Valores calculados sobre itens de vendas finalizadas. Não equivalem a lucro contábil; não deduzem descontos gerais da venda, devoluções, tributos, taxas nem despesas.</p>
            {report.margins.length ? (
              <table className="mt-3 min-w-full text-left text-sm">
                <thead><tr className="border-b text-xs text-slate-600"><th className="p-2">Produto</th><th className="p-2 text-right">Quantidade</th><th className="p-2 text-right">Receita dos itens</th><th className="p-2 text-right">Margem estimada</th></tr></thead>
                <tbody>{report.margins.map((p) => (
                  <tr key={p.sku} className="border-b"><td className="p-2">{p.name}<span className="block text-xs text-slate-500">Código: {p.sku}</span></td><td className="p-2 text-right">{p.quantity}</td><td className="p-2 text-right">{money(p.revenue_items)}</td><td className="p-2 text-right">{money(p.estimated_margin)}</td></tr>
                ))}</tbody>
              </table>
            ) : <p className="mt-2 text-sm text-slate-500">Nenhuma venda finalizada no período.</p>}
          </div>
          {report.unsold.length > 0 && (
            <details className="rounded-lg bg-slate-50 p-3 text-sm">
              <summary className="cursor-pointer font-semibold">Exemplos de produtos sem venda (até 10)</summary>
              <ul className="mt-2 list-disc pl-5">{report.unsold.map((p) => <li key={p.sku}>{p.name} — código {p.sku}</li>)}</ul>
            </details>
          )}
        </>
      )}
    </section>
  )
}
