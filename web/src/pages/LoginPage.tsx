import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { APIError, apiJson } from '../lib/api'
import { setToken } from '../lib/auth'
import PasswordField from '../components/PasswordField'
import { resetFiscalReminderForLogin } from '../lib/fiscalReminderSession'

type LoginResponse = {
  token: { access_token: string; token_type: string; expires_in: number }
  user: { id: string; email: string; name: string; roles: string[] }
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = 'Entrar — Esterium ERP'
  }, [])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (loading) return
    setError('')
    setLoading(true)
    try {
      const data = await apiJson<LoginResponse>('/api/v1/auth/login', {
        method: 'POST',
        body: { email: email.trim(), password },
      })
      setToken(data.token.access_token)
      resetFiscalReminderForLogin()
      navigate('/home', { replace: true })
    } catch (e: unknown) {
      if (e instanceof APIError && e.status === 429) {
        setError('Muitas tentativas de acesso. Aguarde um pouco antes de tentar novamente.')
      } else if (e instanceof APIError && (e.status === 400 || e.status === 401 || e.status === 403)) {
        setError('Não foi possível entrar. Confira seus dados de acesso e tente novamente.')
      } else {
        setError('Não foi possível acessar o sistema agora. Confira sua conexão e tente novamente.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-full bg-gray-50">
      <div className="mx-auto flex min-h-screen max-w-md flex-col justify-center px-4">
        <div className="rounded-lg border bg-white p-6">
          <p className="mb-3 text-xl font-bold tracking-tight text-slate-900"><span aria-hidden="true" className="mr-2 text-blue-600">✦</span>Esterium ERP</p>
          <h1 className="text-lg font-semibold">Bem-vindo de volta</h1>
          <p className="mt-1 text-sm text-gray-600">Entre com seu e-mail ou nome de usuário e sua senha para acessar a loja.</p>

          <form onSubmit={onSubmit} className="mt-4 space-y-3">
            <label className="block">
              <span className="text-sm text-gray-700">E-mail ou nome de usuário</span>
              <input
                value={email}
                onChange={(e) => { setEmail(e.target.value); setError('') }}
                type="text"
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                required
              />
            </label>

            <PasswordField
              id="login-password"
              label="Senha"
              value={password}
              onChange={(value) => { setPassword(value); setError('') }}
              autoComplete="current-password"
              disabled={loading}
            />

            {error ? (
              <div role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
                {error}
              </div>
            ) : null}

            <button
              type="submit"
              disabled={loading || !email.trim() || !password}
              className="w-full rounded-md bg-gray-900 px-3 py-2.5 text-sm font-medium text-white hover:bg-gray-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-700 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {loading ? 'Entrando…' : 'Entrar'}
            </button>

            <p className="text-center text-xs text-gray-500">Acesso pessoal e protegido. Não compartilhe sua senha.</p>
          </form>
        </div>
      </div>
    </div>
  )
}
