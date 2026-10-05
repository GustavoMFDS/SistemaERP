package sefaz

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

const rsaSHA1SignatureMethod = "http://www.w3.org/2000/09/xmldsig#rsa-sha1"

func SignNFCeXML(
	unsignedXML []byte,
	cert tls.Certificate,
	expectedAccessKey string,
	now time.Time,
) ([]byte, error) {
	if err := ValidateClientCertificate(cert, now); err != nil {
		return nil, err
	}
	if err := validateExpectedAccessKey(expectedAccessKey); err != nil {
		return nil, err
	}
	privateKey, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("NF-e/NFC-e XMLDSig requires an RSA private key")
	}
	leaf := cert.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
		leaf = parsed
	}
	publicKey, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("NF-e/NFC-e certificate must contain an RSA public key")
	}
	if privateKey.PublicKey.N.Cmp(publicKey.N) != 0 || privateKey.PublicKey.E != publicKey.E {
		return nil, fmt.Errorf("certificate public key does not match private key")
	}

	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(bytes.TrimSpace(unsignedXML)); err != nil {
		return nil, fmt.Errorf("parse unsigned NFC-e XML: %w", err)
	}
	root := doc.Root()
	if root == nil || root.Tag != "NFe" {
		return nil, fmt.Errorf("unsigned document root must be NFe")
	}

	var infNFe, infNFeSupl *etree.Element
	for _, child := range root.ChildElements() {
		switch child.Tag {
		case "infNFe":
			if infNFe != nil {
				return nil, fmt.Errorf("NFe contains multiple infNFe elements")
			}
			infNFe = child
		case "Signature":
			return nil, fmt.Errorf("NFe already contains a Signature")
		case "infNFeSupl":
			infNFeSupl = child
		}
	}
	if infNFe == nil {
		return nil, fmt.Errorf("infNFe is missing")
	}
	wantID := "NFe" + expectedAccessKey
	if got := infNFe.SelectAttrValue("Id", ""); got != wantID {
		return nil, fmt.Errorf("infNFe Id=%q, want %q", got, wantID)
	}

	signingContext, err := dsig.NewSigningContext(
		privateKey,
		[][]byte{cert.Certificate[0]},
	)
	if err != nil {
		return nil, fmt.Errorf("create XMLDSig context: %w", err)
	}
	signingContext.IdAttribute = "Id"
	signingContext.Prefix = ""
	signingContext.Canonicalizer = dsig.MakeC14N10RecCanonicalizer()
	if err := signingContext.SetSignatureMethod(rsaSHA1SignatureMethod); err != nil {
		return nil, fmt.Errorf("configure NF-e signature method: %w", err)
	}

	signature, err := signingContext.ConstructSignature(infNFe, true)
	if err != nil {
		return nil, fmt.Errorf("construct NFC-e XMLDSig: %w", err)
	}

	insertAt := infNFe.Index() + 1
	if infNFeSupl != nil && infNFeSupl.Index() < insertAt {
		return nil, fmt.Errorf("infNFeSupl appears before infNFe")
	}
	root.InsertChildAt(insertAt, signature)

	signed, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize signed NFC-e: %w", err)
	}
	return signed, nil
}

func SignCancellationEventXML(
	unsignedEventXML []byte,
	cert tls.Certificate,
	expectedEventID string,
	now time.Time,
) ([]byte, error) {
	if err := ValidateClientCertificate(cert, now); err != nil {
		return nil, err
	}
	expectedEventID = strings.TrimSpace(expectedEventID)
	if len(expectedEventID) != 54 ||
		!strings.HasPrefix(expectedEventID, "ID"+CancellationEventType) {
		return nil, fmt.Errorf("invalid cancellation event Id")
	}
	accessKey := expectedEventID[8:52]
	if err := validateExpectedAccessKey(accessKey); err != nil {
		return nil, fmt.Errorf("event Id access key: %w", err)
	}

	privateKey, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("NF-e/NFC-e XMLDSig requires an RSA private key")
	}
	leaf := cert.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
		leaf = parsed
	}
	publicKey, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("NF-e/NFC-e certificate must contain an RSA public key")
	}
	if privateKey.PublicKey.N.Cmp(publicKey.N) != 0 || privateKey.PublicKey.E != publicKey.E {
		return nil, fmt.Errorf("certificate public key does not match private key")
	}

	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(bytes.TrimSpace(unsignedEventXML)); err != nil {
		return nil, fmt.Errorf("parse unsigned cancellation event XML: %w", err)
	}
	root := doc.Root()
	if root == nil || root.Tag != "evento" {
		return nil, fmt.Errorf("unsigned event root must be evento")
	}

	var infEvento *etree.Element
	for _, child := range root.ChildElements() {
		switch child.Tag {
		case "infEvento":
			if infEvento != nil {
				return nil, fmt.Errorf("evento contains multiple infEvento elements")
			}
			infEvento = child
		case "Signature":
			return nil, fmt.Errorf("evento already contains a Signature")
		}
	}
	if infEvento == nil {
		return nil, fmt.Errorf("infEvento is missing")
	}
	if got := infEvento.SelectAttrValue("Id", ""); got != expectedEventID {
		return nil, fmt.Errorf("infEvento Id=%q, want %q", got, expectedEventID)
	}

	signingContext, err := dsig.NewSigningContext(
		privateKey,
		[][]byte{cert.Certificate[0]},
	)
	if err != nil {
		return nil, fmt.Errorf("create XMLDSig context: %w", err)
	}
	signingContext.IdAttribute = "Id"
	signingContext.Prefix = ""
	signingContext.Canonicalizer = dsig.MakeC14N10RecCanonicalizer()
	if err := signingContext.SetSignatureMethod(rsaSHA1SignatureMethod); err != nil {
		return nil, fmt.Errorf("configure NF-e signature method: %w", err)
	}

	signature, err := signingContext.ConstructSignature(infEvento, true)
	if err != nil {
		return nil, fmt.Errorf("construct cancellation event XMLDSig: %w", err)
	}
	root.InsertChildAt(infEvento.Index()+1, signature)

	signed, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize signed cancellation event: %w", err)
	}
	return signed, nil
}

