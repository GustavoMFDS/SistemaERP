package sefaz

import (
	"strings"
	"testing"
)

func TestBuildUnsignedNFCeInutilizationSupportsAlphanumericCNPJ(t *testing.T) {
	xml, requestID, err := BuildUnsignedNFCeInutilization(InutilizationInput{
		Environment:   EnvironmentHomologation,
		IssuerUF:      "MG",
		IssuerCNPJ:    "12.ABC.345/01DE-35",
		Year:          2026,
		Series:        1,
		StartNumber:   101,
		EndNumber:     110,
		Justification: "Falha operacional pulou esta faixa fiscal.",
	})
	if err != nil {
		t.Fatalf("BuildUnsignedNFCeInutilization: %v", err)
	}
	const wantID = "ID312612ABC34501DE3565001000000101000000110"
	if requestID != wantID {
		t.Fatalf("requestID=%s want=%s", requestID, wantID)
	}
	if len(requestID) != 43 {
		t.Fatalf("request ID length=%d want=43", len(requestID))
	}
	body := string(xml)
	for _, want := range []string{
		`<inutNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">`,
		`<infInut Id="ID312612ABC34501DE3565001000000101000000110">`,
		"<tpAmb>2</tpAmb>",
		"<xServ>INUTILIZAR</xServ>",
		"<cUF>31</cUF>",
		"<ano>26</ano>",
		"<CNPJ>12ABC34501DE35</CNPJ>",
		"<mod>65</mod>",
		"<serie>1</serie>",
		"<nNFIni>101</nNFIni>",
		"<nNFFin>110</nNFFin>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("XML missing %q: %s", want, body)
		}
	}
}

func TestBuildUnsignedNFCeInutilizationRejectsUnsafeRange(t *testing.T) {
	base := InutilizationInput{
		Environment:   EnvironmentHomologation,
		IssuerUF:      "MG",
		IssuerCNPJ:    "12.345.678/0001-95",
		Year:          2026,
		Series:        1,
		StartNumber:   1,
		EndNumber:     1,
		Justification: "Falha operacional pulou a numeracao.",
	}
	tests := []struct {
		name string
		edit func(*InutilizationInput)
	}{
		{"reverse", func(v *InutilizationInput) { v.StartNumber, v.EndNumber = 2, 1 }},
		{"too large", func(v *InutilizationInput) { v.StartNumber, v.EndNumber = 1, 10001 }},
		{"bad series", func(v *InutilizationInput) { v.Series = 890 }},
		{"bad justification", func(v *InutilizationInput) { v.Justification = "curta" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.edit(&in)
			if _, _, err := BuildUnsignedNFCeInutilization(in); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestParseInutilizationResponseRegistered(t *testing.T) {
	content := []byte(`<retInutNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">
	<infInut Id="ID131260000000001">
		<tpAmb>2</tpAmb><verAplic>MG4.00</verAplic><cStat>102</cStat>
		<xMotivo>Inutilizacao de numero homologado</xMotivo><cUF>31</cUF>
		<ano>26</ano><CNPJ>12ABC34501DE35</CNPJ><mod>65</mod>
		<serie>1</serie><nNFIni>101</nNFIni><nNFFin>110</nNFFin>
		<dhRecbto>2026-10-04T10:30:00-03:00</dhRecbto><nProt>131260000000001</nProt>
	</infInut>
</retInutNFe>`)
	got, err := ParseInutilizationResponse(content)
	if err != nil {
		t.Fatal(err)
	}
	if got.StatusCode != 102 || got.Protocol != "131260000000001" ||
		got.CNPJ != "12ABC34501DE35" || got.StartNumber != 101 || got.EndNumber != 110 {
		t.Fatalf("unexpected response: %+v", got)
	}

	result, err := inutilizationRemoteResult(
		"ID312612ABC34501DE3565001000000101000000110",
		InutilizationInput{
			Environment: EnvironmentHomologation, IssuerUF: "MG",
			IssuerCNPJ: "12.ABC.345/01DE-35", Year: 2026, Series: 1,
			StartNumber: 101, EndNumber: 110,
		},
		got,
		content,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Registered() {
		t.Fatalf("expected registered result: %+v", result)
	}
}
