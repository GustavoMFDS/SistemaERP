import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { setToken } from '../lib/auth'

type LoginResponse = {
  token: { access_token: string; token_type: string; expires_in: number }
  user: { id: string; email: string; name: string; roles: string[] }
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [email, setEmail] = useState(import.meta.env.DEV ? 'admin@sistema.local' : '')
  const [password, setPassword] = useState(import.meta.env.DEV ? 'admin123' : '')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const data = await apiJson<LoginResponse>('/api/v1/auth/login', {
        method: 'POST',
        body: { email, password },
      })
      setToken(data.token.access_token)
      navigate('/products', { replace: true })
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-full bg-gray-50">
      <div className="mx-auto flex min-h-screen max-w-md flex-col justify-center px-4">
        <div className="rounded-lg border bg-white p-6">
          <h1 className="text-lg font-semibold">Entrar</h1>
          <p className="mt-1 text-sm text-gray-600">Use seu e-mail e senha.</p>

          <form onSubmit={onSubmit} className="mt-4 space-y-3">
            <label className="block">
              <span className="text-sm text-gray-700">E-mail</span>
              <input
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                type="email"
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                autoComplete="email"
                required
              />
            </label>

            <label className="block">
              <span className="text-sm text-gray-700">Senha</span>
              <input
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                type="password"
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                autoComplete="current-password"
                required
              />
            </label>

            {error ? (
              <div className="rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
                {error}
              </div>
            ) : null}

            <button
              disabled={loading}
              className="w-full rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              {loading ? 'Entrando…' : 'Entrar'}
            </button>

            <div className="text-xs text-gray-500">
              API base: {import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'}
              {import.meta.env.DEV ? ' • Demo: admin@sistema.local / admin123' : ''}
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
