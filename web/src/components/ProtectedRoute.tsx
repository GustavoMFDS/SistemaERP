import { useEffect, useState } from 'react'
import { Navigate, Outlet } from 'react-router-dom'
import { refreshAccessToken } from '../lib/api'
import { getToken } from '../lib/auth'

export default function ProtectedRoute() {
  const [checking, setChecking] = useState(!getToken())
  const [token, setTokenState] = useState(getToken())

  useEffect(() => {
    if (token) return
    let active = true
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
  }, [token])

  if (checking) return null
  if (!token) return <Navigate to="/login" replace />
  return <Outlet />
}
