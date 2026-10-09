package sefaz

import (
	"strings"
	"testing"
)

func TestResolveCatalogEntryMinasGerais(t *testing.T) {
	tests := []struct {
		env             Environment
		wantServiceHost string
		wantPortalHost  string
	}{
		{EnvironmentProduction, "https://nfce.fazenda.mg.gov.br/", "https://portalsped.fazenda.mg.gov.br/"},
		{EnvironmentHomologation, "https://hnfce.fazenda.mg.gov.br/", "https://hportalsped.fazenda.mg.gov.br/"},
	}
	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			entry, err := ResolveCatalogEntry("mg", tt.env)
			if err != nil {
				t.Fatalf("ResolveCatalogEntry: %v", err)
			}
			if entry.UF != "MG" || entry.Environment != tt.env {
				t.Fatalf("unexpected catalog identity: %+v", entry)
			}
			if !strings.HasPrefix(entry.Services.Authorization.URL, tt.wantServiceHost) ||
				!strings.HasPrefix(entry.Services.Status.URL, tt.wantServiceHost) ||
				!strings.HasPrefix(entry.Services.Consultation.URL, tt.wantServiceHost) {
				t.Fatalf("unexpected service endpoints: %+v", entry.Services)
			}
			if !strings.HasPrefix(entry.ConsultationURL, tt.wantPortalHost) {
				t.Fatalf("unexpected consultation portal: %s", entry.ConsultationURL)
			}
			if entry.QRCodeBaseURL != "https://portalsped.fazenda.mg.gov.br/portalnfce/sistema/qrcode.xhtml" {
				t.Fatalf("unexpected QR URL: %s", entry.QRCodeBaseURL)
			}
		})
	}
}

func TestResolveCatalogEntryFailsClosed(t *testing.T) {
	if _, err := ResolveCatalogEntry("SP", EnvironmentHomologation); err == nil {
		t.Fatal("expected unsupported UF to fail closed")
	}
	if _, err := ResolveCatalogEntry("MG", Environment("invalid")); err == nil {
		t.Fatal("expected invalid environment to fail")
	}
}
