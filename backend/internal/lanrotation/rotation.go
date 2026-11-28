package lanrotation

import (
	"context"
	"errors"
	"fmt"
)

// Phase identifies a fixed, auditable maintenance operation. The Windows
// adapter must preserve the exact previous state of all managed resources.
type Phase string

const (
	StopERP         Phase = "stop-erp"
	SwitchSCM       Phase = "switch-scm"
	SwitchFirewall  Phase = "switch-firewall"
	SwitchShortcut  Phase = "switch-shortcut"
	SwitchReceipt   Phase = "switch-receipt"
	StartERP        Phase = "start-erp"
	VerifyReadiness Phase = "verify-readiness"
)

// Adapter is the boundary for a future signed Windows maintenance tool.
// Prepare must verify independent approval, preflight, exclusive maintenance
// ownership, baseline readiness and persist a durable protected snapshot
// before returning success. Execute must record intent before every mutation.
// Restore must be idempotent, handle partial Execute failures and stop ERP
// before restoring its original metadata and starting it again. VerifyOriginal
// must check the original configuration and trusted HTTPS readiness.
// Finish must mark the journal committed atomically, retaining evidence.
// An error from Finish must leave the snapshot usable for rollback.
// Release releases only the maintenance lock; it must retain recovery data
// after any incomplete rollback. No adapter method should expose secrets.
type Adapter interface {
	Prepare(context.Context) error
	Execute(context.Context, Phase) error
	Restore(context.Context) error
	VerifyOriginal(context.Context) error
	Finish(context.Context) error
	Release()
}

// Outcome distinguishes refusal from a failed change that was restored,
// and from a failure that requires manual recovery. A successful rotation
// is not a physical terminal acceptance or commercial release approval.
type Outcome string

const (
	Refused          Outcome = "refused"
	Committed        Outcome = "committed"
	RolledBack       Outcome = "rolled-back"
	RecoveryRequired Outcome = "recovery-required"
)

// Rotate coordinates a prepared change without implementing any OS mutation.
// The caller must supply an independently reviewed adapter and bounded
// recovery context which remains usable if the apply context is cancelled.
func Rotate(ctx, recovery context.Context, a Adapter) (Outcome, error) {
	if ctx == nil || recovery == nil || a == nil || ctx.Err() != nil || recovery.Err() != nil {
		return Refused, errors.New("rotation requires active apply and recovery contexts")
	}
	// Prepare owns cleanup of any lock it acquired on failure.
	if err := a.Prepare(ctx); err != nil {
		return Refused, errors.New("rotation preparation refused")
	}
	defer a.Release()

	rollback := func(stage string) (Outcome, error) {
		if err := a.Restore(recovery); err != nil {
			return RecoveryRequired, fmt.Errorf("rotation failed at %s; restoration incomplete", stage)
		}
		if err := a.VerifyOriginal(recovery); err != nil {
			return RecoveryRequired, fmt.Errorf("rotation failed at %s; original readiness unverified", stage)
		}
		return RolledBack, fmt.Errorf("rotation failed at %s; original state restored", stage)
	}

	for _, phase := range []Phase{StopERP, SwitchSCM, SwitchFirewall, SwitchShortcut, SwitchReceipt, StartERP, VerifyReadiness} {
		if ctx.Err() != nil {
			return rollback("cancelled")
		}
		if err := a.Execute(ctx, phase); err != nil {
			return rollback(string(phase))
		}
	}
	if ctx.Err() != nil {
		return rollback("cancelled")
	}
	if err := a.Finish(ctx); err != nil {
		return rollback("commit")
	}
	return Committed, nil
}
