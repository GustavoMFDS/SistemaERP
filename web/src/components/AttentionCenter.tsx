import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'

type Fiscal = { total: number }
type Finance = { summary?: { overdue_open: number } }
type Stock = { total: number }
type Backup = { configured: boolean; state: string; last_run_utc?: string; archive_verified?: boolean }
type Alert = { title: string; detail: string; to: string; severity: 'warning' | 'info' }

export default function AttentionCenter({ permissions }: { permissions: string[] }) {
  const canFiscal = permissions.includes('invoice:read')
  const canFinance = permissions.includes('finance:read')
  const canStock = permissions.includes('inventory:read')
  const canBackup = permissions.includes('audit:read')
  const [alerts, setAlerts] = useState<Alert[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [refreshed, setRefreshed] = useState(false)

  async function refresh() {
    setLoading(true)
    setError('')
    const checks: Promise<Alert | null>[] = []
    if (canFiscal) checks.push(apiJson<Fiscal>('/api/v1/fiscal/obligations?unresolved=true&limit=1&offset=0').then((d) =>
      d.total > 0 ? { title: d.total + ' venda(s) com obrigação fiscal pendente',
        detail: 'Pode incluir NF-e 55, histórico e notas não autorizadas. A emissão não é confirmada pela venda.',
        to: '/fiscal', severity: 'warning' as const } : null))
    if (canFinance) checks.push(apiJson<Finance>('/api/v1/finance/accounts?kind=all&limit=1&offset=0').then((d) =>
      Number(d.summary?.overdue_open ?? 0) > 0 ? {
        title: 'Contas vencidas em aberto',
        detail: 'Há valores vencidos no financeiro. Confira os lançamentos; isso não representa extrato bancário.',
        to: '/finance', severity: 'warning' as const } : null))
    if (canStock) checks.push(apiJson<Stock>('/api/v1/inventory/low-stock?limit=1').then((d) =>
      d.total > 0 ? { title: d.total + ' produto(s) em reposição',
        detail: 'Estoque atual menor ou igual ao mínimo definido.',
        to: '/inventory', severity: 'info' as const } : null))
    if (canBackup) checks.push(apiJson<Backup>('/api/v1/audit/backup-status').then((d) => {
      if (d.configured && d.state === 'ok') return null
      return { title: d.configured ? 'Verificar backup: ' + d.state : 'Backup automático sem monitoramento',
        detail: d.configured ? 'O último backup não está comprovadamente recente e válido. Confira o agendamento e execute ensaio de restauração.' :
          'Configure a rotina de backup no servidor e conecte o arquivo de status. Sem esse controle não podemos garantir recuperação.',
        to: '/home', severity: 'warning' as const }
    }))
    const result = await Promise.allSettled(checks)
    const items = result.flatMap((r) => r.status === 'fulfilled' && r.value ? [r.value] : [])
    const failures = result.filter((r) => r.status === 'rejected')
    setAlerts(items)
    setRefreshed(true)
    setError(failures.length ? 'Não foi possível verificar todas as áreas. Alguns avisos podem estar ausentes. ' +
      errorMessage((failures[0] as PromiseRejectedResult).reason) : '')
    setLoading(false)
  }

  useEffect(() => {
    if (!canFiscal && !canFinance && !canStock && !canBackup) return
    void refresh()
  }, [canFiscal, canFinance, canStock, canBackup])

  if (!canFiscal && !canFinance && !canStock && !canBackup) return null
  return (
    <section aria-label="Central de atenção" className="rounded-2xl border border-slate-200 bg-white p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div><h3 className="text-lg font-bold text-slate-900">Atenção da loja</h3>
          <p className="text-sm text-slate-600">Avisos reais das áreas que você tem permissão para consultar.</p></div>
        <button type="button" onClick={() => void refresh()} disabled={loading} className="rounded-lg border px-3 py-2 text-sm disabled:opacity-50">
          {loading ? 'Verificando…' : 'Atualizar avisos'}
        </button>
      </div>
      {error && <p role="alert" className="mt-3 text-sm text-red-700">{error}</p>}
      {!loading && refreshed && alerts.length === 0 && !error && (
        <p className="mt-3 text-sm text-slate-600">Nenhuma pendência encontrada nas áreas consultadas. Não inclui verificação da SEFAZ em tempo real nem do backup.</p>
      )}
      {alerts.length > 0 && <div className="mt-4 grid gap-3 sm:grid-cols-2">
        {alerts.map((item) => (
          <Link key={item.to} to={item.to} className={'rounded-xl border p-4 hover:bg-slate-50 ' +
            (item.severity === 'warning' ? 'border-amber-300' : 'border-slate-200')}>
            <p className="font-semibold text-slate-900">{item.title} →</p>
            <p className="mt-1 text-sm text-slate-600">{item.detail}</p>
          </Link>
        ))}
      </div>}
      <p className="mt-3 text-xs text-slate-500">
        Backup: o estado do agendamento é consultado apenas para administradores autorizados; status OK comprova arquivo verificado, não um ensaio de restauração ou cópia fora do local.
        Vendas offline do navegador devem ser conferidas no PDV do próprio dispositivo.
      </p>
    </section>
  )
}
