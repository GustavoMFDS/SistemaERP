import { useCallback, useEffect, useState } from 'react'
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
  { to: '/setup', label: 'Configurar minha loja', permission: 'product:write', detail: 'Passo a passo para quem está começando' },
  { to: '/staff', label: 'Gerenciar funcionários', permission: 'team:manage', detail: 'Convidar pessoas e definir gerente ou caixa para esta loja' },
  { to: '/pdv', label: 'Abrir o caixa', permission: 'sale:write', detail: 'Registrar vendas e receber pagamentos' },
  { to: '/products', label: 'Cadastrar produtos', permission: 'product:write', detail: 'Adicionar um produto ou importar uma planilha' },
  { to: '/inventory', label: 'Conferir estoque', permission: 'inventory:read', detail: 'Ver itens em falta e entradas ou ajustes' },
  { to: '/stock-movements', label: 'Histórico do estoque', permission: 'inventory:read', detail: 'Acompanhar entradas, saídas, perdas e ajustes por produto' },
  { to: '/imports', label: 'Ver histórico das importações', permission: 'imports:history', detail: 'Lotes confirmados de produtos e estoque em ordem de data' },
  { to: '/purchases', label: 'Registrar compras', permission: 'procurement:read', detail: 'Acompanhar fornecedores e recebimentos' },
  { to: '/customers', label: 'Cadastrar clientes', permission: 'customer:read', detail: 'Guardar contatos de clientes desta loja com privacidade' },
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
  const [reportPeriod, setReportPeriod] = useState<{ from: string; to: string } | null>(null)
  const [stock, setStock] = useState<StockResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [overviewError, setOverviewError] = useState('')
  const [stockError, setStockError] = useState('')

  const refresh = useCallback(async (fromDate: string, toDate: string, permissions: string[]) => {
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
        .then((data) => { setOverview(data.overview); setReportPeriod({ from: fromDate, to: toDate }) })
        .catch((e: unknown) => { setOverview(null); setReportPeriod(null); setOverviewError(errorMessage(e)) }))
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
  }, [])

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
  }, [refresh])

  async function onFilter(event: FormEvent) {
    event.preventDefault()
    if (me) await refresh(from, to, me.permissions)
  }

  function exportSummary() {
    if (!overview || !reportPeriod) return
    saveCSV([
      ['Relatório da loja', 'Valor'],
      ['Período inicial', reportPeriod.from],
      ['Período final', reportPeriod.to],
      ['Vendas após cancelamentos (R$)', overview.sales_after_cancellations.toFixed(2).replace('.', ',')],
      ['Lucro bruto estimado antes de devoluções e despesas (R$)', overview.estimated_gross_profit.toFixed(2).replace('.', ',')],
      ['Reembolsos registrados (R$)', overview.refunds_recorded.toFixed(2).replace('.', ',')],
      ['Lançamentos de vendas', String(overview.sales_count)],
      ['Lançamentos de cancelamentos', String(overview.cancelled_count)],
      ['Observação', 'Valores por data de lançamento, não equivalem ao lucro contábil.'],
    ], `resumo-loja-${reportPeriod.from}-a-${reportPeriod.to}.csv`)
  }

  // The API serializes platform.Money as decimal BRL units, not integer cents.
  const reports = overview ? [
    { title: 'Vendas após cancelamentos', value: money(overview.sales_after_cancellations), detail: 'Valores lançados no período; não desconta devoluções' },
    { title: 'Lucro bruto estimado', value: money(overview.estimated_gross_profit), detail: 'Antes de devoluções, taxas, impostos e despesas' },
    { title: 'Reembolsos registrados', value: money(overview.refunds_recorded), detail: 'Valores devolvidos registrados no período' },
    { title: 'Vendas registradas', value: String(overview.sales_count), detail: `${overview.cancelled_count} cancelamento(s) lançado(s) no período` },
  ] : []

  const allowedLinks = quickLinks.filter((link) => link.permission === 'imports:history'
    ? (me?.permissions.includes('product:write') || me?.permissions.includes('inventory:adjust'))
    : me?.permissions.includes(link.permission))

  const mainActions = allowedLinks.filter((action) => ['/pdv', '/products', '/inventory', '/finance'].includes(action.to))
  const otherActions = allowedLinks.filter((action) => !mainActions.some((main) => main.to === action.to))

  return (
    <div className="space-y-7">
      <header className="relative overflow-hidden rounded-3xl bg-slate-900 p-7 text-white sm:p-9">
        <div className="pointer-events-none absolute -right-12 -top-20 h-56 w-56 rounded-full border-[32px] border-white/10" aria-hidden="true" />
        <div className="relative z-10">
          <p className="text-xs font-bold uppercase tracking-[0.2em] text-emerald-300">Visão da sua loja</p>
          <h2 className="mt-3 max-w-xl text-3xl font-bold tracking-tight sm:text-4xl">
            Olá{me?.name ? `, ${me.name}` : ''}. Sua loja está em movimento.
          </h2>
          <p className="mt-3 max-w-lg text-sm leading-relaxed text-slate-300">
            Vendas, produtos e o que merece sua atenção — sem precisar procurar em várias telas.
          </p>
          <div className="mt-7 flex flex-wrap gap-3">
            {me?.permissions.includes('sale:write') ? (
              <Link to="/pdv" className="rounded-xl bg-emerald-400 px-5 py-3 text-sm font-bold text-slate-950 hover:bg-emerald-300">
                Ir para o caixa →
              </Link>
            ) : null}
            {me?.permissions.includes('inventory:read') ? (
              <Link to="/inventory" className="rounded-xl border border-white/30 px-5 py-3 text-sm font-semibold text-white hover:bg-white/10">
                Ver estoque
              </Link>
            ) : null}
          </div>
        </div>
      </header>

      {error ? <p role="alert" className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">{error}</p> : null}

      {me?.permissions.includes('finance:read') ? (
        <section aria-label="Resumo da semana" className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div><h3 className="text-lg font-bold text-slate-900">O ritmo da loja</h3>
              <p className="text-xs text-slate-500">Indicadores registrados no período selecionado; não são extrato bancário.</p></div>
            <Link to="/finance" className="text-sm font-semibold text-blue-700 hover:underline">Abrir financeiro →</Link>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {reports.map((report) => (
              <div key={report.title} className="rounded-2xl border border-slate-200 bg-white p-5">
                <p className="text-xs font-medium text-slate-500">{report.title}</p>
                <p className="mt-3 text-2xl font-bold tracking-tight text-slate-900">{report.value}</p>
                <p className="mt-2 text-xs leading-relaxed text-slate-500">{report.detail}</p>
              </div>
            ))}
            {!overview && !overviewError ? <p className="text-sm text-slate-500">Carregando indicadores…</p> : null}
          </div>
          {overviewError ? <p role="alert" className="text-sm text-red-700">Falha ao consultar vendas: {overviewError}</p> : null}
          <details className="rounded-xl border border-slate-200 bg-white p-4">
            <summary className="cursor-pointer text-sm font-semibold text-slate-700">Consultar outro período e exportar resumo</summary>
            <form onSubmit={(e)=>void onFilter(e)} className="mt-4 flex flex-wrap items-end gap-3">
              <label className="text-xs text-slate-600">De
                <input type="date" required value={from} onChange={(e)=>setFrom(e.target.value)} className="mt-1 block rounded-lg border px-3 py-2 text-sm" /></label>
              <label className="text-xs text-slate-600">Até
                <input type="date" required value={to} onChange={(e)=>setTo(e.target.value)} className="mt-1 block rounded-lg border px-3 py-2 text-sm" /></label>
              <button disabled={loading} className="rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white">{loading ? 'Consultando…' : 'Consultar período'}</button>
              <button type="button" onClick={exportSummary} disabled={!overview || loading}
                className="rounded-lg border px-4 py-2 text-sm disabled:opacity-50">Exportar CSV</button>
            </form>
          </details>
        </section>
      ) : null}

      {me?.permissions.includes('inventory:read') ? (
        <section aria-label="Alertas de estoque" className="rounded-2xl border border-slate-200 p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 className="text-lg font-bold text-slate-900">Precisa de atenção</h3>
              <p className="mt-1 text-sm text-slate-600">
                {stock ? (stock.total === 0 ? 'Seu estoque está dentro dos mínimos cadastrados.' : `${stock.total} produto(s) com estoque baixo.`) : 'Conferindo estoque…'}
              </p>
            </div>
            <Link to="/inventory" className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold">Abrir estoque →</Link>
          </div>
          {stockError ? <p role="alert" className="mt-3 text-sm text-red-700">{stockError}</p> : null}
          {stock?.items?.length ? (
            <div className="mt-4 grid gap-2 sm:grid-cols-2">
              {stock.items.slice(0,4).map((item)=>(
                <div key={item.id} className="flex items-center justify-between gap-3 rounded-xl bg-amber-50 p-3 text-sm">
                  <div className="min-w-0"><p className="font-semibold text-slate-900">{item.name}</p><p className="text-xs text-slate-600">{item.sku}</p></div>
                  <span className="shrink-0 text-xs font-bold text-amber-900">{item.qty_on_hand.toFixed(2)} / {item.min_stock.toFixed(2)} {item.unit}</span>
                </div>
              ))}
            </div>
          ) : null}
        </section>
      ) : null}

      <section aria-label="Acessos principais">
        <h3 className="text-lg font-bold text-slate-900">O que vamos fazer agora?</h3>
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          {mainActions.map((action)=>(
            <Link key={action.to} to={action.to} className="group rounded-2xl border border-slate-200 bg-white p-5 transition-colors hover:border-slate-400 hover:bg-slate-50">
              <span className="text-base font-bold text-slate-900">{action.label} <span className="text-blue-700 group-hover:translate-x-1">→</span></span>
              <p className="mt-2 text-sm leading-relaxed text-slate-600">{action.detail}</p>
            </Link>
          ))}
        </div>
        {otherActions.length > 0 ? (
          <details className="mt-4 rounded-xl border border-slate-200 bg-white p-4">
            <summary className="cursor-pointer text-sm font-semibold text-slate-700">Outras áreas e configurações</summary>
            <div className="mt-4 grid gap-2 sm:grid-cols-2">
              {otherActions.map((action)=>(
                <Link key={action.to} to={action.to} className="rounded-lg border p-3 text-sm hover:bg-slate-50">
                  <span className="font-semibold">{action.label} →</span><span className="mt-1 block text-xs text-slate-500">{action.detail}</span>
                </Link>
              ))}
            </div>
          </details>
        ) : null}
        {me && allowedLinks.length===0 ? <p className="mt-3 text-sm">Sua conta ainda não tem áreas liberadas nesta loja. Peça acesso ao responsável.</p> : null}
      </section>
    </div>
  )
}
