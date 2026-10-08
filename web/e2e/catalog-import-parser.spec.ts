import { expect, test } from '@playwright/test'
import { parseProductCSV, PRODUCT_IMPORT_EXAMPLE } from '../src/lib/productImport'

test('modelo CSV brasileiro preserva valores decimais, códigos e NCM', () => {
  const result = parseProductCSV('\uFEFF' + PRODUCT_IMPORT_EXAMPLE)
  expect(result.errors).toEqual([])
  expect(result.valid).toHaveLength(1)
  expect(result.valid[0]).toMatchObject({
    sku: 'PROD-001',
    name: 'Arroz 5kg',
    price_cash: 24.9,
    barcode: '7890000000000',
    ncm: '10063021',
    min_stock: 5,
  })
})

test('CSV com aspas e separador por vírgulas', () => {
  const result = parseProductCSV('sku,nome,preco,unidade\n\"P-1\",\"Arroz, tipo 1\",\"14.90\",un\n')
  expect(result.errors).toEqual([])
  expect(result.valid[0]).toMatchObject({ sku: 'P-1', name: 'Arroz, tipo 1', price_cash: 14.9 })
})

test('importação rejeita duplicados e classificação inválida antes da escrita', () => {
  const result = parseProductCSV('sku;nome;preco;ncm;codigo_barras\nA;Produto 1;10,00;12345678;789000001\nA;Produto 2;12,00;12345678;790000000\nB;Produto 3;13,00;1234;790000000\n')
  expect(result.valid).toHaveLength(1)
  expect(result.errors).toHaveLength(2)
  expect(result.errors[0]).toContain('SKU repetido')
  expect(result.errors[1]).toContain('NCM')
})

test('importação não aceita arquivo vazio, campos obrigatórios ausentes ou aspas inválidas', () => {
  expect(() => parseProductCSV('sku;nome;preco\n')).toThrow()
  expect(() => parseProductCSV('sku;nome\nA;Produto')).toThrow('Colunas obrigatórias')
  expect(() => parseProductCSV('sku;nome;preco\nA;\"Produto;10')).toThrow('aspas não fechadas')
})

test('prévia segue limites de SKU, código de barras, unidade e escala do estoque', () => {
  const csv = [
    'sku;nome;preco;unidade;codigo_barras;estoque_minimo',
    'A;Produto válido;10,00;un;;1,250',
    'B;Produto com barcode curto;11;un;123;0',
    'C;Produto com unidade longa;11;unidade-muito-longa;;0',
    'D;Produto com estoque fracionário;11;un;;1,1234',
    'E;X;11;un;;0',
  ].join('\n')
  const out = parseProductCSV(csv)
  expect(out.valid.map((row) => row.sku)).toEqual(['A'])
  expect(out.errors).toHaveLength(4)
  expect(out.errors.join(' ')).toMatch(/código de barras/)
  expect(out.errors.join(' ')).toMatch(/unidade/)
  expect(out.errors.join(' ')).toMatch(/estoque mínimo/)
  expect(out.errors.join(' ')).toMatch(/nome/)
})
