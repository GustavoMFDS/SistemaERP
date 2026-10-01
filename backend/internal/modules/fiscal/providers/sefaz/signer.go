package sefaz

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
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
	if _, err := doc.ReadFromBytes(bytes.TrimSpace(unsignedXML)); err != nil {
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
