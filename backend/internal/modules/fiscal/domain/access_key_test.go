package domain

import (
	"strings"
	"testing"
	"time"
)

func TestAccessKeyCheckDigitOfficialModule11Example(t *testing.T) {
	const base = "5206043300991100250655012000000780026730161"
	got, err := AccessKeyCheckDigit(base)
	if err != nil {
		t.Fatalf("AccessKeyCheckDigit: %v", err)
	}
	if got != 5 {
		t.Fatalf("check digit=%d, want 5", got)
	}
}

func TestBuildNFCeAccessKey(t *testing.T) {
	key, err := BuildNFCeAccessKey(NFCeAccessKeyInput{
		UF:           "MG",
		IssuedAt:     time.Date(2026, time.September, 30, 10, 0, 0, 0, time.FixedZone("BRT", -3*60*60)),
		CNPJ:         "12345678000195",
		Series:       1,
		Number:       42,
		NumericCode:  "12345678",
		EmissionType: NFCeNormalEmissionType,
	})
	if err != nil {
		t.Fatalf("BuildNFCeAccessKey: %v", err)
	}
	if len(key) != 44 {
		t.Fatalf("key length=%d, want 44: %s", len(key), key)
	}
	if !strings.HasPrefix(key, "31260912345678000195650010000000421") {
		t.Fatalf("unexpected access-key prefix: %s", key)
	}
	dv, err := AccessKeyCheckDigit(key[:43])
	if err != nil {
		t.Fatalf("recalculate check digit: %v", err)
	}
	if int(key[43]-'0') != dv {
		t.Fatalf("key DV=%c, calculated=%d", key[43], dv)
	}
}

func TestBuildNFCeAccessKeyRejectsUnsupportedInputs(t *testing.T) {
	base := NFCeAccessKeyInput{
		UF:           "MG",
		IssuedAt:     time.Now(),
		CNPJ:         "12345678000195",
		Series:       1,
		Number:       1,
		NumericCode:  "12345678",
		EmissionType: NFCeNormalEmissionType,
	}

	tests := []struct {
		name string
		edit func(*NFCeAccessKeyInput)
	}{
		{"invalid UF", func(v *NFCeAccessKeyInput) { v.UF = "XX" }},
		{"invalid CNPJ", func(v *NFCeAccessKeyInput) { v.CNPJ = "123" }},
		{"series above NFC-e range", func(v *NFCeAccessKeyInput) { v.Series = 890 }},
		{"zero number", func(v *NFCeAccessKeyInput) { v.Number = 0 }},
		{"invalid numeric code", func(v *NFCeAccessKeyInput) { v.NumericCode = "1234ABCD" }},
		{"unsupported emission type", func(v *NFCeAccessKeyInput) { v.EmissionType = 9 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.edit(&in)
			if _, err := BuildNFCeAccessKey(in); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
