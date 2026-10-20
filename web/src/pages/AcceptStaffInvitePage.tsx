import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { validStaffPassword } from '../lib/staffPassword'

// React StrictMode can call useState initializers twice during development.
// Preserve the fragment value across that double render without writing the
// credential to localStorage, cookies, logs or the browser's URL history.
let initialInviteToken = ''
function takeInviteToken(): string {
  const hash = new URLSearchParams(window.location.hash.slice(1))
  const token = hash.get('token') ?? ''
  if (token) initialInviteToken = token
  if (window.location.hash) {
    window.history.replaceState(null, '', window.location.pathname)
  }
  return initialInviteToken
}

export default function AcceptStaffInvitePage() {
  const [token] = useState(takeInviteToken)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [completed, setCompleted] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    document.title = 'Ativar acesso — Esterium ERP'
    return () => { initialInviteToken = '' }
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!token || busy || password !== confirm || !validStaffPassword(password)) return
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
          <p className="mb-3 text-xl font-bold tracking-tight text-slate-900"><span aria-hidden="true" className="mr-2 text-blue-600">✦</span>Esterium ERP</p>
          <h1 className="text-lg font-semibold">Ativar acesso à loja</h1>
          <p className="mt-2 text-sm text-gray-600">
            Seu acesso já foi preparado pelo responsável da loja.
            Crie uma senha pessoal para começar. Não é necessário receber o link por e-mail.
          </p>
          {completed ? (
            <div role="status" className="mt-4 rounded-md border p-3 text-sm">
              Conta ativada. Entre com seu e-mail ou nome de usuário e sua nova senha.
              <Link to="/login" className="mt-3 block text-blue-700 underline">Ir para o login</Link>
            </div>
          ) : !token ? (
            <p role="alert" className="mt-4 text-sm text-red-700">
              Este link está ausente ou inválido. Peça um novo ao responsável pela loja.
            </p>
          ) : (
            <form onSubmit={(event) => void submit(event)} className="mt-4 space-y-3">
              <label className="block text-sm">Criar senha (12 a 72 caracteres)
                <input type="password" required minLength={12} maxLength={72}
                  autoComplete="new-password" value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="mt-1 block w-full rounded-md border px-3 py-2" />
              </label>
              <p className="text-xs text-slate-600">Use ao menos 12 caracteres. O limite técnico é de 72 bytes UTF-8; acentos e emojis podem ocupar mais de um byte.</p>
              <label className="block text-sm">Confirmar nova senha
                <input type="password" required minLength={12} maxLength={72}
                  autoComplete="new-password" value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  className="mt-1 block w-full rounded-md border px-3 py-2" />
              </label>
              {password && confirm && password !== confirm ? (
                <p role="alert" className="text-xs text-red-700">As senhas devem ser iguais.</p>
              ) : null}
              {error ? <p role="alert" className="text-sm text-red-700">Não foi possível ativar o acesso: {error}. Se o link venceu ou foi utilizado, peça outro ao responsável pela loja.</p> : null}
              <button type="submit" disabled={busy || !password || password !== confirm || !validStaffPassword(password)}
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
