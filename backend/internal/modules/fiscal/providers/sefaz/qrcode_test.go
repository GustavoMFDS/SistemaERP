package sefaz

import (
	"strings"
	"testing"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

func TestBuildOnlineQRCodeV3URL(t *testing.T) {
	got, err := BuildOnlineQRCodeV3URL(
		"https://www.sefazexemplo.gov.br/nfce/qrcode",
		EnvironmentHomologation,
		testAccessKey,
	)
	if err != nil {
		t.Fatalf("BuildOnlineQRCodeV3URL: %v", err)
	}
	want := "https://www.sefazexemplo.gov.br/nfce/qrcode?p=" + testAccessKey + "|3|2"
	if got != want {
		t.Fatalf("url=%s, want %s", got, want)
	}
}

func TestBuildOnlineQRCodeV3URLProductionWithExistingQuery(t *testing.T) {
	got, err := BuildOnlineQRCodeV3URL(
		"http://www.sefazexemplo.gov.br/nfce/qrcode?x=1",
		EnvironmentProduction,
		testAccessKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "x=1&p="+testAccessKey+"|3|1") {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestBuildOnlineQRCodeV3URLRejectsUnsafeOrInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		base string
		env  Environment
		key  string
	}{
		{"invalid scheme", "ftp://example.test/qrcode", EnvironmentHomologation, testAccessKey},
		{"userinfo", "https://user:pass@example.test/qrcode", EnvironmentHomologation, testAccessKey},
		{"fragment", "https://example.test/qrcode#frag", EnvironmentHomologation, testAccessKey},
		{"existing p", "https://example.test/qrcode?p=old", EnvironmentHomologation, testAccessKey},
		{"invalid env", "https://example.test/qrcode", Environment("bad"), testAccessKey},
		{"invalid key", "https://example.test/qrcode", EnvironmentHomologation, testAccessKey[:43] + "9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildOnlineQRCodeV3URL(tt.base, tt.env, tt.key); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestBuildOfflineQRCodeV3ForUnidentifiedConsumer(t *testing.T) {
	issuedAt := time.Date(
		2026, time.October, 5, 1, 42, 0, 0,
		time.FixedZone("BRT", -3*60*60),
	)
	key, err := fisc.BuildNFCeAccessKey(fisc.NFCeAccessKeyInput{
		UF:           "MG",
		IssuedAt:     issuedAt,
		CNPJ:         "12.345.678/0001-95",
		Series:       7,
		Number:       123,
		NumericCode:  "87654321",
		EmissionType: fisc.NFCeOfflineContingencyEmissionType,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := BuildOfflineQRCodeV3Payload(
		EnvironmentHomologation,
		key,
		issuedAt,
		platform.NewMoneyCents(900),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPayload := key + "|3|2|05|9.00||"
	if payload != wantPayload {
		t.Fatalf("payload=%q want=%q", payload, wantPayload)
	}

	const signature = "c2lnbmF0dXJl"
	got, err := BuildOfflineQRCodeV3URL(
		"https://portal.example/nfce/qrcode",
		payload,
		signature,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "p="+wantPayload+"|"+signature) {
		t.Fatalf("unexpected offline QR URL: %s", got)
	}
}

func TestBuildOfflineQRCodeV3RejectsNormalAccessKey(t *testing.T) {
	if _, err := BuildOfflineQRCodeV3Payload(
		EnvironmentHomologation,
		testAccessKey,
		time.Now(),
		platform.NewMoneyCents(100),
	); err == nil || !strings.Contains(err.Error(), "tpEmis=9") {
		t.Fatalf("expected tpEmis=9 validation, got %v", err)
	}
}

