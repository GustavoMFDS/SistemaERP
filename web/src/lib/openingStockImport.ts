export type OpeningStockRow = { line: number; sku: string; quantity: number }
export type OpeningStockPreview = { rows: OpeningStockRow[]; errors: string[]; total: number }

export const OPENING_STOCK_EXAMPLE =
  'sku;quantidade\nPROD-001;15\nPROD-002;4,500\n'

function readRows(text: string, separator: string): string[][] {
  const rows: string[][] = []
  let cells: string[] = [], value = '', quoted = false
  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i]
    if (quoted) {
      if (ch === '"' && text[i + 1] === '"') { value += '"'; i += 1 }
      else if (ch === '"') quoted = false
      else value += ch
    } else if (ch === '"' && !value) quoted = true
    else if (ch === separator) { cells.push(value.trim()); value = '' }
    else if (ch === '\n' || ch === '\r') {
      cells.push(value.trim())
      rows.push(cells)
      cells = []; value = ''
      if (ch === '\r' && text[i + 1] === '\n') i += 1
    } else value += ch
  }
  if (quoted) throw new Error('Há aspas não fechadas no arquivo.')
  if (value || cells.length) { cells.push(value.trim()); rows.push(cells) }
  return rows.filter((row) => row.some(Boolean))
}

export function parseOpeningStockCSV(source: string): OpeningStockPreview {
  const text = source.replace(/^\uFEFF/, '')
  const line = text.split(/\r?\n/, 1)[0] ?? ''
  const delimiter = (line.match(/;/g) ?? []).length >= (line.match(/,/g) ?? []).length ? ';' : ','
  const table = readRows(text, delimiter)
  if (table.length < 2) throw new Error('Informe o cabeçalho e pelo menos um produto.')
  if (table.length > 101) throw new Error('Importe no máximo 100 produtos por lote.')
  const columns = table[0].map((x) => x.trim().toLowerCase().normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '').replace(/[^a-z0-9]+/g, '_'))
  const skuIndex = columns.findIndex((x) => ['sku', 'codigo', 'codigo_interno'].includes(x))
  const qtyIndex = columns.findIndex((x) => ['quantidade', 'qtd', 'quantity', 'saldo_inicial'].includes(x))
  if (skuIndex < 0 || qtyIndex < 0) throw new Error('Colunas obrigatórias: sku e quantidade.')
  const rows: OpeningStockRow[] = [], errors: string[] = [], seen = new Set<string>()
  for (let i = 1; i < table.length; i += 1) {
    const cells = table[i]
    const sku = (cells[skuIndex] ?? '').trim()
    const raw = (cells[qtyIndex] ?? '').trim().replace(/\s+/g, '')
    const decimal = raw.includes(',') ? raw.replace(',', '.') : raw
    const quantity = Number(decimal)
    let reason = ''
    if (!sku || sku.length > 120) reason = 'SKU vazio ou muito longo'
    else if (!/^\d+(\.\d{1,3})?$/.test(decimal) || !Number.isFinite(quantity) || quantity <= 0) {
      reason = 'quantidade inválida; use um número positivo com até 3 casas decimais'
    } else if (seen.has(sku)) reason = 'SKU repetido no arquivo'
    if (reason) errors.push(`Linha ${i + 1}: ${reason}`)
    else { rows.push({ line: i + 1, sku, quantity }); seen.add(sku) }
  }
  return { rows, errors, total: table.length - 1 }
}
