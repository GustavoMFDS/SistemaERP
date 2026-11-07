export type NonFiscalReceiptLine = {
  name: string
  quantity: number
  unit: string
}

type BuildNonFiscalReceiptInput = {
  brandName: string
  saleId: string
  total: number
  items: NonFiscalReceiptLine[]
  paymentMethods: string[]
  paperWidthMM: 58 | 80
}

const paymentMethodLabels: Record<string, string> = {
  cash: 'Dinheiro',
  pix: 'PIX',
  debit: 'Cartão de débito',
  credit: 'Cartão de crédito',
  transfer: 'Transferência',
  voucher: 'Vale',
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export function buildNonFiscalReceiptHtml(input: BuildNonFiscalReceiptInput): string {
  const money = new Intl.NumberFormat('pt-BR', {
    style: 'currency',
    currency: 'BRL',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
  const quantity = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 3 })
  const items = input.items
    .map(
      (item) => `
        <tr>
          <td class="item">${escapeHtml(item.name)}<small>${escapeHtml(item.unit)}</small></td>
          <td class="quantity">${quantity.format(item.quantity)}</td>
        </tr>`,
    )
    .join('')
  const payments = Array.from(new Set(input.paymentMethods))
    .map((method) => {
      const label = paymentMethodLabels[method] ?? method
      return `<li>${escapeHtml(label)}</li>`
    })
    .join('')
  const width = input.paperWidthMM

  return `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>${escapeHtml(input.brandName)} — Comprovante não fiscal</title>
  <style>
    @page { size: ${width}mm auto; margin: 3mm; }
    * { box-sizing: border-box; }
    body {
      width: ${width - 6}mm;
      margin: 0 auto;
      color: #111827;
      font: 12px/1.4 Arial, sans-serif;
      overflow-wrap: anywhere;
    }
    header, .total, .notice { text-align: center; }
    header { border-bottom: 1px dashed #475569; padding-bottom: 8px; }
    h1 { margin: 0 0 4px; font-size: 18px; }
    .kind { font-size: 12px; font-weight: 800; letter-spacing: .06em; }
    .notice { margin: 6px 0; font-size: 10px; }
    .sale { margin: 8px 0; font-size: 10px; }
    table { width: 100%; border-collapse: collapse; }
    th { border-bottom: 1px solid #475569; text-align: left; }
    th:last-child, td.quantity { text-align: right; }
    td { padding: 4px 0; vertical-align: top; }
    td small { display: block; color: #475569; }
    .total { margin: 10px 0; border-top: 1px dashed #475569; padding-top: 8px; font-size: 15px; }
    .payments { border-top: 1px dashed #475569; padding: 8px 0; }
    .payments ul { margin: 4px 0 0; padding-left: 18px; }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>${escapeHtml(input.brandName)}</h1>
      <div class="kind">COMPROVANTE NÃO FISCAL</div>
      <p class="notice">Não substitui documento fiscal. A obrigação fiscal permanece registrada.</p>
    </header>
    <p class="sale">Venda: <span>${escapeHtml(input.saleId)}</span></p>
    <table aria-label="Itens da venda">
      <thead><tr><th>Produto</th><th>Qtd.</th></tr></thead>
      <tbody>${items}</tbody>
    </table>
    <p class="total">Total confirmado: <strong>${money.format(input.total)}</strong></p>
    <section class="payments" aria-label="Formas de pagamento">
      Forma(s) informada(s):
      <ul>${payments}</ul>
    </section>
    <p class="notice">Este comprovante não é NFC-e nem DANFE.</p>
  </main>
</body>
</html>`
}
