package sefaz

import (
	"strings"
	"testing"
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
