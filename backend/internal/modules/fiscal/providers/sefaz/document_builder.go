package sefaz

import (
	"fmt"
	"strings"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

type DocumentBuilder struct {
	processVersion string
}

func NewDocumentBuilder(processVersion string) *DocumentBuilder {
	return &DocumentBuilder{processVersion: strings.TrimSpace(processVersion)}
}

func (b *DocumentBuilder) BuildUnsignedLegacyCandidate(draft fisc.NFCeDocumentDraft) ([]byte, error) {
	if draft.CustomerID != nil {
		return nil, fmt.Errorf("identified customer NFC-e is not modeled yet")
	}
	env := Environment(strings.TrimSpace(draft.Reservation.Environment))
	catalog, err := ResolveCatalogEntry(draft.Issuer.AddressState, env)
	if err != nil {
		return nil, err
	}

	items := make([]UnsignedNFCeItem, 0, len(draft.Items))
	for _, item := range draft.Items {
		items = append(items, UnsignedNFCeItem{
			Number:        item.Number,
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
			Tax:           item.Tax,
		})
	}

	payments := make([]UnsignedNFCePayment, 0, len(draft.Payments))
	for _, payment := range draft.Payments {
		payments = append(payments, UnsignedNFCePayment{
			Method: payment.Method,
			Amount: payment.Amount,
		})
	}

	return BuildUnsignedNFCeLegacyCandidate(UnsignedNFCeInput{
		Reservation:     draft.Reservation,
		Issuer:          draft.Issuer,
		Items:           items,
		Payments:        payments,
		ProcessVersion:  b.processVersion,
		QRCodeBaseURL:   catalog.QRCodeBaseURL,
		ConsultationURL: catalog.ConsultationURL,
	})
}


func (b *DocumentBuilder) BuildUnsignedCancellationEvent(
	draft fisc.NFCeCancellationDraft,
) ([]byte, string, error) {
	return BuildUnsignedCancellationEvent(CancellationEventInput{
		Environment:           Environment(strings.TrimSpace(draft.Environment)),
		IssuerUF:              draft.IssuerUF,
		IssuerCNPJ:            draft.IssuerCNPJ,
		AccessKey:             draft.AccessKey,
		AuthorizationProtocol: draft.AuthorizationProtocol,
		EventTime:             draft.EventTime,
		Sequence:              draft.Sequence,
		Justification:         draft.Justification,
	})
}
