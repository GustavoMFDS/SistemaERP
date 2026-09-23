import { useEffect, useMemo, useState } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { clearCashSessionId, clearScopedStorage, clearToken } from '../lib/auth'
import { clearOfflineQueue, getQueueCount } from '../lib/offlineQueue'

const PRODUCTS_CACHE_NAMESPACE = 'sistemaemgo:productsCache:v2'

type MeResponse = {
  id: string
  email: string
  name: string
  roles: string[]
}

function classNames(...xs: Array<string | false | undefined>): string {
  return xs.filter(Boolean).join(' ')
}

export default function Layout() {
  const navigate = useNavigate()
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

  const nav = useMemo(
    () =>
      [
        { to: '/products', label: 'Produtos' },
        { to: '/inventory', label: 'Estoque' },
        { to: '/purchases', label: 'Compras' },
        { to: '/returns', label: 'Devoluções/Trocas' },
        { to: '/pdv', label: 'PDV' },
        { to: '/finance', label: 'Financeiro' },
        { to: '/fiscal', label: 'Fiscal (XML)' },
      ] as const,
    [],
  )

  async function logout() {
    if (loggingOut) return

    const pending = getQueueCount()
    if (
      pending > 0 &&
      !window.confirm(
        `Existem ${pending} venda(s) offline pendente(s). Sair agora vai limpar essa fila local. Deseja continuar?`,
      )
    ) {
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
      clearOfflineQueue()
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
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-3">
          <Link to="/products" className="text-sm font-semibold">
            SistemaEmGo
          </Link>
          <div className="flex items-center gap-3">
            <div className="text-xs text-gray-600">
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
          <nav className="rounded-lg border bg-white p-2">
            {nav.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  classNames(
                    'block rounded-md px-3 py-2 text-sm',
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
