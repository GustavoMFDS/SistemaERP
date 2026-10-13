package sefaz

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRenderPrinterCalibrationIsNonFiscalAndSized(t *testing.T) {
	for _, width := range []int{58, 80} {
		t.Run(strconv.Itoa(width)+"mm", func(t *testing.T) {
			rendered, err := RenderPrinterCalibration(width)
			if err != nil { t.Fatal(err) }
			html := string(rendered)
			for _, text := range []string{
				"TESTE DE IMPRESSÃO", "NÃO É DOCUMENTO FISCAL",
				"TESTE — SEM VENDA — SEM NFC-e",
				"receipt-test-"+strconv.Itoa(width),
				"max-width: "+strconv.Itoa(width)+"mm",
				"width: 40mm;", "width: 32mm;",
				"Teste de texto: ção, café, maçã",
				"Descrição comprida",
				"data:image/png;base64,",
			} {
				if !strings.Contains(html, text) {
					t.Fatalf("calibration missing %q", text)
				}
			}
			for _, prohibited := range []string{
				"Protocolo de Autorização", "Chave de acesso",
				"Documento Auxiliar", "<infNFe", "<script",
			} {
				if strings.Contains(html, prohibited) {
					t.Fatalf("test receipt resembles an authorized fiscal document: %q", prohibited)
				}
			}
			if strings.Contains(html, "max-width: "+strconv.Itoa(138-width)+"mm") {
				t.Fatal("unexpected other-roll width in HTML")
			}
			encoded := regexp.MustCompile(`data:image/png;base64,([A-Za-z0-9+/=]+)`).FindStringSubmatch(html)
			if len(encoded) != 2 {
				t.Fatal("no QR encoded as PNG data URL")
			}
			data, err := base64.StdEncoding.DecodeString(encoded[1])
			if err != nil {t.Fatal(err)}
			info, err := png.DecodeConfig(bytes.NewReader(data))
			if err != nil || info.Width != 256 || info.Height != 256 {
				t.Fatalf("not a 256x256 generated QR PNG: %+v %v", info, err)
			}
		})
	}
	for _, width := range []int{0, 48, 60, 100, -58} {
		if _, err := RenderPrinterCalibration(width); err == nil {
			t.Fatalf("invalid roll width %d accepted", width)
		}
	}
}
