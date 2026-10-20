import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { apiJson } from '../lib/api'
import { clearCashSessionId, clearScopedStorage, clearToken } from '../lib/auth'
import { getLegacyQueueCount, getQueueCount } from '../lib/offlineQueue'
import { useInstallApp } from '../lib/installApp'

const PRODUCTS_CACHE_NAMESPACE = 'sistemaemgo:productsCache:v2'

type MeResponse = {
  id: string
  email: string
  name: string
  roles: string[]
  permissions: string[]
}

function classNames(...xs: Array<string | false | undefined>): string {
  return xs.filter(Boolean).join(' ')
}

export default function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const isPDV = location.pathname === '/pdv'
  const appInstall = useInstallApp()
  const [me, setMe] = useState<MeResponse | null>(null)
  const [meError, setMeError] = useState<string>('')
  const [logoutError, setLogoutError] = useState('')
  const [loggingOut, setLoggingOut] = useState(false)

  useEffect(() => {
    let cancelled = false
    apiJson<MeResponse>('/api/v1/auth/me')
      .then((data) => {
        if (!cancelled) setMe(data)
      })
      .catch(() => {
        if (!cancelled) setMeError('Não foi possível carregar seus acessos. Atualize a página para tentar novamente.')
      })
    return () => {
      cancelled = true
    }
  }, [])

  const permissionSet = new Set(me?.permissions ?? [])
  const nav = [
    { to: '/home', label: 'Início', section: 'Principal', permission: null },
    { to: '/pdv', label: 'PDV', section: 'Vendas e clientes', permission: 'sale:write' },
    { to: '/customers', label: 'Clientes', section: 'Vendas e clientes', permission: 'customer:read' },
    { to: '/returns', label: 'Devoluções/Trocas', section: 'Vendas e clientes', permission: 'sale:return' },
    { to: '/products', label: 'Produtos', section: 'Produtos e estoque', permission: 'product:read' },
    { to: '/inventory', label: 'Estoque', section: 'Produtos e estoque', permission: 'inventory:read' },
    { to: '/stock-movements', label: 'Movimentações', section: 'Produtos e estoque', permission: 'inventory:read' },
    { to: '/imports', label: 'Histórico de importações', section: 'Produtos e estoque', permission: 'imports:history' },
    { to: '/purchases', label: 'Compras', section: 'Produtos e estoque', permission: 'procurement:read' },
    { to: '/finance', label: 'Financeiro', section: 'Administração', permission: 'finance:read' },
    { to: '/reports', label: 'Relatórios', section: 'Administração', permission: 'finance:read' },
    { to: '/staff', label: 'Funcionários', section: 'Administração', permission: 'team:manage' },
    { to: '/fiscal', label: 'Fiscal', section: 'Administração', permission: 'invoice:read' },
    { to: '/setup', label: 'Configurar loja', section: 'Administração', permission: null },
  ].filter((item) => item.permission === null ||
    (item.permission === 'imports:history'
      ? permissionSet.has('product:write') || permissionSet.has('inventory:adjust')
      : permissionSet.has(item.permission)))
  const mainPaths = new Set(['/home', '/pdv', '/products', '/inventory'])
  const mainLinks = nav.filter((item) => mainPaths.has(item.to))
  const extraLinks = nav.filter((item) => !mainPaths.has(item.to))

  async function logout() {
    if (loggingOut) return

    const legacyPending = getLegacyQueueCount()
    if (legacyPending > 0) {
      setLogoutError(
        `Há ${legacyPending} registro(s) antigo(s) neste dispositivo que precisam de atenção. Abra o PDV e escolha “Importar para revisão” ou descarte esses registros antes de sair.`,
      )
      return
    }

    const pending = getQueueCount()
    if (pending > 0) {
      setLogoutError(
        `Há ${pending} venda(s) guardada(s) neste dispositivo. Abra o PDV para enviar, conferir ou descartar cada venda antes de sair.`,
      )
      return
    }

    setLogoutError('')
    setLoggingOut(true)
    try {
      // The refresh token is HttpOnly, so the browser cannot safely complete
      // logout locally. Only clear local state after the server confirms the
      // revocation and sends the cookie expiration response.
      await apiJson('/api/v1/auth/logout', { method: 'POST' })

      // Clear scoped state while the current access token still identifies
      // the tenant/user namespace.
      clearCashSessionId()
      clearScopedStorage(PRODUCTS_CACHE_NAMESPACE)
      clearToken()
      navigate('/login', { replace: true })
    } catch {
      setLogoutError('Não foi possível confirmar a saída com segurança. Confira sua conexão e tente novamente.')
    } finally {
      setLoggingOut(false)
    }
  }

  return (
    <div className="min-h-full">
      <a
        href="#conteudo-principal"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-white focus:px-4 focus:py-2 focus:text-sm focus:font-semibold focus:shadow-lg"
      >
        Pular para o conteúdo
      </a>
      <header className="border-b bg-white">
        <div className="mx-auto flex max-w-[1680px] flex-wrap items-center justify-between gap-3 px-4 py-3 lg:px-6">
          <Link to="/home" className="text-sm font-semibold">
            <span aria-hidden="true" className="mr-1.5 text-blue-600">✦</span>
            Esterium ERP
          </Link>
          <div className="flex flex-wrap items-center gap-2 sm:gap-3">
            {appInstall.available ? (
              <button type="button" onClick={() => void appInstall.install()}
                className="rounded-md border px-2 py-1 text-xs hover:bg-gray-50">
                Instalar aplicativo
              </button>
            ) : null}
            <details className="relative text-xs">
              <summary className="cursor-pointer rounded-md border px-2 py-1 hover:bg-gray-50">
                Usar como app
              </summary>
              <div className="absolute right-0 z-20 mt-2 w-64 rounded-md border bg-white p-3 text-gray-700 shadow-lg">
                <p className="font-semibold">Instalar no seu dispositivo</p>
                <p className="mt-1">Android/Chrome: menu ⋮ → Instalar aplicativo ou Adicionar à tela inicial.</p>
                <p className="mt-1">iPhone/Safari: Compartilhar → Adicionar à Tela de Início.</p>
                <p className="mt-1">Computador: ícone de instalação na barra do navegador, quando disponível.</p>
                <p className="mt-2 text-amber-800">Para usar a loja, o servidor precisa estar configurado e acessível. Não desinstale ou limpe dados com vendas offline pendentes.</p>
              </div>
            </details>
            <div className="hidden text-xs text-gray-600 sm:block">
              {me ? (
                <span>{me.name}{me.email ? ' · ' + me.email : ''}</span>
              ) : meError ? (
                <span className="text-red-600">Acessos indisponíveis</span>
              ) : (
                <span>Carregando...</span>
              )}
            </div>
            <button
              type="button"
              onClick={() => void logout()}
              disabled={loggingOut}
              className="rounded-md border px-2 py-1 text-xs hover:bg-gray-50 disabled:opacity-60"
            >
              {loggingOut ? 'Saindo…' : 'Sair'}
            </button>
          </div>
        </div>
      </header>

      {meError ? (
        <div role="alert" className="mx-auto mt-3 max-w-6xl rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          {meError}
        </div>
      ) : null}
      {logoutError ? (
        <div role="alert" className="mx-auto mt-3 max-w-6xl rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {logoutError}
        </div>
      ) : null}

      <div className={classNames(
        'mx-auto grid max-w-[1680px] grid-cols-1 items-start gap-5 px-4 py-5 lg:grid-cols-[210px_minmax(0,1fr)] lg:gap-6 lg:px-6',
      )}>
        <aside className="min-w-0 lg:sticky lg:top-5">
          <nav aria-label="Navegação principal" aria-busy={!me && !meError} className="rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
            <p className="px-2 pb-2 text-xs font-bold text-slate-500">Acessos principais</p>
            <div className="grid grid-cols-2 gap-1 sm:grid-cols-4 lg:grid-cols-1">
              {mainLinks.map((item) => (
                <NavLink key={item.to} to={item.to}
                  className={({ isActive }) => classNames(
                    'block rounded-lg px-3 py-2.5 text-center text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-700 lg:text-left',
                    isActive ? 'bg-slate-900 font-semibold text-white' : 'text-slate-700 hover:bg-slate-100',
                  )}
                >
                  {item.label}
                </NavLink>
              ))}
            </div>
            {extraLinks.length > 0 ? (
              <details key={location.pathname} open={extraLinks.some((item) => item.to === location.pathname)} className="mt-3 border-t border-slate-200 pt-3">
                <summary className="cursor-pointer rounded-lg px-3 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-slate-700">
                  Mais áreas do sistema
                </summary>
                <div className="mt-2 grid grid-cols-2 gap-1 sm:grid-cols-3 lg:grid-cols-1">
                  {extraLinks.map((item) => (
                    <NavLink key={item.to} to={item.to}
                      className={({ isActive }) => classNames(
                        'block rounded-lg px-3 py-2 text-sm transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-700',
                        isActive ? 'bg-slate-900 font-semibold text-white' : 'text-slate-700 hover:bg-slate-100',
                      )}
                    >
                      {item.label}
                    </NavLink>
                  ))}
                </div>
              </details>
            ) : null}
          </nav>
        </aside>
        <main id="conteudo-principal" tabIndex={-1} className="min-w-0">
          <div className={isPDV
            ? 'min-w-0'
            : 'min-w-0 rounded-xl border border-slate-200 bg-white p-4 shadow-sm sm:p-6'}>
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}