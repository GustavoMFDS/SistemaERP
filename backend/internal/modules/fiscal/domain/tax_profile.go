package domain

type ProductFiscalProfile struct {
	TenantID             string  `json:"tenant_id"`
	ProductID            string  `json:"product_id"`
	CFOP                 string  `json:"cfop"`
	ICMSOrigin           string  `json:"icms_origin"`
	ICMSRegime           string  `json:"icms_regime"`
	ICMSCode             string  `json:"icms_code"`
	PISCST               string  `json:"pis_cst"`
	COFINSCST            string  `json:"cofins_cst"`
	IBSCBSCST            *string `json:"ibs_cbs_cst,omitempty"`
	IBSCBSClassification *string `json:"ibs_cbs_classification,omitempty"`
	ISCST                *string `json:"is_cst,omitempty"`
	ISClassification     *string `json:"is_classification,omitempty"`
	ReferenceVersion     string  `json:"reference_version"`
}

type SaleItemFiscalSnapshot struct {
	TenantID             string  `json:"tenant_id"`
	SaleItemID           string  `json:"sale_item_id"`
	SaleID               string  `json:"sale_id"`
	ProductID            string  `json:"product_id"`
	ProductCode          string  `json:"product_code"`
	ProductDescription   string  `json:"product_description"`
	Unit                 string  `json:"unit"`
	NCM                  string  `json:"ncm"`
	CEST                 *string `json:"cest,omitempty"`
	CFOP                 string  `json:"cfop"`
	ICMSOrigin           string  `json:"icms_origin"`
	ICMSRegime           string  `json:"icms_regime"`
	ICMSCode             string  `json:"icms_code"`
	PISCST               string  `json:"pis_cst"`
	COFINSCST            string  `json:"cofins_cst"`
	IBSCBSCST            *string `json:"ibs_cbs_cst,omitempty"`
	IBSCBSClassification *string `json:"ibs_cbs_classification,omitempty"`
	ISCST                *string `json:"is_cst,omitempty"`
	ISClassification     *string `json:"is_classification,omitempty"`
	ReferenceVersion     string  `json:"reference_version"`
	SnapshotSHA256       string  `json:"snapshot_sha256"`
}
