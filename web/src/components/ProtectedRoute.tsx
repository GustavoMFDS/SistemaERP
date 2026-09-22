import { useEffect, useState } from 'react'
import { Navigate, Outlet } from 'react-router-dom'
import { refreshAccessToken } from '../lib/api'
import { getToken, subscribeToken } from '../lib/auth'

export default function ProtectedRoute() {
  const [checking, setChecking] = useState(!getToken())
  const [token, setTokenState] = useState(getToken())

  useEffect(() => {
    return subscribeToken((nextToken) => {
      setTokenState(nextToken)
      setChecking(false)
    })
  }, [])

  useEffect(() => {
    if (getToken()) {
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

  if (checking) return null
  if (!token) return <Navigate to="/login" replace />
  return <Outlet />
}
