package config

import (
	"strings"
	"testing"
)

func validHomologationAutoConfig() Config {
	cfg := validProdConfig()
	cfg.Env = "staging"
	cfg.FiscalProvider = "sefaz"
	cfg.NFCeSEFAZProductionEnabled = false
	cfg.NFCeSEFAZHomologationEnabled = true
	cfg.NFCeAutoHomologationEnabled = true
	cfg.NFCeCertificateSecretDir = "/run/secrets/nfce"
	cfg.NFCeSchemaDir = "/opt/nfce/schema"
	cfg.NFCeSchemaEntrypoint = "leiauteNFe_v4.00.xsd"
	cfg.NFCeEventSchemaEntrypoint = "envEventoCancNFe_v1.00.xsd"
	cfg.NFCeInutilizationSchemaEntrypoint = "inutNFe_v4.00.xsd"
	return cfg
}

func TestNFCeAutoHomologationCanOnlyEnableWithFullHomologationGate(t *testing.T) {
	cfg := validHomologationAutoConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete non-production homologation should be valid: %v", err)
	}
	testCases := []struct {
		name   string
		change func(*Config)
	}{
		{"disabled SEFAZ flag", func(c *Config) { c.NFCeSEFAZHomologationEnabled = false }},
		{"production flag", func(c *Config) {
			c.NFCeSEFAZProductionEnabled = true
		}},
		{"production APP_ENV", func(c *Config) {
			c.Env = "prod"
		}},
		{"MVP provider", func(c *Config) {
			c.FiscalProvider = "mvp"
		}},
		{"missing certificate directory", func(c *Config) {
			c.NFCeCertificateSecretDir = ""
		}},
		{"missing XML schema", func(c *Config) {
			c.NFCeSchemaEntrypoint = ""
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := cfg
			tc.change(&v)
			if err := v.Validate(); err == nil {
				t.Fatal("unsafe NFC-e automatic startup configuration accepted")
			}
		})
	}
}

func TestNFCeAutoHomologationRequiresExplicitOptIn(t *testing.T) {
	cfg := validHomologationAutoConfig()
	cfg.NFCeAutoHomologationEnabled = false
	if err := cfg.Validate(); err != nil {
		t.Fatalf("SEFAZ manual homologation without automatic processing should work: %v", err)
	}
	cfg.NFCeAutoHomologationEnabled = true
	cfg.NFCeSEFAZHomologationEnabled = false
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "NFCE_AUTO_HOMOLOGATION_ENABLED") {
		t.Fatalf("expected manual-invocation gate enforcement, got: %v", err)
	}
}
