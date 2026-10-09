import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiDownload, apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Receipt = {
  batch_id: string
  item_count: number
  actor_name: string
  created_at: string
}

type Page = {
  items: Receipt[]
  limit: number
  offset: number
  has_more: boolean
}

type Props = {
  kind: 'products' | 'opening-stock'
  refreshVersion?: number
}

const pageSize = 10
const maxOffset = 5000

function validDateRange(from: string, to: string): boolean {
  if (!from || !to) return true
  const first = Date.parse(from + 'T00:00:00Z')
  const last = Date.parse(to + 'T00:00:00Z')
  return Number.isFinite(first) && Number.isFinite(last) &&
    last >= first && last - first <= 365 * 24 * 60 * 60 * 1000
}

export default function ImportBatchHistory({ kind, refreshVersion = 0 }: Props) {
  const [offset, setOffset] = useState(0)
  const [page, setPage] = useState<Page | null>(null)
  const [refreshCounter, setRefreshCounter] = useState(0)
  const [loading, setLoading] = useState(true)
  const [exporting, setExporting] = useState(false)
  const [error, setError] = useState('')
  const [exportError, setExportError] = useState('')
  const [fromDraft, setFromDraft] = useState('')
  const [toDraft, setToDraft] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const requestSequence = useRef(0)
  const scope = getSessionScope()

  const endpoint = kind === 'products'
    ? '/api/v1/products/import-batches/history'
    : '/api/v1/inventory/opening-stock/batches/history'

  const periodValid = validDateRange(fromDraft, toDraft)
  const filtersChanged = fromDraft !== from || toDraft !== to

  function buildQuery(): string {
    const query = new URLSearchParams()
    if (from) query.set('from', from)
    if (to) query.set('to', to)
    return query.toString()
  }

  useEffect(() => {
    const requestID = ++requestSequence.current
    const requestScope = getSessionScope()
    if (!requestScope) {
      setPage(null)
      setError('Entre na loja para consultar o histórico.')
      setLoading(false)
      return
    }
    setLoading(true)
    setPage(null)
    setError('')
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(offset) })
    if (from) params.set('from', from)
    if (to) params.set('to', to)
    void apiJson<Page>(`${endpoint}?${params.toString()}`).then((data) => {
      if (requestID !== requestSequence.current || requestScope !== getSessionScope()) return
      setPage(data)
      setLoading(false)
    }).catch((cause: unknown) => {
      if (requestID !== requestSequence.current || requestScope !== getSessionScope()) return
      setPage(null)
      setError(errorMessage(cause))
      setLoading(false)
    })
    return () => { ++requestSequence.current }
  }, [endpoint, offset, refreshCounter, refreshVersion, scope, from, to])

  function applyFilters() {
    if (!periodValid) return
    setOffset(0)
    setFrom(fromDraft)
    setTo(toDraft)
    setExportError('')
    setRefreshCounter((version) => version + 1)
  }

  function clearFilters() {
    setFromDraft('')
    setToDraft('')
    setFrom('')
    setTo('')
    setOffset(0)
    setExportError('')
    setRefreshCounter((version) => version + 1)
  }

  async function exportCSV() {
    if (loading || exporting || filtersChanged || !periodValid || !scope) return
    setExporting(true)
    setExportError('')
    const fileName = kind === 'products'
      ? 'historico-importacao-produtos.csv'
      : 'historico-estoque-inicial.csv'
    const query = buildQuery()
    try {
      await apiDownload(
        `${endpoint}/export.csv${query ? '?'+query : ''}`,
        fileName,
        'text/csv;charset=utf-8',
      )
    } catch (cause: unknown) {
      setExportError(errorMessage(cause))
    } finally {
      setExporting(false)
    }
  }

  return (
    <section className="mt-5 rounded-md border p-3" aria-label="Histórico de importações confirmadas">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 className="text-sm font-semibold">Histórico de importações confirmadas</h4>
          <p className="mt-1 text-xs text-gray-600">
            Somente lotes concluídos nesta loja. Uma tentativa sem confirmação pode não aparecer.
            Não mostra os dados da planilha.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Link to="/imports" className="rounded-md border px-3 py-2 text-xs text-blue-700 hover:bg-gray-50">
            Ver linha do tempo das importações
          </Link>
          <button type="button" disabled={loading}
            onClick={() => setRefreshCounter((value) => value + 1)}
            className="rounded-md border px-3 py-2 text-xs disabled:opacity-50">
            Atualizar histórico
          </button>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-end gap-3 rounded-md bg-gray-50 p-3">
        <label className="text-xs">
          <span className="block text-gray-700">Data inicial</span>
          <input type="date" value={fromDraft} onChange={(event) => setFromDraft(event.target.value)}
            className="mt-1 rounded-md border bg-white px-2 py-2 text-sm" />
        </label>
        <label className="text-xs">
          <span className="block text-gray-700">Data final</span>
          <input type="date" value={toDraft} onChange={(event) => setToDraft(event.target.value)}
            className="mt-1 rounded-md border bg-white px-2 py-2 text-sm" />
        </label>
        <button type="button" onClick={applyFilters} disabled={!periodValid || loading}
          className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
          Filtrar período
        </button>
        <button type="button" onClick={clearFilters} disabled={loading}
          className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
          Limpar filtros
        </button>
        <button type="button" onClick={() => void exportCSV()}
          disabled={loading || exporting || !periodValid || filtersChanged}
          className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50">
          {exporting ? 'Preparando CSV…' : 'Baixar histórico CSV'}
        </button>
        <p className="w-full text-xs text-gray-600">
          Datas no horário de Brasília. Exportação de até 1.000 lotes, somente metadados.
          Para períodos muito grandes, reduza as datas.
        </p>
        {!periodValid ? (
          <p role="alert" className="w-full text-xs text-red-700">
            Informe a data final igual ou posterior à inicial, com intervalo máximo de 365 dias.
          </p>
        ) : null}
        {filtersChanged && periodValid ? (
          <p className="w-full text-xs text-amber-800">
            Aplique os filtros antes de exportar o histórico.
          </p>
        ) : null}
        {exportError ? (
          <p role="alert" className="w-full text-xs text-red-700">
            Não foi possível baixar o CSV: {exportError}
          </p>
        ) : null}
      </div>

      {loading ? <p role="status" className="mt-3 text-xs text-gray-600">Consultando histórico…</p> : null}
      {error ? (
        <p role="alert" className="mt-3 text-xs text-red-700">
          Não foi possível consultar o histórico: {error}
        </p>
      ) : null}
      {!loading && page && page.items.length === 0 ? (
        <p className="mt-3 text-xs text-gray-600">Nenhum lote confirmado nesta página.</p>
      ) : null}
      {!loading && page && page.items.length > 0 ? (
        <div className="mt-3 overflow-auto">
          <table className="min-w-full text-left text-xs">
            <thead className="bg-gray-50 text-gray-600">
              <tr>
                <th className="px-2 py-2">Data</th>
                <th className="px-2 py-2">Responsável</th>
                <th className="px-2 py-2">Produtos</th>
                <th className="px-2 py-2">Identificador do lote</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {page.items.map((item) => (
                <tr key={item.batch_id}>
                  <td className="px-2 py-2">
                    {new Date(item.created_at).toLocaleString('pt-BR', { timeZone: 'America/Sao_Paulo' })}
                  </td>
                  <td className="px-2 py-2">{item.actor_name}</td>
                  <td className="px-2 py-2">{item.item_count}</td>
                  <td className="px-2 py-2"><code className="break-all">{item.batch_id}</code></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <div className="mt-3 flex flex-wrap items-center gap-3 text-xs">
        <button type="button" disabled={loading || offset === 0}
          onClick={() => setOffset((value) => Math.max(0, value - pageSize))}
          className="rounded-md border px-3 py-2 disabled:opacity-50">
          Página anterior
        </button>
        <span>Página {Math.floor(offset / pageSize) + 1}</span>
        <button type="button" disabled={loading || !page?.has_more || offset + pageSize > maxOffset}
          onClick={() => setOffset((value) => value + pageSize)}
          className="rounded-md border px-3 py-2 disabled:opacity-50">
          Próxima página
        </button>
        {offset >= maxOffset ? (
          <span className="text-amber-800">Limite da consulta atingido; reduza o período para lotes mais antigos.</span>
        ) : null}
      </div>
    </section>
  )
}
