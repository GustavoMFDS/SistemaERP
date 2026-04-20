import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { apiDownload, apiJson } from '../lib/api'

type GenerateResponse = { invoice_id: string; xml_file_id: string }

type XMLFile = {
  id: string
  invoice_id: string
  file_name: string
  sha256: string
  created_at: string
}

type ListResponse = { items: XMLFile[]; total: number }

export default function FiscalPage() {
  const [saleId, setSaleId] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [items, setItems] = useState<XMLFile[]>([])
  const [total, setTotal] = useState(0)

  async function load() {
    setError('')
    setLoading(true)
    try {
      const data = await apiJson<ListResponse>('/api/v1/fiscal/nfe/xml?limit=50&offset=0')
      setItems(data.items)
      setTotal(data.total)
    } catch (e: any) {
      setError(String(e?.bodyText ?? e?.message ?? e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function onGenerate(e: FormEvent) {
    e.preventDefault()
    setError('')
    setMessage('')
    try {
      const res = await apiJson<GenerateResponse>('/api/v1/fiscal/nfe/xml', {
        method: 'POST',
        body: { sale_id: saleId.trim() },
      })
      setMessage(`Gerado: invoice_id=${res.invoice_id} xml_file_id=${res.xml_file_id}`)
      setSaleId('')
      await load()
    } catch (e: any) {
      setError(String(e?.bodyText ?? e?.message ?? e))
    }
  }

  async function onDownload(x: XMLFile) {
    setError('')
    try {
      await apiDownload(
        `/api/v1/fiscal/nfe/xml/${x.id}/download`,
        x.file_name || `nfe-${x.id}.xml`,
        'application/xml',
      )
    } catch (e: any) {
      setError(String(e?.bodyText ?? e?.message ?? e))
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Fiscal (NF-e XML MVP)</h2>
          <p className="text-sm text-gray-600">Total de XMLs: {total}</p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      <form onSubmit={onGenerate} className="mt-4 flex flex-col gap-2 md:flex-row md:items-end">
        <label className="block flex-1">
          <span className="text-xs text-gray-600">sale_id</span>
          <input
            value={saleId}
            onChange={(e) => setSaleId(e.target.value)}
            className="mt-1 w-full rounded-md border px-3 py-2 text-sm font-mono"
            placeholder="Cole o ID da venda finalizada"
            required
          />
        </label>
        <button className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white">
          Gerar XML
        </button>
      </form>

      {message ? (
        <div className="mt-3 rounded-md border border-green-200 bg-green-50 p-2 text-sm text-green-700">
          {message}
        </div>
      ) : null}

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 overflow-auto rounded-md border">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-gray-50 text-xs text-gray-600">
            <tr>
              <th className="px-3 py-2">Arquivo</th>
              <th className="px-3 py-2">SHA256</th>
              <th className="px-3 py-2">Criado</th>
              <th className="px-3 py-2"></th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {items.map((x) => (
              <tr key={x.id}>
                <td className="px-3 py-2 font-mono text-xs">{x.file_name}</td>
                <td className="px-3 py-2 font-mono text-xs">{x.sha256}</td>
                <td className="px-3 py-2 font-mono text-xs">{x.created_at}</td>
                <td className="px-3 py-2">
                  <button
                    type="button"
                    onClick={() => void onDownload(x)}
                    className="text-xs text-blue-700 hover:underline"
                  >
                    Download
                  </button>
                </td>
              </tr>
            ))}
            {items.length === 0 ? (
              <tr>
                <td className="px-3 py-6 text-center text-sm text-gray-500" colSpan={4}>
                  Nenhum XML gerado.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  )
}
