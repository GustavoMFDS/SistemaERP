import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
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
      .catch((e: unknown) => {
        if (!cancelled) setMeError(errorMessage(e))
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
    { to: '/staff', label: 'Funcionários', section: 'Administração', permission: 'team:manage' },
    { to: '/fiscal', label: 'Fiscal (XML)', section: 'Administração', permission: 'invoice:read' },
    { to: '/setup', label: 'Configurar loja', section: 'Administração', permission: null },
  ].filter((item) => item.permission === null ||
    (item.permission === 'imports:history'
      ? permissionSet.has('product:write') || permissionSet.has('inventory:adjust')
      : permissionSet.has(item.permission)))
  const navSections = ['Principal', 'Vendas e clientes', 'Produtos e estoque', 'Administração']

  async function logout() {
    if (loggingOut) return

    const legacyPending = getLegacyQueueCount()
    if (legacyPending > 0) {
      setLogoutError(
        `Não é possível sair enquanto existirem ${legacyPending} item(ns) na fila offline legada ainda não revisada. Abra o PDV e importe a fila para atenção ou descarte-a explicitamente antes de encerrar a sessão.`,
      )
      return
    }

    const pending = getQueueCount()
    if (pending > 0) {
      setLogoutError(
        `Não é possível sair enquanto existirem ${pending} venda(s) offline preservada(s). Sincronize, reconcilie ou descarte cada item explicitamente no PDV antes de encerrar a sessão.`,
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
    } catch (e: unknown) {
      setLogoutError(
        `Não foi possível encerrar a sessão no servidor. Verifique a conexão e tente novamente. ${errorMessage(e)}`,
      )
    } finally {
      setLoggingOut(false)
    }
  }

  return (
    <div className="min-h-full">
      <header className="border-b bg-white">
        <div className="mx-auto flex max-w-[1680px] flex-wrap items-center justify-between gap-3 px-4 py-3 lg:px-6">
          <Link to="/home" className="text-sm font-semibold">
            SistemaEmGo
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
              {me?.email ? (
                <span>
                  {me.name} - {me.email}
                </span>
              ) : meError ? (
                <span className="text-red-600">Falha ao carregar usuario</span>
              ) : (
                <span>Carregando...</span>
              )}
            </div>
            <button
              onClick={() => void logout()}
              disabled={loggingOut}
              className="rounded-md border px-2 py-1 text-xs hover:bg-gray-50 disabled:opacity-60"
            >
              {loggingOut ? 'Saindo…' : 'Sair'}
            </button>
          </div>
        </div>
      </header>

      {logoutError ? (
        <div className="mx-auto mt-3 max-w-6xl rounded-md border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
          {logoutError}
        </div>
      ) : null}

      <div className={classNames(
        'mx-auto grid max-w-[1680px] grid-cols-1 items-start gap-5 px-4 py-5 lg:grid-cols-[210px_minmax(0,1fr)] lg:gap-6 lg:px-6',
      )}>
        <aside className="min-w-0 lg:sticky lg:top-5">
          <nav aria-label="Navegação principal"
            className="flex gap-1 overflow-x-auto rounded-xl border border-slate-200 bg-white p-2 shadow-sm lg:block lg:space-y-4 lg:overflow-visible lg:p-3">
            {navSections.map((section) => {
              const links = nav.filter((item) => item.section === section)
              if (!links.length) return null
              return (
                <div key={section} className="flex shrink-0 gap-1 lg:block">
                  <p className="hidden px-3 pb-1 text-[11px] font-bold uppercase tracking-wider text-slate-500 lg:block">
                    {section}
                  </p>
                  {links.map((item) => (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      className={({ isActive }) =>
                        classNames(
                          'block shrink-0 whitespace-nowrap rounded-lg px-3 py-2.5 text-sm transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-slate-700 lg:my-0.5',
                          isActive
                            ? 'bg-slate-900 font-semibold text-white'
                            : 'text-slate-700 hover:bg-slate-100',
                        )
                      }
                    >
                      {item.label}
                    </NavLink>
                  ))}
                </div>
              )
            })}
          </nav>
          <p className="mt-2 px-2 text-xs text-slate-500 lg:hidden">
            Deslize o menu para ver mais áreas do sistema.
          </p>
        </aside>
        <main className="min-w-0">
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