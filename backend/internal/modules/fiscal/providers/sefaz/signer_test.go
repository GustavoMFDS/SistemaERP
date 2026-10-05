package sefaz

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

func testECDSACertificate(t *testing.T, now time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(43),
		Subject:      pkix.Name{CommonName: "SistemaEmGo NFC-e ECDSA test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

func testRSACertificate(t *testing.T, now time.Time) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "SistemaEmGo NFC-e RSA test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

func directChild(parent *etree.Element, tag string) *etree.Element {
	if parent == nil {
		return nil
	}
	for _, child := range parent.ChildElements() {
		if child.Tag == tag {
			return child
		}
	}
	return nil
}

func TestSignNFCeXMLProducesVerifiableOfficialProfile(t *testing.T) {
	input := unsignedLegacyFixture(t)
	unsigned, err := BuildUnsignedNFCeLegacyCandidate(input)
	if err != nil {
		t.Fatal(err)
	}
	now := input.Reservation.IssuedAt
	cert := testRSACertificate(t, now)

	signed, err := SignNFCeXML(unsigned, cert, input.Reservation.AccessKey, now)
	if err != nil {
		t.Fatalf("SignNFCeXML: %v", err)
	}

	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(signed); err != nil {
		t.Fatalf("read signed XML: %v", err)
	}
	root := doc.Root()
	infNFe := directChild(root, "infNFe")
	signature := directChild(root, "Signature")
	supl := directChild(root, "infNFeSupl")
	if infNFe == nil || signature == nil || supl == nil {
		t.Fatalf("signed NFC-e structure incomplete: %s", signed)
	}
	if signature.Index() != infNFe.Index()+1 || supl.Index() <= signature.Index() {
		t.Fatalf("Signature must be between infNFe and infNFeSupl")
	}
	if signature.SelectAttrValue("xmlns", "") != dsig.Namespace {
		t.Fatalf("Signature namespace=%q", signature.SelectAttrValue("xmlns", ""))
	}

	signedInfo := directChild(signature, "SignedInfo")
	reference := directChild(signedInfo, "Reference")
	if reference == nil || reference.SelectAttrValue("URI", "") != "#NFe"+input.Reservation.AccessKey {
		t.Fatalf("unexpected Reference URI")
	}

	signatureMethod := directChild(signedInfo, "SignatureMethod")
	if signatureMethod == nil ||
		signatureMethod.SelectAttrValue("Algorithm", "") != rsaSHA1SignatureMethod {
		t.Fatalf("unexpected SignatureMethod")
	}

	digestMethod := directChild(reference, "DigestMethod")
	if digestMethod == nil ||
		digestMethod.SelectAttrValue("Algorithm", "") != "http://www.w3.org/2000/09/xmldsig#sha1" {
		t.Fatalf("unexpected DigestMethod")
	}

	canonicalizer := dsig.MakeC14N10RecCanonicalizer()
	canonicalInfNFe, err := canonicalizer.Canonicalize(infNFe)
	if err != nil {
		t.Fatalf("canonicalize infNFe: %v", err)
	}
	digest := sha1.Sum(canonicalInfNFe)
	digestValue := directChild(reference, "DigestValue")
	if digestValue == nil || strings.TrimSpace(digestValue.Text()) != base64.StdEncoding.EncodeToString(digest[:]) {
		t.Fatalf("DigestValue does not match canonical infNFe")
	}

	canonicalSignedInfo, err := canonicalizer.Canonicalize(signedInfo)
	if err != nil {
		t.Fatalf("canonicalize SignedInfo: %v", err)
	}
	signedInfoHash := sha1.Sum(canonicalSignedInfo)
	signatureValue := directChild(signature, "SignatureValue")
	rawSignature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureValue.Text()))
	if err != nil {
		t.Fatalf("decode SignatureValue: %v", err)
	}
	publicKey := cert.Leaf.PublicKey.(*rsa.PublicKey)
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA1, signedInfoHash[:], rawSignature); err != nil {
		t.Fatalf("RSA/SHA-1 SignatureValue verification failed: %v", err)
	}

	keyInfo := directChild(signature, "KeyInfo")
	x509Data := directChild(keyInfo, "X509Data")
	certificateElements := make([]*etree.Element, 0)
	for _, child := range x509Data.ChildElements() {
		if child.Tag == "X509Certificate" {
			certificateElements = append(certificateElements, child)
		}
	}
	if len(certificateElements) != 1 {
		t.Fatalf("X509Certificate count=%d, want 1", len(certificateElements))
	}
	if strings.TrimSpace(certificateElements[0].Text()) != base64.StdEncoding.EncodeToString(cert.Certificate[0]) {
		t.Fatal("embedded certificate does not match end-entity certificate")
	}
}

func TestSignNFCeXMLRejectsECDSAAndDuplicateSignature(t *testing.T) {
	input := unsignedLegacyFixture(t)
	unsigned, err := BuildUnsignedNFCeLegacyCandidate(input)
	if err != nil {
		t.Fatal(err)
	}
	now := input.Reservation.IssuedAt
	ecdsaCert := testECDSACertificate(t, now)
	if _, err := SignNFCeXML(unsigned, ecdsaCert, input.Reservation.AccessKey, now); err == nil ||
		!strings.Contains(err.Error(), "RSA") {
		t.Fatalf("expected ECDSA rejection, got %v", err)
	}

	rsaCert := testRSACertificate(t, now)
	signed, err := SignNFCeXML(unsigned, rsaCert, input.Reservation.AccessKey, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignNFCeXML(signed, rsaCert, input.Reservation.AccessKey, now); err == nil ||
		!strings.Contains(err.Error(), "already contains") {
		t.Fatalf("expected duplicate Signature rejection, got %v", err)
	}
}

func TestSignInutilizationXMLSignsInfInutByRequestID(t *testing.T) {
	unsigned, requestID, err := BuildUnsignedNFCeInutilization(InutilizationInput{
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
		t.Fatal(err)
	}
	now := time.Now()
	cert := testRSACertificate(t, now)
	signed, err := SignInutilizationXML(unsigned, cert, requestID, now)
	if err != nil {
		t.Fatalf("SignInutilizationXML: %v", err)
	}
	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(signed); err != nil {
		t.Fatal(err)
	}
	root := doc.Root()
	info := directChild(root, "infInut")
	signature := directChild(root, "Signature")
	if info == nil || signature == nil {
		t.Fatalf("signed inutilization structure incomplete: %s", signed)
	}
	if signature.Index() != info.Index()+1 {
		t.Fatal("Signature must follow infInut")
	}
	reference := directChild(directChild(signature, "SignedInfo"), "Reference")
	if reference == nil || reference.SelectAttrValue("URI", "") != "#"+requestID {
		t.Fatalf("unexpected inutilization signature reference")
	}
}
