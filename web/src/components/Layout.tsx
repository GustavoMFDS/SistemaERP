import { useEffect, useMemo, useState } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import { APIError, apiJson, errorMessage } from '../lib/api'
import {
  clearAllCashSessionsForCurrentUser,
  clearAllUserScopedStorage,
  clearToken,
  getAllCashSessionIdsForCurrentUser,
  getCashSessionId,
  setToken,
} from '../lib/auth'
import {
  clearAllOfflineQueuesForCurrentUser,
  getAllUserQueueCount,
  getLegacyQueueCount,
  getQueueCount,
} from '../lib/offlineQueue'

const PRODUCTS_CACHE_NAMESPACE = 'sistemaemgo:productsCache:v2'

type MeResponse = {
  id: string
  email: string
  name: string
  tenant_id: string
  roles: string[]
}

type TenantInfo = {
  id: string
  legal_name: string
  trade_name?: string | null
}

type TenantListResponse = {
  items: TenantInfo[]
  current_tenant_id: string
}

type SwitchTenantResponse = {
  token: { access_token: string; token_type: string; expires_in: number }
  user: MeResponse
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
  const [tenants, setTenants] = useState<TenantInfo[]>([])
  const [tenantError, setTenantError] = useState('')
  const [switchingTenant, setSwitchingTenant] = useState(false)

  useEffect(() => {
    let cancelled = false
    Promise.all([
      apiJson<MeResponse>('/api/v1/auth/me'),
      apiJson<TenantListResponse>('/api/v1/auth/tenants'),
    ])
      .then(([meData, tenantData]) => {
        if (cancelled) return
        setMe(meData)
        setTenants(tenantData.items)
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

  async function switchTenant(targetTenantID: string) {
    if (!me || targetTenantID === me.tenant_id || switchingTenant) return

    const pending = getQueueCount()
    const cashSessionID = getCashSessionId()
    if (
      (pending > 0 || cashSessionID) &&
      !window.confirm(
        [
          'Você está trocando de loja/CNPJ.',
          pending > 0
            ? `Existem ${pending} venda(s) offline pendente(s) nesta loja; elas permanecerão salvas e voltarão a aparecer quando você retornar.`
            : '',
          cashSessionID
            ? 'Existe uma sessão de caixa local vinculada a esta loja; ela permanecerá isolada neste CNPJ.'
            : '',
          'Deseja continuar?',
        ]
          .filter(Boolean)
          .join('\n\n'),
      )
    ) {
      return
    }

    setTenantError('')
    setSwitchingTenant(true)
    try {
      const data = await apiJson<SwitchTenantResponse>(
        '/api/v1/auth/switch-tenant',
        {
          method: 'POST',
          body: { tenant_id: targetTenantID },
        },
      )
      setToken(data.token.access_token)
      setMe(data.user)

      // A full reload deliberately discards in-memory page state from the old
      // tenant. The new HttpOnly refresh cookie restores the selected tenant,
      // while cash/offline/product caches remain namespaced by tenant+user.
      window.location.replace('/products')
    } catch (e: unknown) {
      if (e instanceof APIError && e.status === 409) {
        // Another tab may already have rotated the shared refresh cookie to a
        // different tenant. Reload drops the stale in-memory access token and
        // lets ProtectedRoute restore the current tenant from that cookie.
        window.location.reload()
        return
      }
      if (e instanceof APIError && e.status === 401) {
        clearToken()
        navigate('/login', { replace: true })
        return
      }
      setTenantError(errorMessage(e))
      setSwitchingTenant(false)
    }
  }

  async function logout() {
    if (loggingOut) return

    const openCashSessions = getAllCashSessionIdsForCurrentUser()
    if (openCashSessions.length > 0) {
      setLogoutError(
        `Existem ${openCashSessions.length} caixa(s) local(is) ainda aberto(s) em uma ou mais lojas. Troque para cada loja e feche o caixa antes de sair.`,
      )
      return
    }

    const legacyPending = getLegacyQueueCount()
    if (legacyPending > 0) {
      setLogoutError(
        `Existem ${legacyPending} venda(s) na fila offline legada sem escopo de loja confiável. Abra o PDV e importe para revisão ou descarte explicitamente antes de sair.`,
      )
      return
    }

    const pending = getAllUserQueueCount()
    if (
      pending > 0 &&
      !window.confirm(
        `Existem ${pending} venda(s) offline pendente(s) somando todas as lojas deste usuário. Sair agora vai limpar essas filas locais. Deseja continuar?`,
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

      // Logout is account-wide on this browser. Switching tenants preserves
      // scoped state, but signing out removes local data from every CNPJ that
      // belongs to the current user so another person cannot inherit it.
      clearAllCashSessionsForCurrentUser()
      clearAllOfflineQueuesForCurrentUser()
      clearAllUserScopedStorage(PRODUCTS_CACHE_NAMESPACE)
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
            {tenants.length > 1 && me ? (
              <label className="flex items-center gap-2 text-xs text-gray-600">
                <span>Loja</span>
                <select
                  value={me.tenant_id}
                  onChange={(e) => void switchTenant(e.target.value)}
                  disabled={switchingTenant}
                  className="max-w-56 rounded-md border bg-white px-2 py-1 text-xs disabled:opacity-60"
                  aria-label="Loja ativa"
                >
                  {tenants.map((tenant) => (
                    <option key={tenant.id} value={tenant.id}>
                      {tenant.trade_name || tenant.legal_name}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
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
      {tenantError ? (
        <div className="mx-auto mt-3 max-w-6xl rounded-md border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700">
          Não foi possível trocar de loja. {tenantError}
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
