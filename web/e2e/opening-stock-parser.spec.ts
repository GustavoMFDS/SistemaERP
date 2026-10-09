import { expect, test } from '@playwright/test'
import { OPENING_STOCK_EXAMPLE, parseOpeningStockCSV } from '../src/lib/openingStockImport'

test('estoque inicial reconhece SKU e quantidades pt-BR com três casas', () => {
  const out = parseOpeningStockCSV('\uFEFF' + OPENING_STOCK_EXAMPLE)
  expect(out.errors).toEqual([])
  expect(out.rows).toMatchObject([
    { sku: 'PROD-001', quantity: 15 },
    { sku: 'PROD-002', quantity: 4.5 },
  ])
})

test('importação protege contra duplicações, quantidade negativa e casas excedentes', () => {
  const out = parseOpeningStockCSV(
    'sku;quantidade\nA;2,500\nA;7\nB;-1\nC;1,1234\nD;3\n',
  )
  expect(out.rows.map((r) => r.sku)).toEqual(['A', 'D'])
  expect(out.errors).toHaveLength(3)
  expect(out.errors[0]).toContain('repetido')
})

test('importação aceita campos CSV entre aspas', () => {
  const out = parseOpeningStockCSV('sku,quantidade\n"P-1","3.250"\n')
  expect(out.errors).toEqual([])
  expect(out.rows[0]).toMatchObject({ sku: 'P-1', quantity: 3.25 })
})

test('importação rejeita CSV sem cabeçalho esperado ou com aspas incompletas', () => {
  expect(() => parseOpeningStockCSV('sku;nome\nA;Produto')).toThrow('Colunas obrigatórias')
  expect(() => parseOpeningStockCSV('sku;quantidade\n"A;20')).toThrow('aspas')
  expect(() => parseOpeningStockCSV('sku;quantidade\n')).toThrow()
})
