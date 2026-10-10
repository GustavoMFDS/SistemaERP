package sefaz

import (
	"context"
	"strings"
	"testing"
)

func TestHomologationAuthorizerRejectsProductionBeforeNetwork(t *testing.T) {
	authorizer := NewHomologationAuthorizer(nil, 0)
	_, err := authorizer.Authorize(
		context.Background(),
		"secret://cert",
		"MG",
		"production",
		testAccessKey,
		1,
		[]byte("<NFe/>"),
	)
	if err == nil || !strings.Contains(err.Error(), "restricted to homologation") {
		t.Fatalf("expected production rejection, got %v", err)
	}
}

func TestProductionAuthorizerRejectsHomologationBeforeNetwork(t *testing.T) {
	authorizer := NewProductionAuthorizer(nil, 0)
	_, err := authorizer.Authorize(
		context.Background(),
		"secret://cert",
		"MG",
		"homologation",
		testAccessKey,
		1,
		[]byte("<NFe/>"),
	)
	if err == nil || !strings.Contains(err.Error(), "restricted to production") {
		t.Fatalf("expected homologation rejection, got %v", err)
	}
}

func TestAuthorizationOutcomeMapsPendingAuthorizedAndRejected(t *testing.T) {
	pending, err := authorizationOutcome(testAccessKey, AuthorizationResponse{
		StatusCode: 103,
		Reason:     "Lote recebido com sucesso",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pending.Pending() || pending.StatusCode != 103 {
		t.Fatalf("unexpected pending outcome: %+v", pending)
	}

	authorized, err := authorizationOutcome(testAccessKey, AuthorizationResponse{
		StatusCode: 104,
		Reason:     "Lote processado",
		Protocol: &Protocol{
			AccessKey:  testAccessKey,
			ReceivedAt: "2026-10-01T00:30:00-03:00",
			Protocol:   "131260000000001",
			StatusCode: 100,
			Reason:     "Autorizado o uso da NF-e",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !authorized.Authorized() || authorized.Protocol != "131260000000001" ||
		authorized.ReceivedAt.IsZero() {
		t.Fatalf("unexpected authorized outcome: %+v", authorized)
	}

	rejected, err := authorizationOutcome(testAccessKey, AuthorizationResponse{
		StatusCode: 104,
		Reason:     "Lote processado",
		Protocol: &Protocol{
			AccessKey:  testAccessKey,
			StatusCode: 539,
			Reason:     "Duplicidade de NF-e",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rejected.Rejected() || rejected.StatusCode != 539 {
		t.Fatalf("unexpected rejected outcome: %+v", rejected)
	}
}

func TestConsultationOutcomeMapsMissingProtocolAsPending(t *testing.T) {
	outcome, err := consultationOutcome(testAccessKey, ConsultationResponse{
		StatusCode: 217,
		Reason:     "NF-e nao consta na base de dados",
		AccessKey:  testAccessKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Pending() || outcome.StatusCode != 217 {
		t.Fatalf("unexpected consultation outcome: %+v", outcome)
	}
}

func TestAuthorizationOutcomeRejectsMismatchedKey(t *testing.T) {
	_, err := authorizationOutcome(testAccessKey, AuthorizationResponse{
		StatusCode: 104,
		Protocol: &Protocol{
			AccessKey:  "31260912ABC34501DE35650010000000421123456789",
			StatusCode: 100,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected access-key mismatch, got %v", err)
	}
}

func TestAuthorizedSEFAZOutcomeRequiresGenuineProtocolAndAccessKey(t *testing.T) {
	for _, build := range []func() Protocol{
		func() Protocol {
			return Protocol{StatusCode: 100, AccessKey: testAccessKey,
				ReceivedAt: "2026-10-01T00:30:00-03:00"}
		},
		func() Protocol {
			return Protocol{StatusCode: 100, Protocol: "131260000000001",
				ReceivedAt: "2026-10-01T00:30:00-03:00"}
		},
		func() Protocol {
			return Protocol{StatusCode: 100, AccessKey: testAccessKey,
				Protocol: "131260000000001"}
		},
	} {
		proto := build()
		if result, err := authorizationOutcome(testAccessKey, AuthorizationResponse{
			StatusCode: 104, Protocol: &proto,
		}); err == nil || result.Authorized() {
			t.Fatalf("malformed authorization erroneously accepted: result=%+v err=%v", result, err)
		}
		if result, err := consultationOutcome(testAccessKey, ConsultationResponse{
			StatusCode: 100, AccessKey: testAccessKey, Protocol: &proto,
		}); err == nil || result.Authorized() {
			t.Fatalf("malformed consultation erroneously accepted: result=%+v err=%v", result, err)
		}
	}
}
