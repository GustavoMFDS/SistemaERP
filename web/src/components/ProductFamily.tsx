import { useEffect, useState } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Option = {
  id: string
  sku: string
  name: string
  option_label: string
  is_base: boolean
  qty_on_hand: number
  price_cash: number
  active: boolean
}
type Family = { parent_id: string; items: Option[] }

export default function ProductFamily({ productId }: { productId: string }) {
  const [family, setFamily] = useState<Family | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    const scope = getSessionScope()
    setFamily(null)
    setError('')
    setLoading(true)
    apiJson<Family>('/api/v1/products/' + encodeURIComponent(productId) + '/variations')
      .then((result) => {
        if (!cancelled && getSessionScope() === scope) setFamily(result)
      })
      .catch((failure: unknown) => {
        if (!cancelled && getSessionScope() === scope) setError(errorMessage(failure))
      })
      .finally(() => {
        if (!cancelled && getSessionScope() === scope) setLoading(false)
      })
    return () => { cancelled = true }
  }, [productId])

  return (
    <section aria-label="Cores e tamanhos do produto" className="rounded-xl bg-slate-50 p-4">
      <h3 className="font-semibold text-slate-800">Cores e tamanhos</h3>
      <p className="mt-1 text-xs text-slate-600">
        Cada opção cadastrada com SKU próprio tem estoque separado. Fotos com nomes não dividem o saldo.
      </p>
      {loading ? <p className="mt-3 text-sm">Carregando opções…</p> : null}
      {error ? <p role="alert" className="mt-3 text-sm text-red-700">{error}</p> : null}
      {!loading && family && family.items.length === 1 ?
        <p className="mt-3 text-sm text-slate-600">Este produto ainda não tem outras opções com estoque próprio.</p> : null}
      {family && family.items.length > 1 ? (
        <div className="mt-3 overflow-auto">
          <table className="min-w-full text-left text-sm">
            <thead><tr>
              <th className="px-3 py-2">Opção</th>
              <th className="px-3 py-2">Código SKU</th>
              <th className="px-3 py-2">Em estoque</th>
              <th className="px-3 py-2">Preço</th>
              <th className="px-3 py-2">Situação</th>
            </tr></thead>
            <tbody>{family.items.map((option) => (
              <tr key={option.id} className="border-t border-slate-200">
                <td className="px-3 py-2">{option.is_base ? 'Padrão' : option.option_label}</td>
                <td className="px-3 py-2 font-mono text-xs">{option.sku}</td>
                <td className="px-3 py-2">{option.qty_on_hand.toFixed(2)}</td>
                <td className="px-3 py-2">R$ {option.price_cash.toFixed(2)}</td>
                <td className="px-3 py-2">{option.active ? 'Ativo' : 'Inativo'}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      ) : null}
    </section>
  )
}
