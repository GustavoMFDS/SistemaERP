package sefaz

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

func TestSignNFCeXMLProducesValidNFESignature(t *testing.T) {
	input := unsignedLegacyFixture(t)
	unsignedXML, err := BuildUnsignedNFCeLegacyCandidate(input)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 1, 1, 0, 0, 0, time.UTC)
	cert := testCertificate(t, now.Add(-time.Hour), now.Add(24*time.Hour))

	signedXML, err := SignNFCeXML(unsignedXML, cert)
	if err != nil {
		t.Fatalf("SignNFCeXML: %v", err)
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(signedXML); err != nil {
		t.Fatalf("parse signed XML: %v", err)
	}
	root := doc.Root()
	if root == nil {
		t.Fatal("missing NFe root")
	}
	infNFe := root.FindElement("./infNFe")
	signature := root.FindElement("./Signature")
	supl := root.FindElement("./infNFeSupl")
	if infNFe == nil || signature == nil || supl == nil {
		t.Fatalf("signed XML structure incomplete")
	}

	signatureIndex := -1
	suplIndex := -1
	for i, child := range root.ChildElements() {
		switch child.Tag {
		case "Signature":
			signatureIndex = i
		case "infNFeSupl":
			suplIndex = i
		}
	}
	if signatureIndex < 0 || suplIndex < 0 || signatureIndex > suplIndex {
		t.Fatalf("Signature must be before infNFeSupl")
	}

	signedInfo := signature.FindElement("./SignedInfo")
	if signedInfo == nil {
		t.Fatal("SignedInfo missing")
	}
	canonicalization := signedInfo.FindElement("./CanonicalizationMethod")
	signatureMethod := signedInfo.FindElement("./SignatureMethod")
	reference := signedInfo.FindElement("./Reference")
	if canonicalization == nil || signatureMethod == nil || reference == nil {
		t.Fatal("SignedInfo metadata missing")
	}
	if canonicalization.SelectAttrValue("Algorithm", "") !=
		"http://www.w3.org/TR/2001/REC-xml-c14n-20010315" {
		t.Fatalf("unexpected canonicalization algorithm")
	}
	if signatureMethod.SelectAttrValue("Algorithm", "") != dsig.RSASHA1SignatureMethod {
		t.Fatalf("unexpected signature method")
	}
	if reference.SelectAttrValue("URI", "") != "#"+infNFe.SelectAttrValue("Id", "") {
		t.Fatalf("Reference URI does not target infNFe")
	}

	digestValue := reference.FindElement("./DigestValue")
	if digestValue == nil {
		t.Fatal("DigestValue missing")
	}
	c14n := dsig.MakeC14N10RecCanonicalizer()
	canonicalInf, err := c14n.Canonicalize(infNFe)
	if err != nil {
		t.Fatalf("canonicalize infNFe: %v", err)
	}
	digest := sha1.Sum(canonicalInf)
	wantDigest := base64.StdEncoding.EncodeToString(digest[:])
	if strings.TrimSpace(digestValue.Text()) != wantDigest {
		t.Fatalf("DigestValue mismatch: got %s want %s", digestValue.Text(), wantDigest)
	}

	canonicalSignedInfo, err := c14n.Canonicalize(signedInfo)
	if err != nil {
		t.Fatalf("canonicalize SignedInfo: %v", err)
	}
	signedInfoHash := sha1.Sum(canonicalSignedInfo)
	signatureValue := signature.FindElement("./SignatureValue")
	if signatureValue == nil {
		t.Fatal("SignatureValue missing")
	}
	rawSignature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureValue.Text()))
	if err != nil {
		t.Fatalf("decode SignatureValue: %v", err)
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	publicKey, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatal("certificate public key is not RSA")
	}
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA1, signedInfoHash[:], rawSignature); err != nil {
		t.Fatalf("verify XMLDSig signature: %v", err)
	}

	x509Element := signature.FindElement("./KeyInfo/X509Data/X509Certificate")
	if x509Element == nil {
		t.Fatal("X509Certificate missing")
	}
	embedded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(x509Element.Text()))
	if err != nil {
		t.Fatal(err)
	}
	if base64.StdEncoding.EncodeToString(embedded) != base64.StdEncoding.EncodeToString(cert.Certificate[0]) {
		t.Fatal("embedded certificate is not the end certificate used for signing")
	}
}

func TestSignNFCeXMLRejectsDuplicateSignature(t *testing.T) {
	input := unsignedLegacyFixture(t)
	unsignedXML, err := BuildUnsignedNFCeLegacyCandidate(input)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cert := testCertificate(t, now.Add(-time.Hour), now.Add(time.Hour))
	first, err := SignNFCeXML(unsignedXML, cert)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignNFCeXML(first, cert); err == nil {
		t.Fatal("expected duplicate signature to be rejected")
	}
}
