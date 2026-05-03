import { useEffect, useMemo, useState } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { clearCashSessionId, clearToken } from '../lib/auth'
import { clearOfflineQueue, getQueueCount } from '../lib/offlineQueue'

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
        { to: '/pdv', label: 'PDV' },
        { to: '/finance', label: 'Financeiro' },
        { to: '/fiscal', label: 'Fiscal (XML)' },
      ] as const,
    [],
  )

  function logout() {
    const pending = getQueueCount()
    if (
      pending > 0 &&
      !window.confirm(
        `Existem ${pending} venda(s) offline pendente(s). Sair agora vai limpar essa fila local. Deseja continuar?`,
      )
    ) {
      return
    }
    void apiJson('/api/v1/auth/logout', { method: 'POST' }).catch(() => {
      // Local logout still wins if the network is unavailable.
    })
    clearToken()
    clearCashSessionId()
    clearOfflineQueue()
    navigate('/login', { replace: true })
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
              onClick={logout}
              className="rounded-md border px-2 py-1 text-xs hover:bg-gray-50"
            >
              Sair
            </button>
          </div>
        </div>
      </header>

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
