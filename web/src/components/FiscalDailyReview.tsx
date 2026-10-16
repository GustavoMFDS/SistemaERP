import { useState } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Daily = {
  date: string
  total: number
  nfce: number
  nfe: number
  authorized_nfce: number
  unresolved: number
  manual_review: number
  cancelled_sales: number
  integrity_alert: number
}
const today = () => new Date().toLocaleDateString('en-CA', { timeZone: 'America/Sao_Paulo' })

export default function FiscalDailyReview() {
  const [date, setDate] = useState(today)
  const [data, setData] = useState<Daily | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function query() {
    setBusy(true)
    setError('')
    setData(null)
    try {
      setData(await apiJson<Daily>('/api/v1/fiscal/daily-close?date=' + encodeURIComponent(date)))
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  function downloadCSV() {
    if (!data) return
    const rows = [
      ['Conferência operacional fiscal (não substitui SEFAZ)', data.date],
      ['Obrigações de venda registradas', String(data.total)],
      ['NFC-e previstas', String(data.nfce)],
      ['NF-e 55 previstas (emissor não concluído)', String(data.nfe)],
      ['NFC-e com autorização e arquivo íntegro', String(data.authorized_nfce)],
      ['Obrigações sem comprovação suficiente', String(data.unresolved)],
      ['Precisam de revisão', String(data.manual_review)],
      ['Vendas canceladas para conferir', String(data.cancelled_sales)],
      ['Inconsistências de arquivo autorizado', String(data.integrity_alert)],
    ]
    const value = '\uFEFF' + rows.map((r) => r.map((c) => '"' + c.replaceAll('"', '""') + '"').join(';')).join('\r\n') + '\r\n'
    const url = URL.createObjectURL(new Blob([value], { type: 'text/csv;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = 'conferencia-fiscal-' + data.date + '.csv'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <section aria-label="Conferência fiscal diária" className="mt-5 rounded-2xl border border-slate-200 bg-white p-5">
      <h3 className="text-lg font-bold text-slate-900">Conferência diária das notas</h3>
      <p className="mt-1 text-sm text-slate-600">
        Compare as vendas que exigem documento com as NFC-e realmente autorizadas e arquivadas.
        Conferência interna por dia de registro da obrigação, não fechamento fiscal legal nem consulta ao vivo na SEFAZ.
      </p>
      <form className="mt-4 flex flex-wrap items-end gap-3" onSubmit={(e) => { e.preventDefault(); void query() }}>
        <label className="text-sm text-slate-700">Dia
          <input type="date" required value={date} onChange={(e) => { setDate(e.target.value); setData(null) }}
            className="mt-1 block rounded-lg border px-3 py-2" />
        </label>
        <button type="submit" disabled={busy} className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">
          {busy ? 'Conferindo…' : 'Conferir dia'}
        </button>
        <button type="button" onClick={downloadCSV} disabled={!data}
          className="rounded-lg border px-4 py-2 text-sm disabled:opacity-50">Exportar resumo CSV</button>
      </form>
      {error && <p role="alert" className="mt-3 text-sm text-red-700">{error}</p>}
      {data && (
        <>
          <div className="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
            {[
              ['Obrigações', data.total],
              ['NFC-e comprovadas', data.authorized_nfce],
              ['Sem comprovação', data.unresolved],
              ['Revisão necessária', data.manual_review],
              ['NFC-e previstas', data.nfce],
              ['NF-e previstas', data.nfe],
              ['Vendas canceladas', data.cancelled_sales],
              ['Integridade pendente', data.integrity_alert],
            ].map(([name, value]) => (
              <div key={name} className="rounded-xl border bg-slate-50 p-3">
                <p className="text-xs text-slate-600">{name}</p>
                <p className="mt-2 text-2xl font-bold text-slate-900">{value}</p>
              </div>
            ))}
          </div>
          {data.unresolved > 0
            ? <p className="mt-3 text-sm text-amber-800">Existem pendências. Consulte a lista de vendas abaixo e não retransmita documentos automaticamente.</p>
            : <p className="mt-3 text-sm text-slate-600">Nenhuma obrigação sem comprovação identificada para esse dia na base local. Isso não atesta homologação fiscal externa.</p>}
        </>
      )}
    </section>
  )
}
