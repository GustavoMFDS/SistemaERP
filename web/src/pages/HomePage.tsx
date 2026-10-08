import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'

type Overview = {
  sales_after_cancellations: number
  estimated_gross_profit: number
  refunds_recorded: number
  sales_count: number
  cancelled_count: number
}

type StockProduct = {
  id: string
  sku: string
  name: string
  unit: string
  qty_on_hand: number
  min_stock: number
}

type StockResponse = { items: StockProduct[]; total: number }
type Me = { name: string; permissions: string[] }

const money = (amount: number) => new Intl.NumberFormat('pt-BR', {
  style: 'currency', currency: 'BRL',
}).format(Number(amount) || 0)

function localDate(offsetDays = 0): string {
  const day = new Date()
  day.setDate(day.getDate() + offsetDays)
  const year = day.getFullYear()
  const month = String(day.getMonth() + 1).padStart(2, '0')
  return `${year}-${month}-${String(day.getDate()).padStart(2, '0')}`
}

const quickLinks = [
  { to: '/pdv', label: 'Abrir o caixa', permission: 'sale:write', detail: 'Registrar vendas e receber pagamentos' },
  { to: '/products', label: 'Cadastrar produtos', permission: 'product:write', detail: 'Adicionar um produto ou importar uma planilha' },
  { to: '/inventory', label: 'Conferir estoque', permission: 'inventory:read', detail: 'Ver itens em falta e entradas ou ajustes' },
  { to: '/purchases', label: 'Registrar compras', permission: 'procurement:read', detail: 'Acompanhar fornecedores e recebimentos' },
  { to: '/finance', label: 'Conferir pagamentos', permission: 'finance:read', detail: 'Ver divergências e conciliações' },
  { to: '/fiscal', label: 'Configurar nota fiscal', permission: 'invoice:read', detail: 'Seguir o passo a passo da NFC-e' },
]

