import { useEffect, useState } from 'react'
import { Navigate, Outlet } from 'react-router-dom'
import { refreshAccessToken } from '../lib/api'
import { getToken, hasUsableAccessToken, subscribeToken } from '../lib/auth'

export default function ProtectedRoute() {
  const [checking, setChecking] = useState(!hasUsableAccessToken())
  const [token, setTokenState] = useState(hasUsableAccessToken() ? getToken() : '')

  useEffect(() => {
    return subscribeToken((nextToken) => {
      setTokenState(hasUsableAccessToken() ? nextToken : '')
      setChecking(false)
    })
  }, [])

  useEffect(() => {
    if (hasUsableAccessToken()) {
      setTokenState(getToken())
      setChecking(false)
      return
    }

    let active = true
    setChecking(true)
    refreshAccessToken()
      .then((ok) => {
        if (!active) return
        setTokenState(ok ? getToken() : '')
      })
      .finally(() => {
        if (active) setChecking(false)
      })

    return () => {
      active = false
    }
  }, [])

  if (checking) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-gray-50 px-4">
        <div role="status" aria-live="polite" className="w-full max-w-sm rounded-xl border border-slate-200 bg-white p-6 text-center shadow-sm">
          <p className="font-semibold text-slate-900">Esterium ERP</p>
          <p className="mt-2 text-sm text-slate-600">Verificando acesso à loja…</p>
        </div>
      </main>
    )
  }
  if (!token) return <Navigate to="/login" replace />
  return <Outlet />
}
