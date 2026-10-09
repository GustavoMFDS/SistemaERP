package application

import (
	"testing"

	"github.com/example/sistemaemgo/internal/platform"
	"github.com/go-playground/validator/v10"
)

func validProductRequestForFiscalTest() ProductCreateRequest {
	return ProductCreateRequest{
		SKU:       "SKU-1",
		Name:      "Produto fiscal",
		Unit:      "UN",
		PriceCash: platform.NewMoneyCents(1000),
		Active:    true,
	}
}

func TestNormalizeProductRequestTrimsFiscalCodes(t *testing.T) {
	ncm := " 01012100 "
	cest := " 0100100 "
	req := validProductRequestForFiscalTest()
	req.NCM = &ncm
	req.CEST = &cest

	got := normalizeProductRequest(req)
	if got.NCM == nil || *got.NCM != "01012100" {
		t.Fatalf("NCM=%v, want 01012100", got.NCM)
	}
	if got.CEST == nil || *got.CEST != "0100100" {
		t.Fatalf("CEST=%v, want 0100100", got.CEST)
	}
}

func TestNormalizeProductRequestTurnsBlankFiscalCodesIntoNil(t *testing.T) {
	blank := "   "
	req := validProductRequestForFiscalTest()
	req.NCM = &blank
	req.CEST = &blank

	got := normalizeProductRequest(req)
	if got.NCM != nil {
		t.Fatalf("NCM=%v, want nil", got.NCM)
	}
	if got.CEST != nil {
		t.Fatalf("CEST=%v, want nil", got.CEST)
	}
}

func TestProductFiscalCodeValidation(t *testing.T) {
	v := validator.New()

	validNCM := "01012100"
	validCEST := "0100100"
	req := validProductRequestForFiscalTest()
	req.NCM = &validNCM
	req.CEST = &validCEST
	if err := v.Struct(normalizeProductRequest(req)); err != nil {
		t.Fatalf("valid fiscal codes rejected: %v", err)
	}

	invalidNCM := "1234567A"
	req.NCM = &invalidNCM
	if err := v.Struct(normalizeProductRequest(req)); err == nil {
		t.Fatal("expected alphanumeric NCM to be rejected")
	}

	req.NCM = &validNCM
	invalidCEST := "123456"
	req.CEST = &invalidCEST
	if err := v.Struct(normalizeProductRequest(req)); err == nil {
		t.Fatal("expected six-digit CEST to be rejected")
	}
}
