import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'

// The secret is never stored in the address bar, cookies, or localStorage.
// Keep it in memory for React StrictMode's double initializer only.
let initialSecret = ''
function takeSecret(): string {
  const token = new URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
  if (token) initialSecret = token
  if (window.location.hash) window.history.replaceState(null, '', window.location.pathname)
  return initialSecret
}

export default function ResetStaffPasswordPage() {
  const [token] = useState(takeSecret)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [completed, setCompleted] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = 'Recuperar acesso — Esterium ERP'
    return () => { initialSecret = '' }
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!token || busy || password !== confirm || password.length < 12 || password.length > 72) return
    setBusy(true)
    setError('')
    try {
      await apiJson('/api/v1/staff/complete-password-reset', {
        method: 'POST', body: { token, password },
      })
      setCompleted(true)
      setPassword('')
      setConfirm('')
    } catch (cause: unknown) {
      setError(errorMessage(cause))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="mx-auto flex min-h-screen max-w-md flex-col justify-center px-4">
        <div className="rounded-xl border bg-white p-6">
          <p className="mb-3 text-xl font-bold tracking-tight text-slate-900"><span aria-hidden="true" className="mr-2 text-blue-600">✦</span>Esterium ERP</p>
          <h1 className="text-lg font-semibold">Recuperar meu acesso</h1>
          <p className="mt-2 text-sm text-gray-600">
            Defina uma nova senha pessoal. O responsável pela loja não pode visualizá-la.
          </p>
          {completed ? (
            <div role="status" className="mt-4 rounded-md border p-3 text-sm">
              Senha alterada. As sessões anteriores foram invalidadas.
              <Link to="/login" className="mt-3 block text-blue-700 underline">Ir para o login</Link>
            </div>
          ) : !token ? (
            <p role="alert" className="mt-4 text-sm text-red-700">
              Link ausente ou inválido. Peça outro ao responsável pela loja.
            </p>
          ) : (
            <form onSubmit={(event) => void submit(event)} className="mt-4 space-y-3">
              <label className="block text-sm">Nova senha (12 a 72 caracteres)
                <input type="password" required minLength={12} maxLength={72}
                  autoComplete="new-password" value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="mt-1 block w-full rounded-md border px-3 py-2" />
              </label>
              <label className="block text-sm">Confirmar nova senha
                <input type="password" required minLength={12} maxLength={72}
                  autoComplete="new-password" value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  className="mt-1 block w-full rounded-md border px-3 py-2" />
              </label>
              {password && confirm && password !== confirm ? (
                <p role="alert" className="text-xs text-red-700">As senhas devem ser iguais.</p>
              ) : null}
              {error ? (
                <p role="alert" className="text-sm text-red-700">Não foi possível alterar a senha: {error}.</p>
              ) : null}
              <button disabled={busy || !password || password !== confirm || password.length < 12}
                className="w-full rounded-md bg-gray-900 px-4 py-2 text-white disabled:opacity-50">
                {busy ? 'Alterando…' : 'Alterar minha senha'}
              </button>
            </form>
          )}
          {!completed ? <Link to="/login" className="mt-4 block text-xs text-blue-700 underline">Voltar ao login</Link> : null}
        </div>
      </div>
    </div>
  )
}
