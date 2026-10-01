package sefaz

import (
	"bytes"
	"crypto"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

func SignNFCeXML(unsignedXML []byte, cert tls.Certificate) ([]byte, error) {
	if err := ValidateClientCertificate(cert, zeroTime()); err != nil {
		return nil, err
	}
	signer, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("client certificate private key does not implement crypto.Signer")
	}
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("client certificate chain is empty")
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(unsignedXML); err != nil {
		return nil, fmt.Errorf("parse NFC-e XML: %w", err)
	}
	root := doc.Root()
	if root == nil || root.Tag != "NFe" {
		return nil, fmt.Errorf("root element must be NFe")
	}
	infNFe := root.FindElement("./infNFe")
	if infNFe == nil {
		return nil, fmt.Errorf("infNFe element not found")
	}
	id := strings.TrimSpace(infNFe.SelectAttrValue("Id", ""))
	if id == "" || !strings.HasPrefix(id, "NFe") {
		return nil, fmt.Errorf("infNFe Id is missing or invalid")
	}
	if err := validateExpectedAccessKey(strings.TrimPrefix(id, "NFe")); err != nil {
		return nil, err
	}
	if existing := root.FindElement("./Signature"); existing != nil {
		return nil, fmt.Errorf("NFC-e XML already contains a Signature")
	}

	ctx, err := dsig.NewSigningContext(signer, [][]byte{cert.Certificate[0]})
	if err != nil {
		return nil, fmt.Errorf("create XMLDSig context: %w", err)
	}
	ctx.Canonicalizer = dsig.MakeC14N10RecCanonicalizer()
	ctx.IdAttribute = "Id"
	ctx.Prefix = ""
	if err := ctx.SetSignatureMethod(dsig.RSASHA1SignatureMethod); err != nil {
		return nil, fmt.Errorf("configure XMLDSig signature method: %w", err)
	}

	signature, err := ctx.ConstructSignature(infNFe, true)
	if err != nil {
		return nil, fmt.Errorf("construct NFC-e XML signature: %w", err)
	}

	insertIndex := len(root.Child)
	for i, child := range root.Child {
		if element, ok := child.(*etree.Element); ok && element.Tag == "infNFeSupl" {
			insertIndex = i
			break
		}
	}
	root.InsertChildAt(insertIndex, signature)

	doc.WriteSettings.CanonicalAttrVal = true
	content, err := doc.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serialize signed NFC-e XML: %w", err)
	}
	content = bytes.TrimSpace(content)
	if !bytes.HasPrefix(content, []byte("<?xml")) {
		content = append([]byte(xmlHeaderUTF8()), content...)
	}
	return content, nil
}

func zeroTime() time.Time {
	return time.Time{}
}

func xmlHeaderUTF8() string {
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
}
