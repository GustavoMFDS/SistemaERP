import { useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'

export default function FinancePage() {
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [data, setData] = useState<Record<string, unknown> | null>(null)

  async function load(e?: FormEvent) {
    e?.preventDefault()
    setError('')
    setLoading(true)
    try {
      const qs = new URLSearchParams()
      if (from) qs.set('from', from)
      if (to) qs.set('to', to)
      const url = `/api/v1/finance/dashboard${qs.toString() ? `?${qs.toString()}` : ''}`
      const res = await apiJson<Record<string, unknown>>(url)
      setData(res)
    } catch (e: unknown) {
      setError(errorMessage(e))
      setData(null)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Financeiro</h2>
          <p className="text-sm text-gray-600">Dashboard (MVP).</p>
        </div>
      </div>

      <form onSubmit={load} className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-5">
        <label className="block">
          <span className="text-xs text-gray-600">De (YYYY-MM-DD)</span>
          <input
            value={from}
            onChange={(e) => setFrom(e.target.value)}
            className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            placeholder="2026-01-01"
          />
        </label>
        <label className="block">
          <span className="text-xs text-gray-600">Até (YYYY-MM-DD)</span>
          <input
            value={to}
            onChange={(e) => setTo(e.target.value)}
            className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            placeholder="2026-01-31"
          />
        </label>
        <div className="md:col-span-3 flex items-end">
          <button
            disabled={loading}
            className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
          >
            {loading ? 'Carregando…' : 'Carregar'}
          </button>
        </div>
      </form>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4">
        <h3 className="text-sm font-semibold">Resposta</h3>
        <pre className="mt-2 overflow-auto rounded-md bg-gray-900 p-3 text-xs text-gray-100">
          {data ? JSON.stringify(data, null, 2) : '{ }'}
        </pre>
      </div>
    </div>
  )
}
