package sefaz

import (
	"strings"
	"testing"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

func draftFromLegacyFixture(t *testing.T) fisc.NFCeDocumentDraft {
	t.Helper()
	fixture := unsignedLegacyFixture(t)
	items := make([]fisc.NFCeDocumentItem, 0, len(fixture.Items))
	for _, item := range fixture.Items {
		net, err := item.GrossValue.SubChecked(item.DiscountValue)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, fisc.NFCeDocumentItem{
			Number:        item.Number,
			SaleItemID:    "11111111-1111-1111-1111-111111111111",
			ProductID:     "22222222-2222-2222-2222-222222222222",
			Code:          item.Code,
			Description:   item.Description,
			Unit:          item.Unit,
			NCM:           item.NCM,
			CEST:          item.CEST,
			CFOP:          item.CFOP,
			Quantity:      item.Quantity,
			UnitPrice:     item.UnitPrice,
			GrossValue:    item.GrossValue,
			DiscountValue: item.DiscountValue,
			NetValue:      net,
			Tax:           item.Tax,
		})
	}
	payments := make([]fisc.NFCeDocumentPayment, 0, len(fixture.Payments))
	for _, payment := range fixture.Payments {
		payments = append(payments, fisc.NFCeDocumentPayment{
			Method: payment.Method,
			Amount: payment.Amount,
		})
	}
	return fisc.NFCeDocumentDraft{
		Reservation:     fixture.Reservation,
		Issuer:          fixture.Issuer,
		CommercialTotal: payments[0].Amount,
		Items:           items,
		Payments:        payments,
	}
}

func TestDocumentBuilderBuildsMGHomologationCandidate(t *testing.T) {
	builder := NewDocumentBuilder("test-1")
	content, err := builder.BuildUnsignedLegacyCandidate(draftFromLegacyFixture(t))
	if err != nil {
		t.Fatalf("BuildUnsignedLegacyCandidate: %v", err)
	}
	xml := string(content)
	if !strings.Contains(xml, "hportalsped.fazenda.mg.gov.br/portalnfce") {
		t.Fatalf("MG homologation consultation URL missing: %s", xml)
	}
	if !strings.Contains(xml, "portalsped.fazenda.mg.gov.br/portalnfce/sistema/qrcode.xhtml") {
		t.Fatalf("MG QR URL missing: %s", xml)
	}
	if !strings.Contains(xml, "<verProc>test-1</verProc>") {
		t.Fatalf("verProc missing: %s", xml)
	}
}

func TestDocumentBuilderRejectsIdentifiedCustomerUntilRecipientSnapshotExists(t *testing.T) {
	builder := NewDocumentBuilder("test-1")
	draft := draftFromLegacyFixture(t)
	customerID := "33333333-3333-3333-3333-333333333333"
	draft.CustomerID = &customerID
	if _, err := builder.BuildUnsignedLegacyCandidate(draft); err == nil ||
		!strings.Contains(err.Error(), "identified customer") {
		t.Fatalf("expected identified-customer block, got %v", err)
	}
}

func TestDocumentBuilderFailsClosedForUnsupportedUF(t *testing.T) {
	builder := NewDocumentBuilder("test-1")
	draft := draftFromLegacyFixture(t)
	draft.Issuer.AddressState = "SP"
	if _, err := builder.BuildUnsignedLegacyCandidate(draft); err == nil ||
		!strings.Contains(err.Error(), "does not support UF") {
		t.Fatalf("expected unsupported-UF error, got %v", err)
	}
}
