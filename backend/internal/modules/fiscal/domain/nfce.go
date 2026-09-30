package domain

// NFCeReadiness describes whether a tenant has the non-secret data required
// to start NFC-e homologation work. It does not mean that SEFAZ transmission
// is enabled or homologated.
type NFCeReadiness struct {
	TenantID                       string   `json:"tenant_id"`
	Model                          int      `json:"model"`
	IssuerIdentityConfigured       bool     `json:"issuer_identity_configured"`
	IssuerAddressConfigured        bool     `json:"issuer_address_configured"`
	MunicipalityCodeConfigured     bool     `json:"municipality_code_configured"`
	ConfigExists                   bool     `json:"config_exists"`
	TransmissionEnabled            bool     `json:"transmission_enabled"`
	Environment                    string   `json:"environment,omitempty"`
	Series                         int      `json:"series,omitempty"`
	CSCReferenceConfigured         bool     `json:"csc_reference_configured"`
	CertificateReferenceConfigured bool     `json:"certificate_reference_configured"`
	ActiveProducts                 int      `json:"active_products"`
	ProductsMissingNCM             int      `json:"products_missing_ncm"`
	ReadyForHomologationData       bool     `json:"ready_for_homologation_data"`
	BlockingReasons                []string `json:"blocking_reasons"`
}

// NFCeConfig contains tenant-scoped NFC-e preparation. SecretRef fields hold
// identifiers in an external secret store and are intentionally excluded from
// JSON responses.
type NFCeConfig struct {
	TenantID                        string  `json:"tenant_id"`
	Enabled                         bool    `json:"enabled"`
	Environment                     string  `json:"environment"`
	Series                          int     `json:"series"`
	CSCID                           *string `json:"csc_id,omitempty"`
	CSCSecretRef                    *string `json:"-"`
	CertificateSecretRef            *string `json:"-"`
	CSCReferenceConfigured          bool    `json:"csc_reference_configured"`
	CertificateReferenceConfigured bool    `json:"certificate_reference_configured"`
}

// NFCeIssuerProfile is the issuer data that can be prepared for NFC-e without
// changing the tenant legal identity (legal name/CNPJ).
type NFCeIssuerProfile struct {
	TenantID            string  `json:"tenant_id"`
	LegalName           string  `json:"legal_name"`
	TradeName           *string `json:"trade_name,omitempty"`
	CNPJ                string  `json:"cnpj"`
	IE                  string  `json:"ie"`
	CRT                 string  `json:"crt"`
	AddressStreet       string  `json:"address_street"`
	AddressNumber       string  `json:"address_number"`
	AddressComplement   *string `json:"address_complement,omitempty"`
	AddressNeighborhood string  `json:"address_neighborhood"`
	AddressCity         string  `json:"address_city"`
	AddressCityCode     string  `json:"address_city_code"`
	AddressState        string  `json:"address_state"`
	AddressZIP          string  `json:"address_zip"`
}
