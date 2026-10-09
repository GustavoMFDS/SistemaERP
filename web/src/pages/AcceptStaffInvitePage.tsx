import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'

// Invite token is contained in the URL fragment, never the server request URL.
// Immediately remove the fragment from browser history after parsing it.
function takeInviteToken(): string {
  const hash = new URLSearchParams(window.location.hash.slice(1))
  const token = hash.get('token') ?? ''
  if (window.location.hash) {
    window.history.replaceState(null, '', window.location.pathname)
  }
  return token
}

export default function AcceptStaffInvitePage() {
  const [token] = useState(takeInviteToken)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [completed, setCompleted] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = 'Ativar conta — SistemaEmGo'
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!token || busy || password !== confirm || password.length < 12 || password.length > 72) return
    setBusy(true)
    setError('')
    try {
      await apiJson('/api/v1/staff/accept-invite', {
        method: 'POST',
        body: { token, password },
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
        <div className="rounded-lg border bg-white p-6">
          <h1 className="text-lg font-semibold">Ativar acesso à loja</h1>
          <p className="mt-2 text-sm text-gray-600">
            Defina uma senha pessoal. O vínculo com a empresa e a função
            foram escolhidos pelo administrador que enviou o convite.
          </p>
          {completed ? (
            <div role="status" className="mt-4 rounded-md border p-3 text-sm">
              Conta ativada. Agora você pode entrar com seu e-mail e sua nova senha.
              <Link to="/login" className="mt-3 block text-blue-700 underline">Ir para o login</Link>
            </div>
          ) : !token ? (
            <p role="alert" className="mt-4 text-sm text-red-700">
              Não há um link de convite válido nesta página. Peça um novo ao responsável pela loja.
            </p>
          ) : (
            <form onSubmit={(event) => void submit(event)} className="mt-4 space-y-3">
              <label className="block text-sm">Criar senha (12 a 72 caracteres)
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
              {error ? <p role="alert" className="text-sm text-red-700">{error}</p> : null}
              <button type="submit" disabled={busy || !password || password !== confirm || password.length < 12}
                className="w-full rounded-md bg-gray-900 px-4 py-2 text-white disabled:opacity-50">
                {busy ? 'Ativando…' : 'Ativar minha conta'}
              </button>
            </form>
          )}
          {!completed ? <Link to="/login" className="mt-4 block text-xs text-blue-700 underline">Voltar ao login</Link> : null}
        </div>
      </div>
    </div>
  )
}
