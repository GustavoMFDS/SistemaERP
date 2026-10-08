import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
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
    { to: '/home', label: 'Início', permission: null },
    { to: '/setup', label: 'Configurar loja', permission: null },
    { to: '/products', label: 'Produtos', permission: 'product:read' },
    { to: '/inventory', label: 'Estoque', permission: 'inventory:read' },
    { to: '/purchases', label: 'Compras', permission: 'procurement:read' },
    { to: '/returns', label: 'Devoluções/Trocas', permission: 'sale:return' },
    { to: '/pdv', label: 'PDV', permission: 'sale:write' },
    { to: '/finance', label: 'Financeiro', permission: 'finance:read' },
    { to: '/fiscal', label: 'Fiscal (XML)', permission: 'invoice:read' },
  ].filter((item) => item.permission === null || permissionSet.has(item.permission))

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
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-2 px-4 py-3">
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

      <div className="mx-auto grid max-w-6xl grid-cols-12 gap-4 px-4 py-4">
        <aside className="col-span-12 md:col-span-3">
          <nav className="flex gap-1 overflow-x-auto rounded-lg border bg-white p-2 md:block">
            {nav.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  classNames(
                    'block shrink-0 whitespace-nowrap rounded-md px-3 py-2 text-sm',
                    isActive ? 'bg-gray-100 font-medium' : 'hover:bg-gray-50',
                  )
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>
        </aside>
        <main className="col-span-12 md:col-span-9">
          <div className="rounded-lg border bg-white p-4">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
