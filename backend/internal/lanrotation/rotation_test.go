package lanrotation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeAdapter struct {
	calls                       []string
	fail                        string
	restoreFail, verifyFail     bool
	cancel                      context.CancelFunc
	partial, restored, released bool
}

func (f *fakeAdapter) Prepare(context.Context) error {
	f.calls = append(f.calls, "prepare")
	if f.fail == "prepare" {
		return errors.New("synthetic private detail")
	}
	return nil
}

func (f *fakeAdapter) Execute(_ context.Context, phase Phase) error {
	f.calls = append(f.calls, string(phase))
	f.partial = true
	if f.cancel != nil && phase == SwitchSCM {
		f.cancel()
	}
	if f.fail == string(phase) {
		return errors.New("synthetic private detail")
	}
	return nil
}

func (f *fakeAdapter) Restore(ctx context.Context) error {
	f.calls = append(f.calls, "restore")
	if ctx.Err() != nil || f.restoreFail {
		return errors.New("restore failed")
	}
	f.partial = false
	f.restored = true
	return nil
}

func (f *fakeAdapter) VerifyOriginal(context.Context) error {
	f.calls = append(f.calls, "verify-original")
	if f.verifyFail || !f.restored || f.partial {
		return errors.New("original not ready")
	}
	return nil
}

func (f *fakeAdapter) Finish(context.Context) error {
	f.calls = append(f.calls, "finish")
	if f.fail == "finish" {
		return errors.New("commit failed")
	}
	return nil
}

func (f *fakeAdapter) Release() {
	f.calls = append(f.calls, "release")
	f.released = true
}

func TestRotate(t *testing.T) {
	ctx := context.Background()
	phases := []string{"stop-erp", "switch-scm", "switch-firewall", "switch-shortcut", "switch-receipt", "start-erp", "verify-readiness"}
	t.Run("commit-order", func(t *testing.T) {
		f := &fakeAdapter{}
		out, err := Rotate(ctx, ctx, f)
		want := append([]string{"prepare"}, phases...)
		want = append(want, "finish", "release")
		if out != Committed || err != nil || !reflect.DeepEqual(f.calls, want) || f.restored {
			t.Fatalf("unexpected outcome %s %v; calls %v", out, err, f.calls)
		}
	})
	t.Run("prepare-refusal", func(t *testing.T) {
		f := &fakeAdapter{fail: "prepare"}
		out, err := Rotate(ctx, ctx, f)
		if out != Refused || err == nil || !reflect.DeepEqual(f.calls, []string{"prepare"}) {
			t.Fatalf("unexpected refusal: %s %v %v", out, err, f.calls)
		}
	})
	for _, phase := range append(phases, "finish") {
		t.Run("rollback-"+phase, func(t *testing.T) {
			f := &fakeAdapter{fail: phase}
			out, err := Rotate(ctx, ctx, f)
			if out != RolledBack || err == nil || !f.restored || f.partial || !f.released ||
				strings.Contains(err.Error(), "private detail") {
				t.Fatalf("unexpected rollback: %s %v %v", out, err, f.calls)
			}
			if !reflect.DeepEqual(f.calls[len(f.calls)-3:], []string{"restore", "verify-original", "release"}) {
				t.Fatalf("incorrect recovery order: %v", f.calls)
			}
		})
	}
	t.Run("restore-failure", func(t *testing.T) {
		f := &fakeAdapter{fail: "switch-scm", restoreFail: true}
		out, err := Rotate(ctx, ctx, f)
		if out != RecoveryRequired || err == nil || !f.released || f.restored {
			t.Fatalf("unexpected recovery result: %s %v %v", out, err, f.calls)
		}
	})
	t.Run("original-not-ready", func(t *testing.T) {
		f := &fakeAdapter{fail: "verify-readiness", verifyFail: true}
		out, err := Rotate(ctx, ctx, f)
		if out != RecoveryRequired || err == nil || !f.released {
			t.Fatalf("unexpected recovery result: %s %v %v", out, err, f.calls)
		}
	})
	t.Run("cancelled-apply-still-recovers", func(t *testing.T) {
		apply, cancel := context.WithCancel(ctx)
		defer cancel()
		f := &fakeAdapter{cancel: cancel}
		out, err := Rotate(apply, ctx, f)
		if out != RolledBack || err == nil || !f.restored || !f.released {
			t.Fatalf("unexpected cancellation result: %s %v %v", out, err, f.calls)
		}
	})
}
