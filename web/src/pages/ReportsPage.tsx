import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import OperationalReports from '../components/OperationalReports'
import TopProductsReport from '../components/TopProductsReport'
import { apiJson } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type AccessStatus = 'checking' | 'allowed' | 'denied' | 'error'

export default function ReportsPage() {
  const scope = getSessionScope()
  const [access, setAccess] = useState<AccessStatus>('checking')

  useEffect(() => {
    let active = true
    // Revalidate permissions with the server; hiding a menu item is not authorization.
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((me) => {
        if (active) setAccess(me.permissions.includes('finance:read') ? 'allowed' : 'denied')
      })
      .catch(() => { if (active) setAccess('error') })
    return () => { active = false }
  }, [scope])

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-bold tracking-tight text-slate-900">Relatórios da loja</h2>
        <p className="mt-2 text-sm text-slate-600">
          Confira os horários com mais vendas, os produtos mais vendidos e o que precisa de atenção.
          Cada relatório possui seu próprio período de consulta.
        </p>
      </div>
      {access === 'checking' ? <p role="status" className="text-sm text-slate-600">Verificando acesso aos relatórios…</p> : null}
      {access === 'denied' ? (
        <p role="alert" className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900">
          Sua função não permite consultar relatórios financeiros desta loja. Peça acesso ao responsável.
        </p>
      ) : null}
      {access === 'error' ? (
        <p role="alert" className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">
          Não foi possível verificar suas permissões. Volte ao Início e tente novamente.
        </p>
      ) : null}
      {access !== 'allowed' ? <Link to="/home" className="inline-block text-sm font-semibold text-blue-700 underline">Voltar ao Início</Link> : null}
      {access === 'allowed' ? (
        <>
          <OperationalReports />
          <TopProductsReport />
        </>
      ) : null}
    </div>
  )
}