func SignInutilizationXML(
	unsignedXML []byte,
	cert tls.Certificate,
	expectedRequestID string,
	now time.Time,
) ([]byte, error) {
	if err := ValidateClientCertificate(cert, now); err != nil {
		return nil, err
	}
	expectedRequestID = strings.TrimSpace(expectedRequestID)
	if len(expectedRequestID) != 43 || !strings.HasPrefix(expectedRequestID, "ID") {
		return nil, fmt.Errorf("invalid inutilization request Id")
	}

	privateKey, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("NF-e/NFC-e XMLDSig requires an RSA private key")
	}
	leaf := cert.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("parse client certificate: %w", err)
		}
		leaf = parsed
	}
	publicKey, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("NF-e/NFC-e certificate must contain an RSA public key")
	}
	if privateKey.PublicKey.N.Cmp(publicKey.N) != 0 || privateKey.PublicKey.E != publicKey.E {
		return nil, fmt.Errorf("certificate public key does not match private key")
	}

	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	if err := doc.ReadFromBytes(bytes.TrimSpace(unsignedXML)); err != nil {
		return nil, fmt.Errorf("parse unsigned inutilization XML: %w", err)
	}
	root := doc.Root()
	if root == nil || root.Tag != "inutNFe" {
		return nil, fmt.Errorf("unsigned inutilization root must be inutNFe")
	}

	var infInut *etree.Element
	for _, child := range root.ChildElements() {
		switch child.Tag {
		case "infInut":
			if infInut != nil {
				return nil, fmt.Errorf("inutNFe contains multiple infInut elements")
			}
			infInut = child
		case "Signature":
			return nil, fmt.Errorf("inutNFe already contains a Signature")
		}
	}
	if infInut == nil {
		return nil, fmt.Errorf("infInut is missing")
	}
	if got := infInut.SelectAttrValue("Id", ""); got != expectedRequestID {
		return nil, fmt.Errorf("infInut Id=%q, want %q", got, expectedRequestID)
	}

	signingContext, err := dsig.NewSigningContext(
		privateKey,
		[][]byte{cert.Certificate[0]},
	)
	if err != nil {
		return nil, fmt.Errorf("create XMLDSig context: %w", err)
	}
	signingContext.IdAttribute = "Id"
	signingContext.Prefix = ""
	signingContext.Canonicalizer = dsig.MakeC14N10RecCanonicalizer()
	if err := signingContext.SetSignatureMethod(rsaSHA1SignatureMethod); err != nil {
		return nil, fmt.Errorf("configure NF-e signature method: %w", err)
	}

	signature, err := signingContext.ConstructSignature(infInut, true)
	if err != nil {
		return nil, fmt.Errorf("construct inutilization XMLDSig: %w", err)
	}
	root.InsertChildAt(infInut.Index()+1, signature)

	signed, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize signed inutilization XML: %w", err)
	}
	return signed, nil
}

func SignOfflineQRCodeV3(
	payload string,
	cert tls.Certificate,
	now time.Time,
) (string, error) {
	if err := ValidateClientCertificate(cert, now); err != nil {
		return "", err
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", fmt.Errorf("offline QR Code payload is required")
	}
	privateKey, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("offline QR Code signing requires an RSA private key")
	}
	digest := sha1.Sum([]byte(payload))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA1, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign offline QR Code: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

