import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Account = {
  id: string
  kind: 'payable' | 'receivable'
  description: string
  amount: number
  due_date: string
  status: string
  settled_at: string | null
  settlement_method: string | null
  purchase_id?: string | null
}
type TrendPoint = { date: string; inflow: number; outflow: number }
type ListResponse = { items: Account[]; total: number; limit: number; offset: number; truncated: boolean; summary?: { payable_open: number; receivable_open: number; overdue_open: number } }
type TrendResponse = { items: TrendPoint[]; days: number }
const money = (n: number) => new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(n)
const PAYMENT_METHODS = [
  ['transfer', 'Transferência'], ['cash', 'Dinheiro'], ['pix', 'Pix (registrado)'],
  ['debit', 'Débito'], ['credit', 'Crédito'], ['other', 'Outro'],
] as const
const today = () => new Date().toLocaleDateString('en-CA', { timeZone: 'America/Sao_Paulo' })

export default function FinancialOverview() {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [trends, setTrends] = useState<TrendPoint[]>([])
  const [canManage, setCanManage] = useState(false)
  const [days, setDays] = useState('14')
  const [view, setView] = useState<'all' | 'payable' | 'receivable'>('all')
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [accountCount, setAccountCount] = useState(0)
  const [pageOffset, setPageOffset] = useState(0)
  const [balances, setBalances] = useState({ payable: 0, receivable: 0, overdue: 0 })
  const [summaryReady, setSummaryReady] = useState(false)
  const pageSize = 80
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ kind: 'payable', description: '', amount: '', due_date: today() })
  const [settle, setSettle] = useState<string | null>(null)
  const [settleMethod, setSettleMethod] = useState('transfer')
  const [settleNote, setSettleNote] = useState('')
  const createKey = useRef('')
  const settlementKeys = useRef<Record<string, string>>({})

  async function refresh(withTrend = true) {
    setLoading(true)
    setError('')
    try {
      const [list, chart] = await Promise.all([
        apiJson<ListResponse>(`/api/v1/finance/accounts?kind=${view}&limit=${pageSize}&offset=${pageOffset}`),
        withTrend ? apiJson<TrendResponse>(`/api/v1/finance/trends?days=${days}`) : Promise.resolve(null),
      ])
      setAccounts(Array.isArray(list.items) ? list.items : [])
      setAccountCount(list.total)
      if (list.summary) {
        setBalances({ payable: Number(list.summary.payable_open), receivable: Number(list.summary.receivable_open), overdue: Number(list.summary.overdue_open) })
        setSummaryReady(true)
      } else {
        setSummaryReady(false)
        setError('O servidor precisa ser atualizado para mostrar o total correto de todas as contas. A lista continua disponível.')
      }
      if (chart) setTrends(Array.isArray(chart.items) ? chart.items : [])
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let active = true
    apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((data) => { if (active) setCanManage(data.permissions?.includes('finance:reconcile') ?? false) })
      .catch(() => { if (active) setCanManage(false) })
    return () => { active = false }
  }, [])

  useEffect(() => { void refresh() }, [days, view, pageOffset]) // requested period and account page

  const visible = accounts // already tenant-scoped and filtered/paginated by the API
  const chartMax = Math.max(1, ...trends.flatMap((item) => [Math.max(0, item.inflow), Math.abs(item.outflow)]))
  const chartIn = trends.reduce((sum, item) => sum + item.inflow, 0)
  const chartOut = trends.reduce((sum, item) => sum + Math.abs(item.outflow), 0)

  async function addAccount(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    const value = Number(form.amount)
    if (!Number.isFinite(value) || value <= 0 || form.description.trim().length < 3 || !form.due_date) {
      setError('Informe descrição, valor maior que zero e vencimento.'); return
    }
    setBusy(true); setError(''); setNotice('')
    if (!createKey.current) createKey.current = crypto.randomUUID()
    try {
      await apiJson('/api/v1/finance/accounts', {
        method: 'POST',
        headers: { 'Idempotency-Key': createKey.current },
        body: { ...form, description: form.description.trim(), amount: value },
      })
      createKey.current = ''
      setForm({ kind: 'payable', description: '', amount: '', due_date: today() })
      setShowCreate(false)
      setNotice('Conta registrada. Ela não movimentou o saldo bancário.')
      await refresh(false)
    } catch (e: unknown) {
      setError(`Não foi possível confirmar o cadastro: ${errorMessage(e)}. Repita sem alterar os dados para evitar duplicação.`)
    } finally { setBusy(false) }
  }

  async function settleAccount(item: Account) {
    if (busy) return
    setBusy(true); setError(''); setNotice('')
    if (!settlementKeys.current[item.id]) settlementKeys.current[item.id] = crypto.randomUUID()
    try {
      await apiJson(`/api/v1/finance/accounts/${item.kind}/${item.id}/settle`, {
        method: 'POST',
        headers: { 'Idempotency-Key': settlementKeys.current[item.id] },
        body: { method: settleMethod, note: settleNote.trim() },
      })
      delete settlementKeys.current[item.id]
      setSettle(null); setSettleNote('')
      setNotice('Baixa registrada com auditoria e lançamento financeiro.')
      await refresh()
    } catch (e: unknown) {
      setError(`Baixa não confirmada: ${errorMessage(e)}. Confira antes de tentar novamente.`)
    } finally { setBusy(false) }
  }

  return (
    <section aria-label="Painel de contas e evolução" className="space-y-6">
      <div className="grid gap-3 sm:grid-cols-3">
        {[
          ['A receber', balances.receivable, 'Valores abertos a receber'],
          ['A pagar', balances.payable, 'Despesas ainda não baixadas'],
          ['Vencidas', balances.overdue, 'Contas abertas após o vencimento'],
        ].map(([title, amount, caption]) => (
          <div key={String(title)} className="rounded-2xl border border-slate-200 bg-white p-5">
            <p className="text-sm font-medium text-slate-600">{title}</p>
            <p className="mt-2 text-2xl font-bold tracking-tight text-slate-900">{summaryReady ? money(Number(amount)) : '—'}</p>
            <p className="mt-1 text-xs text-slate-500">{caption}</p>
          </div>
        ))}
      </div>
      <p className="text-xs text-slate-500">{summaryReady ? "Os totais acima consideram todas as contas da loja, inclusive as que não estão na página atual." : "Os totais estão indisponíveis até atualizar o servidor. Não use os valores exibidos na lista como saldo da loja."}</p>

      <section aria-labelledby="trends-title" className="rounded-2xl border border-slate-200 bg-white p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 id="trends-title" className="text-base font-bold">Movimento financeiro</h3>
            <p className="mt-1 text-xs text-slate-500">Lançamentos registrados no sistema; não representam extrato bancário.</p>
          </div>
          <label className="text-xs font-medium text-slate-600">Período
            <select aria-label="Período do gráfico" className="ml-2 rounded-lg border px-3 py-2" value={days} onChange={(e) => setDays(e.target.value)}>
              <option value="7">7 dias</option><option value="14">14 dias</option><option value="30">30 dias</option><option value="90">90 dias</option>
            </select>
          </label>
        </div>
        <div className="mt-4 flex flex-wrap gap-4 text-xs font-medium">
          <span className="text-emerald-700">Entradas: {money(chartIn)}</span>
          <span className="text-rose-700">Saídas e estornos: {money(chartOut)}</span>
        </div>
        <div role="img" aria-label="Gráfico de barras: entradas em verde e saídas em rosa por dia" className="mt-5 flex h-44 items-end gap-1 overflow-hidden border-b border-slate-200 pb-1">
          {trends.map((point) => (
            <div key={point.date} className="flex h-full min-w-0 flex-1 items-end justify-center gap-px" title={`${point.date}: entradas ${money(point.inflow)}, saídas ${money(Math.abs(point.outflow))}`}>
              <div className="w-1/2 rounded-t bg-emerald-500" style={{ height: `${Math.max(1, Math.max(0,point.inflow) / chartMax * 100)}%` }} />
              <div className="w-1/2 rounded-t bg-rose-400" style={{ height: `${Math.max(1, Math.abs(point.outflow) / chartMax * 100)}%` }} />
            </div>
          ))}
        </div>
        <div className="mt-2 flex justify-between text-xs text-slate-500">
          <span>{trends[0]?.date ?? 'Sem lançamentos'}</span><span>{trends[trends.length-1]?.date ?? ''}</span>
        </div>
      </section>

      <section aria-labelledby="accounts-title" className="rounded-2xl border border-slate-200 bg-white p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div><h3 id="accounts-title" className="text-base font-bold">Contas da loja</h3>
            <p className="mt-1 text-xs text-slate-500">Contas de compras e lançamentos manuais de despesas e recebimentos.</p></div>
          {canManage ? <button type="button" onClick={() => { setShowCreate((v)=>!v); setError('') }}
            className="rounded-lg bg-slate-900 px-4 py-3 text-sm font-semibold text-white"> {showCreate ? 'Fechar formulário' : 'Nova conta'} </button> : null}
        </div>
        {notice ? <p role="status" className="mt-4 rounded-lg bg-emerald-50 p-3 text-sm text-emerald-800">{notice}</p> : null}
        {error ? <p role="alert" className="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-800">{error}</p> : null}
        {showCreate && canManage ? (
          <form onSubmit={(e)=>void addAccount(e)} className="mt-5 grid gap-3 rounded-xl bg-slate-50 p-4 sm:grid-cols-2">
            <label className="text-sm">Tipo
              <select value={form.kind} onChange={(e)=>{ createKey.current=''; setForm({...form,kind:e.target.value}) }} className="mt-1 block w-full rounded-lg border px-3 py-2">
                <option value="payable">Conta a pagar</option><option value="receivable">Conta a receber</option>
              </select>
            </label>
            <label className="text-sm">Vencimento
              <input required type="date" value={form.due_date} onChange={(e)=>{createKey.current='';setForm({...form,due_date:e.target.value})}} className="mt-1 block w-full rounded-lg border px-3 py-2" />
            </label>
            <label className="text-sm sm:col-span-2">Descrição
              <input required minLength={3} maxLength={250} placeholder="Ex.: aluguel do mês" value={form.description} onChange={(e)=>{createKey.current='';setForm({...form,description:e.target.value})}} className="mt-1 block w-full rounded-lg border px-3 py-2" />
            </label>
            <label className="text-sm">Valor (R$)
              <input required type="number" min="0.01" step="0.01" value={form.amount} onChange={(e)=>{createKey.current='';setForm({...form,amount:e.target.value})}} className="mt-1 block w-full rounded-lg border px-3 py-2" />
            </label>
            <button disabled={busy} type="submit" className="self-end rounded-lg bg-slate-900 px-4 py-3 text-sm font-semibold text-white disabled:opacity-50">
              {busy ? 'Salvando…' : 'Cadastrar conta'}
            </button>
          </form>
        ) : null}
        <div className="mt-5 flex flex-wrap gap-2">
          {([['all','Todas'],['payable','A pagar'],['receivable','A receber']] as const).map(([value,label])=>(
            <button key={value} type="button" onClick={()=>{setView(value);setPageOffset(0);setSettle(null)}}
              aria-pressed={view===value}
              className={view===value?'rounded-full bg-slate-900 px-4 py-2 text-xs font-semibold text-white':'rounded-full border px-4 py-2 text-xs font-medium text-slate-700'}>
              {label}
            </button>
          ))}
          <button type="button" onClick={()=>void refresh()} disabled={loading} className="ml-auto rounded-lg border px-3 py-2 text-xs">Atualizar</button>
        </div>
        <div className="mt-4 space-y-2">
          {visible.map((item)=>(
            <article key={item.id} className="flex flex-col justify-between gap-3 rounded-xl border border-slate-200 p-4 sm:flex-row sm:items-center">
              <div className="min-w-0">
                <p className="font-semibold text-slate-900">{item.description}</p>
                <p className="mt-1 text-xs text-slate-500">
                  {item.kind==='payable'?'A pagar':'A receber'} • vence {item.due_date} • {item.status==='open'?'Aberta':item.status==='paid'?'Paga':item.status==='received'?'Recebida':item.status}
                </p>
              </div>
              <div className="flex shrink-0 items-center justify-between gap-3">
                <span className="font-bold tabular-nums">{money(item.amount)}</span>
                {canManage && item.status==='open' ? (
                  <button type="button" className="rounded-lg border border-slate-300 px-3 py-2 text-xs font-semibold" onClick={()=>{setSettle(item.id===settle?null:item.id);setSettleNote('');setSettleMethod('transfer')}}>Dar baixa</button>
                ) : null}
              </div>
              {settle === item.id ? (
                <div className="flex flex-wrap items-end gap-2 border-t pt-3 sm:basis-full">
                  <label className="text-xs">Forma registrada
                    <select value={settleMethod} onChange={(e)=>setSettleMethod(e.target.value)} className="mt-1 block rounded-lg border px-2 py-2">
                      {PAYMENT_METHODS.map(([v,l])=><option key={v} value={v}>{l}</option>)}
                    </select>
                  </label>
                  <label className="text-xs">Observação opcional
                    <input value={settleNote} onChange={(e)=>setSettleNote(e.target.value)} maxLength={500} className="mt-1 block rounded-lg border px-2 py-2" />
                  </label>
                  <button disabled={busy} onClick={()=>void settleAccount(item)} className="rounded-lg bg-slate-900 px-4 py-2.5 text-xs font-semibold text-white disabled:opacity-50">Confirmar baixa</button>
                </div>
              ) : null}
            </article>
          ))}
          {visible.length === 0 ? <p className="rounded-xl bg-slate-50 p-6 text-center text-sm text-slate-500">Nenhuma conta nessa categoria.</p> : null}
          <nav aria-label="Páginas de contas" className="flex flex-wrap items-center justify-between gap-3 pt-4 text-sm">
            <span className="text-slate-600">
              {accountCount === 0 ? 'Nenhuma conta' : `Exibindo ${pageOffset + 1}–${pageOffset + visible.length} de ${accountCount} contas`}
            </span>
            <div className="flex gap-2">
              <button type="button" disabled={loading || pageOffset === 0}
                onClick={()=>setPageOffset((value)=>Math.max(0,value-pageSize))}
                className="rounded-lg border px-4 py-2 disabled:opacity-40">Anterior</button>
              <button type="button" disabled={loading || pageOffset + visible.length >= accountCount}
                onClick={()=>setPageOffset((value)=>value+pageSize)}
                className="rounded-lg border px-4 py-2 disabled:opacity-40">Próxima</button>
            </div>
          </nav>
        </div>
      </section>
    </section>
  )
}
