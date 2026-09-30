package domain

// NFCeReadiness describes whether a tenant has the non-secret data required
// to start NFC-e homologation work. It does not mean that SEFAZ transmission
// is enabled or homologated.
type NFCeReadiness struct {
	TenantID                      string   `json:"tenant_id"`
	Model                         int      `json:"model"`
	IssuerIdentityConfigured      bool     `json:"issuer_identity_configured"`
	IssuerAddressConfigured       bool     `json:"issuer_address_configured"`
	MunicipalityCodeConfigured    bool     `json:"municipality_code_configured"`
	ConfigExists                  bool     `json:"config_exists"`
	TransmissionEnabled           bool     `json:"transmission_enabled"`
	Environment                   string   `json:"environment,omitempty"`
	Series                        int      `json:"series,omitempty"`
	CSCReferenceConfigured        bool     `json:"csc_reference_configured"`
	CertificateReferenceConfigured bool    `json:"certificate_reference_configured"`
	ActiveProducts                int      `json:"active_products"`
	ProductsMissingNCM            int      `json:"products_missing_ncm"`
	ReadyForHomologationData      bool     `json:"ready_for_homologation_data"`
	BlockingReasons               []string `json:"blocking_reasons"`
}
