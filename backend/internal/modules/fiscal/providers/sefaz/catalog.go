package sefaz

import (
	"fmt"
	"strings"
)

type CatalogEntry struct {
	UF              string
	Environment     Environment
	Services        ServiceEndpoints
	QRCodeBaseURL   string
	ConsultationURL string
	EventURL        string
	InutilizationURL string
}

func ResolveCatalogEntry(uf string, env Environment) (CatalogEntry, error) {
	uf = strings.ToUpper(strings.TrimSpace(uf))
	switch uf {
	case "MG":
		return resolveMinasGeraisCatalog(env)
	default:
		return CatalogEntry{}, fmt.Errorf("NFC-e endpoint catalog does not support UF %s yet", uf)
	}
}

func resolveMinasGeraisCatalog(env Environment) (CatalogEntry, error) {
	var base, portal string
	switch env {
	case EnvironmentProduction:
		base = "https://nfce.fazenda.mg.gov.br/nfce/services/"
		portal = "https://portalsped.fazenda.mg.gov.br/portalnfce"
	case EnvironmentHomologation:
		base = "https://hnfce.fazenda.mg.gov.br/nfce/services/"
		portal = "https://hportalsped.fazenda.mg.gov.br/portalnfce"
	default:
		return CatalogEntry{}, fmt.Errorf("invalid SEFAZ environment %q", env)
	}
	entry := CatalogEntry{
		UF:          "MG",
		Environment: env,
		Services: ServiceEndpoints{
			Status: Endpoint{
				URL:           base + "NFeStatusServico4",
				WSDLNamespace: WSDLStatusService,
			},
			Authorization: Endpoint{
				URL:           base + "NFeAutorizacao4",
				WSDLNamespace: WSDLAuthorizationService,
			},
			Consultation: Endpoint{
				URL:           base + "NFeConsultaProtocolo4",
				WSDLNamespace: WSDLConsultationService,
			},
		},
		QRCodeBaseURL:    "https://portalsped.fazenda.mg.gov.br/portalnfce/sistema/qrcode.xhtml",
		ConsultationURL:  portal,
		EventURL:         base + "NFeRecepcaoEvento4",
		InutilizationURL: base + "NFeInutilizacao4",
	}
	if err := entry.Validate(); err != nil {
		return CatalogEntry{}, err
	}
	return entry, nil
}

func (e CatalogEntry) Validate() error {
	if e.UF == "" {
		return fmt.Errorf("catalog UF is required")
	}
	if err := e.Services.Validate(); err != nil {
		return err
	}
	if _, err := validatePublicFiscalURL(e.QRCodeBaseURL); err != nil {
		return fmt.Errorf("QR Code URL: %w", err)
	}
	if _, err := validatePublicFiscalURL(e.ConsultationURL); err != nil {
		return fmt.Errorf("consultation portal URL: %w", err)
	}
	for label, raw := range map[string]string{
		"event": e.EventURL,
		"inutilization": e.InutilizationURL,
	} {
		parsed := Endpoint{
			URL:           raw,
			WSDLNamespace: SEFAZWSDLNamespacePrefix + "NFeRecepcaoEvento4",
		}
		if label == "inutilization" {
			parsed.WSDLNamespace = SEFAZWSDLNamespacePrefix + "NFeInutilizacao4"
		}
		if err := parsed.Validate(); err != nil {
			return fmt.Errorf("%s endpoint: %w", label, err)
		}
	}
	return nil
}
