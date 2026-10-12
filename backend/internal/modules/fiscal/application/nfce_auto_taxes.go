package application

import (
	"context"

	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

// PrepareAutomaticNFCeTaxes copies only EXPLICIT, persisted legacy tax-profile
// fields into the immutable per-invoice calculation archive. It never guesses
// tax rates or bypasses the RTC/Selective Tax engine. Mixed/unknown RTC codes
// require accountant/engine review BEFORE inserting any automatic calculation.
// This pathway is exclusively used for test-environment homologation.
func (s *FiscalService) PrepareAutomaticNFCeTaxes(
	ctx context.Context, tenantID, actorUserID, invoiceID string,
) error {
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(ctx, tx, tenantID, invoiceID)
	if err != nil {
		return err
	}
	if reservation.Status != fisc.NFCeStatusReserved ||
		reservation.Environment != "homologation" {
		return common.ErrConflict
	}
	snapshots, err := s.fiscal.GetSaleItemFiscalSnapshots(ctx, tx, tenantID, reservation.SaleID)
	if err != nil {
		return err
	}
	existing, err := s.fiscal.GetInvoiceItemTaxCalculations(ctx, tx, tenantID, invoiceID)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 {
		return common.ErrFiscalNotReady
	}
	already := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		already[item.SaleItemID] = struct{}{}
	}
	// Do not persist partial automatic taxes if even one item needs a
	// configured RTC/IS calculation. No default rate is defensible here.
	for _, item := range snapshots {
		if item.IBSCBSCST != nil || item.IBSCBSClassification != nil ||
			item.ISCST != nil || item.ISClassification != nil {
			return common.ErrFiscalNotReady
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, item := range snapshots {
		if _, ok := already[item.SaleItemID]; ok {
			continue
		}
		_, err := s.PrepareLegacyOnlyTaxCalculation(ctx, tenantID, actorUserID,
			invoiceID, item.SaleItemID, "auto-hom-legacy-profile-v1")
		if err != nil {
			return err
		}
	}
	return nil
}