function saveCSV(rows: string[][], name: string) {
  const content = '\uFEFF' + rows.map((row) =>
    row.map((cell) => '"' + cell.replace(/"/g, '""') + '"').join(';'),
  ).join('\r\n') + '\r\n'
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

export default function HomePage() {
  const [me, setMe] = useState<Me | null>(null)
  const [from, setFrom] = useState(localDate(-6))
  const [to, setTo] = useState(localDate())
  const [overview, setOverview] = useState<Overview | null>(null)
  const [stock, setStock] = useState<StockResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [overviewError, setOverviewError] = useState('')
  const [stockError, setStockError] = useState('')

  async function refresh(fromDate: string, toDate: string, permissions: string[]) {
    if (fromDate > toDate) {
      setError('A data inicial não pode ser posterior à data final.')
      return
    }
    setLoading(true)
    setError('')
    setOverviewError('')
    setStockError('')
    const finance = permissions.includes('finance:read')
    const inventory = permissions.includes('inventory:read')
    const qs = new URLSearchParams({ from: fromDate, to: toDate })
    const tasks: Promise<void>[] = []
    if (finance) {
      tasks.push(apiJson<{ overview: Overview }>(`/api/v1/finance/overview?${qs.toString()}`)
        .then((data) => setOverview(data.overview))
        .catch((e: unknown) => { setOverview(null); setOverviewError(errorMessage(e)) }))
    } else {
      setOverview(null)
    }
    if (inventory) {
      tasks.push(apiJson<StockResponse>('/api/v1/inventory/low-stock?limit=8')
        .then((data) => setStock({ ...data, items: data.items ?? [] }))
        .catch((e: unknown) => { setStock(null); setStockError(errorMessage(e)) }))
    } else {
      setStock(null)
    }
    await Promise.all(tasks)
    setLoading(false)
  }

  useEffect(() => {
    let cancelled = false
    apiJson<Me>('/api/v1/auth/me')
      .then((data) => {
        if (cancelled) return
        setMe(data)
        void refresh(localDate(-6), localDate(), data.permissions)
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(errorMessage(e))
      })
    return () => { cancelled = true }
  }, [])

  async function onFilter(event: FormEvent) {
    event.preventDefault()
    if (me) await refresh(from, to, me.permissions)
  }

  function exportSummary() {
    if (!overview) return
    saveCSV([
      ['Relatório da loja', 'Valor'],
      ['Período inicial', from],
      ['Período final', to],
      ['Vendas após cancelamentos (R$)', overview.sales_after_cancellations.toFixed(2).replace('.', ',')],
      ['Lucro bruto estimado antes de devoluções e despesas (R$)', overview.estimated_gross_profit.toFixed(2).replace('.', ',')],
      ['Reembolsos registrados (R$)', overview.refunds_recorded.toFixed(2).replace('.', ',')],
      ['Lançamentos de vendas', String(overview.sales_count)],
      ['Lançamentos de cancelamentos', String(overview.cancelled_count)],
      ['Observação', 'Valores por data de lançamento, não equivalem ao lucro contábil.'],
    ], `resumo-loja-${from}-a-${to}.csv`)
  }

  // The API serializes platform.Money as decimal BRL units, not integer cents.
  const reports = overview ? [
    { title: 'Vendas após cancelamentos', value: money(overview.sales_after_cancellations), detail: 'Valores lançados no período; não desconta devoluções' },
    { title: 'Lucro bruto estimado', value: money(overview.estimated_gross_profit), detail: 'Antes de devoluções, taxas, impostos e despesas' },
    { title: 'Reembolsos registrados', value: money(overview.refunds_recorded), detail: 'Valores devolvidos registrados no período' },
    { title: 'Vendas registradas', value: String(overview.sales_count), detail: `${overview.cancelled_count} cancelamento(s) lançado(s) no período` },
  ] : []

  const allowedLinks = quickLinks.filter((link) => me?.permissions.includes(link.permission))

  return (
    <div>
      <h2 className="text-lg font-semibold">Início — minha loja</h2>
      <p className="mt-1 text-sm text-gray-600">
        Olá{me?.name ? `, ${me.name}` : ''}. Escolha o que deseja fazer; o sistema mostra somente as áreas autorizadas para sua conta.
      </p>

      {error ? <p role="alert" className="mt-3 rounded-md border border-red-200 p-3 text-sm text-red-700">{error}</p> : null}

      <h3 className="mt-5 text-sm font-semibold">O que deseja fazer?</h3>
      <div className="mt-2 grid gap-3 sm:grid-cols-2">
        {allowedLinks.map((action) => (
          <Link key={action.to} to={action.to} className="rounded-lg border p-4 hover:border-blue-400 hover:bg-blue-50 focus:outline-none focus:ring-2 focus:ring-blue-400">
            <span className="block text-sm font-semibold">{action.label} →</span>
            <span className="mt-1 block text-xs text-gray-600">{action.detail}</span>
          </Link>
        ))}
      </div>
      {me && !allowedLinks.length ? <p className="mt-3 text-sm">Seu usuário ainda não possui permissões nesta loja. Peça acesso ao responsável.</p> : null}

      {me?.permissions.includes('finance:read') ? (
        <section className="mt-7">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-base font-semibold">Resumo das vendas</h3>
            <button type="button" onClick={exportSummary} disabled={!overview || loading}
              className="rounded-md border px-3 py-2 text-xs disabled:opacity-50">Exportar CSV</button>
          </div>
          <form onSubmit={(e) => void onFilter(e)} className="mt-3 flex flex-wrap items-end gap-3">
            <label className="text-xs text-gray-700">De
              <input type="date" required value={from} onChange={(e) => setFrom(e.target.value)}
                className="mt-1 block rounded-md border px-2 py-2 text-sm" />
            </label>
            <label className="text-xs text-gray-700">Até
              <input type="date" required value={to} onChange={(e) => setTo(e.target.value)}
                className="mt-1 block rounded-md border px-2 py-2 text-sm" />
            </label>
            <button disabled={loading} className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50">
              {loading ? 'Atualizando…' : 'Consultar período'}
            </button>
          </form>
          {overviewError ? <p role="alert" className="mt-2 text-sm text-red-700">Não foi possível carregar as vendas: {overviewError}</p> : null}
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            {reports.map((report) => (
              <div key={report.title} className="rounded-lg border p-4">
                <p className="text-xs text-gray-600">{report.title}</p>
                <p className="mt-2 text-xl font-semibold">{report.value}</p>
                <p className="mt-2 text-xs text-gray-600">{report.detail}</p>
              </div>
            ))}
          </div>
          <p className="mt-2 text-xs text-gray-500">
            Os valores vêm do livro financeiro da loja, por data de lançamento. Lucro estimado não é lucro contábil nem saldo bancário.
          </p>
        </section>
      ) : null}

      {me?.permissions.includes('inventory:read') ? (
        <section className="mt-7">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-base font-semibold">Produtos que precisam de atenção</h3>
            <Link to="/inventory" className="text-xs text-blue-700 underline">Abrir estoque completo</Link>
          </div>
          {stockError ? <p role="alert" className="mt-2 text-sm text-red-700">Falha ao carregar estoque: {stockError}</p> : null}
          {stock ? (
            <>
              <p className="mt-2 text-sm">
                <strong>{stock.total}</strong> produto(s) ativos no estoque mínimo ou abaixo em toda esta loja.
                {stock.total > stock.items.length ? ' Abaixo estão os primeiros itens prioritários.' : ''}
              </p>
              <div className="mt-2 overflow-x-auto rounded-md border">
                <table className="min-w-full text-left text-sm">
                  <thead className="bg-gray-50 text-xs"><tr>
                    <th className="px-3 py-2">Produto</th><th className="px-3 py-2">Quantidade</th><th className="px-3 py-2">Mínimo</th>
                  </tr></thead>
                  <tbody className="divide-y">
                    {stock.items.map((item) => (
                      <tr key={item.id}>
                        <td className="px-3 py-2">{item.name}<span className="block text-xs text-gray-500">{item.sku}</span></td>
                        <td className="px-3 py-2">{item.qty_on_hand.toFixed(2)} {item.unit}</td>
                        <td className="px-3 py-2">{item.min_stock.toFixed(2)} {item.unit}</td>
                      </tr>
                    ))}
                    {!stock.items.length ? <tr><td colSpan={3} className="px-3 py-4 text-center text-gray-600">Nenhum produto com estoque baixo.</td></tr> : null}
                  </tbody>
                </table>
              </div>
            </>
          ) : null}
        </section>
      ) : null}
    </div>
  )
}
