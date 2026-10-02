package sefaz

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
)

func TestBuildUnsignedCancellationEvent(t *testing.T) {
	input := CancellationEventInput{
		Environment:           EnvironmentHomologation,
		IssuerUF:              "MG",
		IssuerCNPJ:            "12.ABC.345/01DE-35",
		AccessKey:             testAccessKey,
		AuthorizationProtocol: "131260000000001",
		EventTime:             time.Date(2026, 10, 1, 0, 45, 0, 0, time.FixedZone("BRT", -3*60*60)),
		Sequence:              1,
		Justification:         "Cancelamento solicitado por erro operacional.",
	}
	content, eventID, err := BuildUnsignedCancellationEvent(input)
	if err != nil {
		t.Fatalf("BuildUnsignedCancellationEvent: %v", err)
	}
	wantID := "ID110111" + testAccessKey + "01"
	if eventID != wantID {
		t.Fatalf("eventID=%s, want %s", eventID, wantID)
	}

	var event struct {
		Version string `xml:"versao,attr"`
		Inf struct {
			ID         string `xml:"Id,attr"`
			COrgao     string `xml:"cOrgao"`
			TpAmb      string `xml:"tpAmb"`
			CNPJ       string `xml:"CNPJ"`
			ChNFe      string `xml:"chNFe"`
			TpEvento   string `xml:"tpEvento"`
			NSeqEvento int    `xml:"nSeqEvento"`
			VerEvento  string `xml:"verEvento"`
			Det struct {
				Version    string `xml:"versao,attr"`
				DescEvento string `xml:"descEvento"`
				NProt      string `xml:"nProt"`
				XJust      string `xml:"xJust"`
			} `xml:"detEvento"`
		} `xml:"infEvento"`
	}
	if err := xml.Unmarshal(content, &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if event.Version != "1.00" || event.Inf.ID != wantID ||
		event.Inf.COrgao != "31" || event.Inf.TpAmb != "2" {
		t.Fatalf("unexpected event header: %+v", event)
	}
	if event.Inf.CNPJ != "12ABC34501DE35" || event.Inf.ChNFe != testAccessKey {
		t.Fatalf("unexpected issuer/key: %+v", event.Inf)
	}
	if event.Inf.TpEvento != "110111" || event.Inf.NSeqEvento != 1 ||
		event.Inf.Det.DescEvento != "Cancelamento" ||
		event.Inf.Det.NProt != "131260000000001" {
		t.Fatalf("unexpected event detail: %+v", event.Inf)
	}
}

func TestBuildUnsignedCancellationEventRejectsInvalidJustificationAndProtocol(t *testing.T) {
	base := CancellationEventInput{
		Environment:           EnvironmentHomologation,
		IssuerUF:              "MG",
		IssuerCNPJ:            "12.ABC.345/01DE-35",
		AccessKey:             testAccessKey,
		AuthorizationProtocol: "131260000000001",
		EventTime:             time.Now(),
		Sequence:              1,
		Justification:         "Justificativa valida para cancelar.",
	}
	short := base
	short.Justification = "curta"
	if _, _, err := BuildUnsignedCancellationEvent(short); err == nil {
		t.Fatal("expected short justification to fail")
	}
	badProtocol := base
	badProtocol.AuthorizationProtocol = "ABC"
	if _, _, err := BuildUnsignedCancellationEvent(badProtocol); err == nil {
		t.Fatal("expected invalid protocol to fail")
	}
}

func TestBuildCancellationEventBatch(t *testing.T) {
	event := []byte("<evento xmlns=\"http://www.portalfiscal.inf.br/nfe\" versao=\"1.00\"><infEvento Id=\"ID110111" + testAccessKey + "01\"></infEvento><Signature xmlns=\"http://www.w3.org/2000/09/xmldsig#\"></Signature></evento>")
	content, err := BuildCancellationEventBatch("42", event)
	if err != nil {
		t.Fatalf("BuildCancellationEventBatch: %v", err)
	}
	text := string(content)
	for _, want := range []string{"<envEvento", "<idLote>42</idLote>", "<evento "} {
		if !strings.Contains(text, want) {
			t.Fatalf("batch missing %q: %s", want, text)
		}
	}
}

func TestParseEventResponseCancellationRegistered(t *testing.T) {
	payload := []byte("<retEnvEvento xmlns=\"http://www.portalfiscal.inf.br/nfe\" versao=\"1.00\"><idLote>1</idLote><tpAmb>2</tpAmb><verAplic>TEST</verAplic><cOrgao>31</cOrgao><cStat>128</cStat><xMotivo>Lote de Evento Processado</xMotivo><retEvento versao=\"1.00\"><infEvento><tpAmb>2</tpAmb><verAplic>TEST</verAplic><cOrgao>31</cOrgao><cStat>135</cStat><xMotivo>Evento registrado e vinculado a NF-e</xMotivo><chNFe>" + testAccessKey + "</chNFe><tpEvento>110111</tpEvento><xEvento>Cancelamento homologado</xEvento><nSeqEvento>1</nSeqEvento><dhRegEvento>2026-10-01T00:46:00-03:00</dhRegEvento><nProt>131260000000002</nProt></infEvento></retEvento></retEnvEvento>")
	got, err := ParseEventResponse(payload)
	if err != nil {
		t.Fatalf("ParseEventResponse: %v", err)
	}
	if got.BatchStatusCode != 128 || !got.CancellationRegistered() ||
		got.AccessKey != testAccessKey || got.Protocol != "131260000000002" {
		t.Fatalf("unexpected event response: %+v", got)
	}
}


func TestSignCancellationEventXMLTargetsInfEvento(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 45, 0, 0, time.FixedZone("BRT", -3*60*60))
	unsigned, eventID, err := BuildUnsignedCancellationEvent(CancellationEventInput{
		Environment:           EnvironmentHomologation,
		IssuerUF:              "MG",
		IssuerCNPJ:            "12.ABC.345/01DE-35",
		AccessKey:             testAccessKey,
		AuthorizationProtocol: "131260000000001",
		EventTime:             now,
		Sequence:              1,
		Justification:         "Cancelamento solicitado por erro operacional.",
	})
	if err != nil {
		t.Fatal(err)
	}
	cert := testRSACertificate(t, now)
	signed, err := SignCancellationEventXML(unsigned, cert, eventID, now)
	if err != nil {
		t.Fatalf("SignCancellationEventXML: %v", err)
	}

	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(signed); err != nil {
		t.Fatalf("parse signed event: %v", err)
	}
	root := doc.Root()
	infEvento := directChild(root, "infEvento")
	signature := directChild(root, "Signature")
	if infEvento == nil || signature == nil {
		t.Fatalf("signed event structure incomplete: %s", signed)
	}
	if signature.Index() != infEvento.Index()+1 {
		t.Fatal("Signature must immediately follow infEvento")
	}
	signedInfo := directChild(signature, "SignedInfo")
	reference := directChild(signedInfo, "Reference")
	if reference == nil || reference.SelectAttrValue("URI", "") != "#"+eventID {
		t.Fatalf("unexpected event Reference URI")
	}
}
