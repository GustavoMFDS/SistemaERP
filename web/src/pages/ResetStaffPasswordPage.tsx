import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { APIError, apiJson } from '../lib/api'
import PasswordField from '../components/PasswordField'
import { validStaffPassword } from '../lib/staffPassword'

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
  const passwordTooShort = password.length > 0 && Array.from(password).length < 12
  const passwordTooLong = password.length > 0 && !passwordTooShort && !validStaffPassword(password)
  const passwordHelp = passwordTooShort
    ? 'Use pelo menos 12 caracteres para sua senha.'
    : passwordTooLong
      ? 'Sua senha está longa demais. Escolha uma um pouco mais curta.'
      : 'Use pelo menos 12 caracteres e evite reutilizar senhas de outros serviços.'

  useEffect(() => {
    const previousTitle = document.title
    document.title = 'Recuperar acesso — Esterium ERP'
    return () => { initialSecret = ''; document.title = previousTitle }
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!token || busy || password !== confirm || !validStaffPassword(password)) return
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
      if (cause instanceof APIError && cause.status === 429) {
        setError('Muitas tentativas. Aguarde um pouco antes de tentar novamente.')
      } else if (cause instanceof APIError && [400, 401, 403, 404, 409, 410].includes(cause.status)) {
        setError('Não foi possível confirmar este link. Peça outro ao responsável pela loja.')
      } else {
        setError('Não foi possível alterar a senha agora. Confira sua conexão e tente novamente.')
      }
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
              <PasswordField
                id="reset-password"
                label="Nova senha"
                value={password}
                onChange={(value) => { setPassword(value); setError('') }}
                autoComplete="new-password"
                minLength={12}
                maxLength={72}
                disabled={busy}
                describedBy="reset-password-help"
              />
              <p id="reset-password-help" aria-live="polite" className="text-xs text-slate-600">{passwordHelp}</p>
              <PasswordField
                id="reset-confirm-password"
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
              {error ? (
                <p role="alert" className="text-sm text-red-700">{error}</p>
              ) : null}
              <button disabled={busy || !password || password !== confirm || !validStaffPassword(password)}
                className="w-full rounded-md bg-gray-900 px-4 py-2.5 text-white hover:bg-gray-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-700 disabled:cursor-not-allowed disabled:opacity-50">
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
