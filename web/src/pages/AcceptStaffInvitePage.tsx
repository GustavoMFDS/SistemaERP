import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { APIError, apiJson } from '../lib/api'
import PasswordField from '../components/PasswordField'
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
  const passwordTooShort = password.length > 0 && Array.from(password).length < 12
  const passwordTooLong = password.length > 0 && !passwordTooShort && !validStaffPassword(password)
  const passwordHelp = passwordTooShort
    ? 'Use pelo menos 12 caracteres para sua senha.'
    : passwordTooLong
      ? 'Sua senha está longa demais. Escolha uma um pouco mais curta.'
      : 'Use pelo menos 12 caracteres e evite reutilizar senhas de outros serviços.'

  useEffect(() => {
    const previousTitle = document.title
    document.title = 'Ativar acesso — Esterium ERP'
    return () => { initialInviteToken = ''; document.title = previousTitle }
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
      if (cause instanceof APIError && cause.status === 429) {
        setError('Muitas tentativas. Aguarde um pouco antes de tentar novamente.')
      } else if (cause instanceof APIError && [400, 401, 403, 404, 409, 410].includes(cause.status)) {
        setError('Este link não pode mais ser usado. Peça outro ao responsável pela loja.')
      } else {
        setError('Não foi possível ativar o acesso agora. Confira sua conexão e tente novamente.')
      }
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
              <PasswordField
                id="invite-password"
                label="Criar senha"
                value={password}
                onChange={(value) => { setPassword(value); setError('') }}
                autoComplete="new-password"
                minLength={12}
                maxLength={72}
                disabled={busy}
                describedBy="invite-password-help"
              />
              <p id="invite-password-help" aria-live="polite" className="text-xs text-slate-600">{passwordHelp}</p>
              <PasswordField
                id="invite-confirm-password"
                label="Confirmar nova senha"
                value={confirm}
                onChange={(value) => { setConfirm(value); setError('') }}
                autoComplete="new-password"
                minLength={12}
                maxLength={72}
                disabled={busy}
              />
              {password && confirm && password !== confirm ? (
                <p role="alert" className="text-xs text-red-700">As senhas não são iguais. Confira a confirmação.</p>
              ) : null}
              {error ? <p role="alert" className="text-sm text-red-700">{error}</p> : null}
              <button type="submit" disabled={busy || !password || password !== confirm || !validStaffPassword(password)}
                className="w-full rounded-md bg-gray-900 px-4 py-2.5 text-white hover:bg-gray-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-700 disabled:cursor-not-allowed disabled:opacity-50">
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
