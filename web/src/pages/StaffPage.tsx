import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Member = {
  id: string
  name: string
  email: string
  role: string
  active: boolean
}
type Invitation = {
  id: string
  name: string
  email: string
  role: string
  expires_at: string
}
type TeamResponse = { members: Member[]; invitations: Invitation[] }
type InviteResponse = { invitation: Invitation; token: string }

const roleName = (role: string) =>
  role === 'admin' ? 'Administrador' : role === 'manager' ? 'Gerente' : 'Caixa'

export default function StaffPage() {
  const [permissions, setPermissions] = useState<string[] | null>(null)
  const [currentUser, setCurrentUser] = useState('')
  const [members, setMembers] = useState<Member[]>([])
  const [invitations, setInvitations] = useState<Invitation[]>([])
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'manager' | 'cashier'>('cashier')
  const [inviteLink, setInviteLink] = useState('')
  const [loading, setLoading] = useState(false)
  const [working, setWorking] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const scope = getSessionScope()
  const seq = useRef(0)
  const canManage = permissions?.includes('team:manage') ?? false

  useEffect(() => {
    const seqID = ++seq.current
    const currentScope = getSessionScope()
    setPermissions(null)
    setCurrentUser('')
    setInviteLink('')
    setMembers([])
    setInvitations([])
    void apiJson<{ id: string; permissions: string[] }>('/api/v1/auth/me')
      .then((data) => {
        if (seqID !== seq.current || currentScope !== getSessionScope()) return
        setPermissions(data.permissions)
        setCurrentUser(data.id)
      }).catch((cause: unknown) => {
        if (seqID !== seq.current || currentScope !== getSessionScope()) return
        setError(errorMessage(cause))
      })
    return () => { ++seq.current }
  }, [scope])

  async function reload() {
    if (!canManage || !scope) return
    setLoading(true)
    setError('')
    const session = getSessionScope()
    try {
      const data = await apiJson<TeamResponse>('/api/v1/staff')
      if (session !== getSessionScope()) return
      setMembers(data.members ?? [])
      setInvitations(data.invitations ?? [])
    } catch (cause: unknown) {
      if (session !== getSessionScope()) return
      setMembers([])
      setInvitations([])
      setError(errorMessage(cause))
    } finally {
      if (session === getSessionScope()) setLoading(false)
    }
  }

  useEffect(() => {
    if (canManage) void reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canManage, scope])

  async function invite(event: FormEvent) {
    event.preventDefault()
    if (!canManage || working) return
    setWorking(true)
    setInviteLink('')
    setError('')
    setMessage('')
    const originalScope = scope
    try {
      const result = await apiJson<InviteResponse>('/api/v1/staff/invitations', {
        method: 'POST',
        body: { name: name.trim(), email: email.trim(), role },
      })
      if (originalScope !== getSessionScope()) return
      // Fragments are never sent to the web server with the HTTP request.
      const link = window.location.origin + '/accept-invite#token=' + result.token
      setInviteLink(link)
      setMessage('Convite criado. Copie o link abaixo e entregue somente à pessoa convidada. Ele será mostrado uma única vez.')
      setName('')
      setEmail('')
      await reload()
    } catch (cause: unknown) {
      if (originalScope === getSessionScope()) setError(errorMessage(cause))
    } finally {
      setWorking(false)
    }
  }

  async function changeMember(member: Member, update: 'role' | 'status', value: string | boolean) {
    if (!canManage || working || member.id === currentUser || member.role === 'admin') return
    const action = update === 'role'
      ? `Alterar a função de ${member.name} para ${roleName(String(value))}?`
      : `${value ? 'Reativar' : 'Desativar'} o acesso de ${member.name} somente a esta loja?`
    if (!window.confirm(action)) return
    setWorking(true)
    setError('')
    setMessage('')
    try {
      await apiJson(`/api/v1/staff/${member.id}/${update}`, {
        method: 'PUT',
        body: update === 'role' ? { role: value } : { active: value },
      })
      setMessage('Acesso atualizado. As próximas requisições já usarão as novas permissões.')
      await reload()
    } catch (cause: unknown) {
      setError(errorMessage(cause))
    } finally {
      setWorking(false)
    }
  }

  async function revoke(invitation: Invitation) {
    if (!window.confirm(`Cancelar o convite de ${invitation.name}?`) || working) return
    setWorking(true)
    setError('')
    try {
      await apiJson(`/api/v1/staff/invitations/${invitation.id}`, { method: 'DELETE' })
      setInviteLink('')
      setMessage('Convite cancelado. O link anterior não pode mais ser usado.')
      await reload()
    } catch (cause: unknown) {
      setError(errorMessage(cause))
    } finally {
      setWorking(false)
    }
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(inviteLink)
      setMessage('Link copiado. Envie somente para o funcionário convidado.')
    } catch {
      setError('Não foi possível copiar automaticamente. Selecione o link no campo para copiar.')
    }
  }

  return (
    <div>
      <h2 className="text-lg font-semibold">Funcionários e permissões</h2>
      <p className="mt-1 text-sm text-gray-600">
        Veja quem pode usar o sistema e altere o perfil de cada pessoa nesta loja.
        Em um único computador, cada funcionário ainda precisa entrar com seu acesso.
      </p>
      {error ? <p role="alert" className="mt-3 rounded-md border p-3 text-sm text-red-700">{error}</p> : null}
      {message ? <p role="status" className="mt-3 rounded-md border p-3 text-sm">{message}</p> : null}
      {permissions && !canManage ? (
        <p className="mt-3 text-sm text-amber-800">Sua conta não pode administrar funcionários desta loja.</p>
      ) : null}
      {canManage ? (
        <>
          <section className="mt-5 rounded-2xl border border-slate-200 p-5" aria-label="Equipe da loja">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="font-semibold">Equipe da loja</h3>
              <button type="button" onClick={() => void reload()} disabled={loading || working}
                className="rounded-md border px-3 py-2 text-xs disabled:opacity-50">
                {loading ? 'Atualizando…' : 'Atualizar'}
              </button>
            </div>
            <div className="mt-3 overflow-x-auto">
              <table className="min-w-full text-left text-xs">
                <thead><tr>
                  <th className="px-2 py-2">Nome</th><th className="px-2 py-2">E-mail</th>
                  <th className="px-2 py-2">Função</th><th className="px-2 py-2">Situação</th>
                  <th className="px-2 py-2">Ações</th>
                </tr></thead>
                <tbody className="divide-y">
                  {members.map((member) => (
                    <tr key={member.id}>
                      <td className="px-2 py-2">{member.name}</td>
                      <td className="px-2 py-2">{member.email}</td>
                      <td className="px-2 py-2">{roleName(member.role)}</td>
                      <td className="px-2 py-2">{member.active ? 'Ativo' : 'Sem acesso nesta loja'}</td>
                      <td className="px-2 py-2">
                        {member.role === 'admin' || member.id === currentUser ? (
                          <span className="text-gray-500">Protegido</span>
                        ) : (
                          <div className="flex flex-wrap gap-2">
                            <button type="button" disabled={working || loading}
                              onClick={() => void changeMember(member, 'role', member.role === 'cashier' ? 'manager' : 'cashier')}
                              className="rounded-md border px-2 py-1 disabled:opacity-50">
                              Mudar para {member.role === 'cashier' ? 'gerente' : 'caixa'}
                            </button>
                            <button type="button" disabled={working || loading}
                              onClick={() => void changeMember(member, 'status', !member.active)}
                              className="rounded-md border px-2 py-1 disabled:opacity-50">
                              {member.active ? 'Suspender acesso' : 'Reativar acesso'}
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {!loading && members.length === 0 ? <p className="py-3 text-xs">Nenhum membro listado.</p> : null}
            </div>
          </section>
          <details className="mt-6 rounded-2xl border border-slate-200 p-5" aria-label="Adicionar novo acesso">
            <summary className="cursor-pointer font-semibold">Adicionar outro funcionário</summary>
            <p className="mt-1 text-xs text-gray-600">
              Mesmo usando um único computador, cada pessoa deve acessar a própria conta para manter
              os registros corretos. Crie um acesso individual e abra o link neste computador
              para a pessoa escolher sua senha. O link vence em 48 horas.
            </p>
            <form onSubmit={(event) => void invite(event)} className="mt-3 grid gap-3 sm:grid-cols-2">
              <label className="text-sm">Nome completo
                <input type="text" value={name} minLength={2} maxLength={120} required
                  onChange={(event) => setName(event.target.value)}
                  className="mt-1 block w-full rounded-md border px-3 py-2" />
              </label>
              <label className="text-sm">E-mail do funcionário
                <input type="email" value={email} maxLength={254} required
                  onChange={(event) => setEmail(event.target.value)}
                  className="mt-1 block w-full rounded-md border px-3 py-2" />
              </label>
              <label className="text-sm">Função inicial
                <select value={role} onChange={(event) => setRole(event.target.value as 'manager' | 'cashier')}
                  className="mt-1 block w-full rounded-md border px-3 py-2">
                  <option value="cashier">Caixa — vendas e operações permitidas</option>
                  <option value="manager">Gerente — gestão operacional</option>
                </select>
              </label>
              <button type="submit" disabled={working || loading}
                className="self-end rounded-md bg-gray-900 px-4 py-2 text-sm text-white disabled:opacity-50">
                {working ? 'Processando…' : 'Criar acesso individual'}
              </button>
            </form>
            {inviteLink ? (
              <div className="mt-3 rounded-md border p-3">
                <label className="block text-xs font-semibold">Link de ativação — cópia única
                  <input aria-label="Link de ativação" readOnly value={inviteLink}
                    onFocus={(event) => event.target.select()}
                    className="mt-2 block w-full rounded-md border px-2 py-2 text-xs" />
                </label>
                <button type="button" onClick={() => void copyLink()}
                  className="mt-2 rounded-md border px-3 py-2 text-xs">Copiar link</button>
                <p className="mt-2 text-xs text-amber-800">
                  Quem possuir este link poderá criar a conta. Não publique nem compartilhe em grupos.
                  Ele deixa de aparecer após sair desta tela.
                </p>
              </div>
            ) : null}
          </details>
          <details className="mt-5 rounded-2xl border border-slate-200 p-5" aria-label="Convites pendentes">
            <summary className="cursor-pointer font-semibold">Acessos aguardando ativação ({invitations.length})</summary>
            {invitations.length ? (
              <ul className="mt-3 space-y-2">
                {invitations.map((item) => (
                  <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 border-b pb-2 text-xs">
                    <span>{item.name} ({item.email}) — {roleName(item.role)} — vence em{' '}
                      {new Date(item.expires_at).toLocaleString('pt-BR')}</span>
                    <button type="button" disabled={working || loading} onClick={() => void revoke(item)}
                      className="rounded-md border px-3 py-2 disabled:opacity-50">Cancelar convite</button>
                  </li>
                ))}
              </ul>
            ) : <p className="mt-2 text-xs text-gray-600">Nenhum convite pendente.</p>}
          </details>
          <p className="mt-4 text-xs text-gray-600">
            Por segurança, a função de administrador não pode ser concedida nem alterada por esta página.
            Contas que já existem em outro CNPJ precisam de provisionamento controlado;
            o convite não vincula automaticamente uma identidade de outra empresa.
          </p>
        </>
      ) : null}
    </div>
  )
}
