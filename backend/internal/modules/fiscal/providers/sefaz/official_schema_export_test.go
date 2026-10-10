package sefaz

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

// TestExportOfficialNFCeSchemaFixtures is opt-in and uses fictitious taxpayer
// details plus an ephemeral RSA certificate. It does NOT contact the SEFAZ.
// It feeds scripts/fiscal/validate_official_nfce.py with real builder output.
func TestExportOfficialNFCeSchemaFixtures(t *testing.T) {
	dir := os.Getenv("NFCE_OFFICIAL_VALIDATION_EXPORT_DIR")
	if dir == "" {
		t.Skip("set NFCE_OFFICIAL_VALIDATION_EXPORT_DIR for the independent official XSD check")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatal("validation export directory must already exist")
	}
	input := unsignedLegacyFixture(t)
	now := input.Reservation.IssuedAt
	cert := testRSACertificate(t, now)

	export := func(name string, input UnsignedNFCeInput) {
		t.Helper()
		unsigned, err := BuildUnsignedNFCeLegacyCandidate(input)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := SignNFCeXML(unsigned, cert, input.Reservation.AccessKey, now)
		if err != nil {
			t.Fatal(err)
		}
		// The fixture deliberately uses a synthetic protNFe. Its purpose is
		// to prove the 2026 XSD structure, NOT a real SEFAZ authorization.
		protocolTime := input.Reservation.IssuedAt.Add(90 * time.Second)
		protocolXML := authenticTestProtocol(
			t, signed, input.Reservation.AccessKey, "2", protocolTime,
		)
		processed, err := BuildAuthorizedNFeProc(
			signed, protocolXML, input.Reservation.AccessKey,
			"131260000000001", protocolTime,
		)
		if err != nil {
			t.Fatal(err)
		}
		for filename, content := range map[string][]byte{
			"nfce-" + name + "-unsigned.xml": unsigned,
			"nfce-" + name + "-signed.xml":   signed,
			"nfce-" + name + "-proc.xml":     processed,
		} {
			path := filepath.Join(dir, filename)
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	export("legacy", input)

	ibsUF, err := fisc.ParseTaxRate("0.1000")
	if err != nil {
		t.Fatal(err)
	}
	ibsMun, err := fisc.ParseTaxRate("0.0000")
	if err != nil {
		t.Fatal(err)
	}
	cbs, err := fisc.ParseTaxRate("0.9000")
	if err != nil {
		t.Fatal(err)
	}
	rtc, err := fisc.CalculateRegularIBSCBS(fisc.RegularIBSCBSInput{
		CST: "000", Classification: "000001",
		Base:         platform.NewMoneyCents(900),
		IBSUF:        fisc.RegularTaxComponentInput{Rate: ibsUF},
		IBSMunicipal: fisc.RegularTaxComponentInput{Rate: ibsMun},
		CBS:          fisc.RegularTaxComponentInput{Rate: cbs},
	})
	if err != nil {
		t.Fatal(err)
	}
	input.Items[0].Tax.RTCTax.IBSCBS = &rtc
	export("rtc", input)
}
