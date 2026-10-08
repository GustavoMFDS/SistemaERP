export type ProductImportRow = {
  line: number
  sku: string
  name: string
  unit: string
  barcode: string | null
  ncm: string | null
  cest: string | null
  price_cash: number
  min_stock: number
}

export type ProductImportPreview = {
  valid: ProductImportRow[]
  errors: string[]
  lines: number
}

const aliases: Record<string, string[]> = {
  sku: ['sku', 'codigo', 'codigo_interno'],
  name: ['nome', 'produto', 'descricao'],
  price_cash: ['preco', 'preco_venda', 'preco_a_vista', 'price_cash'],
  unit: ['unidade', 'un', 'unit'],
  barcode: ['codigo_barras', 'codigo_de_barras', 'barcode', 'ean', 'gtin'],
  ncm: ['ncm'],
  cest: ['cest'],
  min_stock: ['estoque_minimo', 'min_stock', 'minimo'],
}

function parseCells(text: string, delimiter: string): string[][] {
  const rows: string[][] = []
  let row: string[] = []
  let value = ''
  let quoted = false
  for (let i = 0; i < text.length; i += 1) {
    const c = text[i]
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        value += '"'
        i += 1
      } else if (c === '"') {
        quoted = false
      } else {
        value += c
      }
    } else if (c === '"' && value.length === 0) {
      quoted = true
    } else if (c === delimiter) {
      row.push(value.trim())
      value = ''
    } else if (c === '\n' || c === '\r') {
      row.push(value.trim())
      rows.push(row)
      row = []
      value = ''
      if (c === '\r' && text[i + 1] === '\n') i += 1
    } else {
      value += c
    }
  }
  if (quoted) throw new Error('CSV com aspas não fechadas.')
  if (value.length || row.length) {
    row.push(value.trim())
    rows.push(row)
  }
  return rows.filter((cells) => cells.some((cell) => cell !== ''))
}

function normalizeHeading(value: string): string {
  return value.trim().toLowerCase().normalize('NFD').replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '')
}

function parseDecimal(value: string): number {
  const text = value.trim().replace(/\s/g, '')
  if (!text) return Number.NaN
  const normalized = text.includes(',') && text.includes('.')
    ? text.lastIndexOf(',') > text.lastIndexOf('.')
      ? text.replace(/\./g, '').replace(',', '.')
      : text.replace(/,/g, '')
    : text.replace(',', '.')
  if (!/^-?\d+(\.\d+)?$/.test(normalized)) return Number.NaN
  return Number(normalized)
}

export function parseProductCSV(source: string): ProductImportPreview {
  const text = source.replace(/^\uFEFF/, '')
  const firstLine = text.split(/\r?\n/, 1)[0] ?? ''
  const delimiter = (firstLine.match(/;/g) ?? []).length >= (firstLine.match(/,/g) ?? []).length
    ? ';' : ','
  const rows = parseCells(text, delimiter)
  if (rows.length < 2) throw new Error('Informe um cabeçalho e pelo menos um produto.')
  if (rows.length > 501) throw new Error('Importe até 500 produtos de cada vez.')
  const headings = rows[0].map(normalizeHeading)
  const indexes: Record<string, number> = {}
  for (const [key, names] of Object.entries(aliases)) {
    indexes[key] = headings.findIndex((heading) => names.includes(heading))
  }
  if (indexes.sku < 0 || indexes.name < 0 || indexes.price_cash < 0) {
    throw new Error('Colunas obrigatórias: sku, nome, preco.')
  }
  const get = (values: string[], key: string) => indexes[key] < 0 ? '' : (values[indexes[key]] ?? '').trim()
  const valid: ProductImportRow[] = []
  const errors: string[] = []
  const skus = new Set<string>()
  const barcodes = new Set<string>()
  for (let i = 1; i < rows.length; i += 1) {
    const values = rows[i], line = i + 1
    const sku = get(values, 'sku'), name = get(values, 'name')
    const price = parseDecimal(get(values, 'price_cash'))
    const minRaw = get(values, 'min_stock')
    const minStock = minRaw ? parseDecimal(minRaw) : 0
    const barcode = get(values, 'barcode') || null
    const ncm = get(values, 'ncm') || null
    const cest = get(values, 'cest') || null
    let reason = ''
    if (!sku || !name) reason = 'SKU ou nome vazio'
    else if (!Number.isFinite(price) || price <= 0 || Math.round(price * 100) !== price * 100) reason = 'preço inválido (use até 2 casas decimais)'
    else if (!Number.isFinite(minStock) || minStock < 0) reason = 'estoque mínimo inválido'
    else if (ncm && !/^\d{8}$/.test(ncm)) reason = 'NCM deve ter 8 dígitos'
    else if (cest && !/^\d{7}$/.test(cest)) reason = 'CEST deve ter 7 dígitos'
    else if (skus.has(sku.toLowerCase())) reason = 'SKU repetido no arquivo'
    else if (barcode && barcodes.has(barcode)) reason = 'código de barras repetido no arquivo'
    if (reason) {
      errors.push(`Linha ${line}: ${reason}`)
      continue
    }
    skus.add(sku.toLowerCase())
    if (barcode) barcodes.add(barcode)
    valid.push({
      line, sku, name, unit: get(values, 'unit') || 'un', barcode, ncm, cest,
      price_cash: price, min_stock: minStock,
    })
  }
  return { valid, errors, lines: rows.length - 1 }
}

export const PRODUCT_IMPORT_EXAMPLE =
  'sku;nome;preco;unidade;codigo_barras;ncm;cest;estoque_minimo\n' +
  'PROD-001;Arroz 5kg;24,90;un;7890000000000;10063021;;5\n'
