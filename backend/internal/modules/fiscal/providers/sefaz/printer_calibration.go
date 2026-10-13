package sefaz

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// RenderPrinterCalibration produces a genuinely NON-FISCAL thermal printer
// calibration coupon. It does not build, reserve, sign, simulate or send an
// NF-e/NFC-e, and cannot be mistaken for a DANFE or a payment receipt.
// The QR contains ONLY a literal printer test string, never a tax access key.
func RenderPrinterCalibration(paperWidthMM int) ([]byte, error) {
	if paperWidthMM != 58 && paperWidthMM != 80 {
		return nil, fmt.Errorf("printer calibration supports only 58 or 80 mm")
	}
	const testPayload = "TESTE DE IMPRESSORA - NAO E DOCUMENTO FISCAL"
	qrPNG, err := qrcode.Encode(testPayload, qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("generate non-fiscal calibration QR: %w", err)
	}
	var result bytes.Buffer
	err = printerCalibrationTemplate.Execute(&result, struct {
		Width int
		GeneratedAt string
		QRCodeDataURL template.URL
	}{
		Width: paperWidthMM,
		GeneratedAt: time.Now().Format("02/01/2006 15:04:05 -07:00"),
		// The bytes are generated internally, not supplied by the client.
		QRCodeDataURL: template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG)),
	})
	if err != nil {
		return nil, fmt.Errorf("render calibration receipt: %w", err)
	}
	return result.Bytes(), nil
}

var printerCalibrationTemplate = template.Must(template.New("printer-calibration").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<title>Teste de impressora térmica - {{.Width}} mm</title>
<style>
@page { margin: 2mm; }
* { box-sizing: border-box; }
body { width: 100%; max-width: {{.Width}}mm; margin: 0 auto; padding: 2mm; font: 11px/1.25 Arial, Helvetica, sans-serif; color: #000; background: #fff; overflow-wrap: anywhere; }
.center { text-align: center; }
.strong { font-weight: 700; }
.banner { border: 2px solid #000; padding: 2mm; margin: 2mm 0; font-weight: 700; text-align: center; break-inside: avoid; page-break-inside: avoid; }
.section { border-top: 1px dashed #000; padding-top: 2mm; margin-top: 2mm; }
.row { display: flex; justify-content: space-between; gap: 2mm; }
.qr { text-align: center; break-inside: avoid; page-break-inside: avoid; }
.qr img { width: 32mm; height: 32mm; image-rendering: pixelated; }
.ruler { margin-top: 3mm; width: 40mm; max-width: 100%; height: 3mm; border-left: 1px solid #000; border-right: 1px solid #000; border-bottom: 1px solid #000; }
@media print {
  body { width: 100%; max-width: {{.Width}}mm; margin: 0 auto; padding: 0 1mm; }
}
</style>
</head>
<body class="receipt-test-{{.Width}}">
<div class="banner">TESTE DE IMPRESSÃO
<div>NÃO É DOCUMENTO FISCAL</div>
</div>
<div class="center strong">Teste de bobina de {{.Width}} mm</div>
<div class="center">{{.GeneratedAt}}</div>
<section class="section">
  <div class="strong">Teste de texto: ção, café, maçã, número, preço.</div>
  <p>Descrição comprida para verificar a quebra de linhas no papel estreito e impedir o corte de palavras ou preços no lado direito da bobina.</p>
  <div class="row"><span>Qtd. 2 UN x 5,00</span><strong>10,00</strong></div>
  <div class="row"><span>Teste de alinhamento</span><strong>R$ 123,45</strong></div>
</section>
<section class="section qr">
  <img src="{{.QRCodeDataURL}}" alt="QR Code de teste da impressora, sem validade fiscal">
  <div>QR de teste: ao ler, deve aparecer uma mensagem de TESTE DE IMPRESSORA.</div>
</section>
<div class="ruler" aria-label="Régua de referência de 40 mm"></div>
<div class="center">Referência de 40 mm: conferir com régua física.</div>
<div class="banner">TESTE — SEM VENDA — SEM NFC-e</div>
</body>
</html>`))
