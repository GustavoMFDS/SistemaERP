package sefaz

import (
 "bytes"
 "encoding/xml"
 "strings"
 "testing"
 "time"
)

func authenticTestProtocol(t *testing.T, signed []byte, key string, env string, at time.Time) []byte {
 t.Helper()
 var d signedDocumentForProc
 if err:=xml.Unmarshal(signed,&d);err!=nil {t.Fatal(err)}
 return []byte(`<protNFe versao="4.00"><infProt><tpAmb>`+env+
 `</tpAmb><verAplic>MG-test.4.00</verAplic><chNFe>`+key+
 `</chNFe><dhRecbto>`+at.Format(time.RFC3339)+
 `</dhRecbto><nProt>131260000000001</nProt><digVal>`+
 d.Signature.SignedInfo.Reference.Digest+
 `</digVal><cStat>100</cStat><xMotivo>Autorizado o uso da NF-e</xMotivo></infProt></protNFe>`)
}

func TestBuildAuthorizedNFeProcPreservesSignedXMLAndGenuineProtocol(t *testing.T) {
 input:=unsignedLegacyFixture(t)
 unsigned,err:=BuildUnsignedNFCeLegacyCandidate(input)
 if err!=nil {t.Fatal(err)}
 cert:=testRSACertificate(t,input.Reservation.IssuedAt)
 signed,err:=SignNFCeXML(unsigned,cert,input.Reservation.AccessKey,input.Reservation.IssuedAt)
 if err!=nil {t.Fatal(err)}
 at:=input.Reservation.IssuedAt.Add(90*time.Second)
 p:=authenticTestProtocol(t,signed,input.Reservation.AccessKey,"2",at)
 soap:=[]byte(`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"><soap:Body><retEnviNFe xmlns="http://www.portalfiscal.inf.br/nfe"><protNFe versao="4.00">`)
 soap=append(soap,p[len(`<protNFe versao="4.00">`):]...)
 soap=append(soap,[]byte(`</retEnviNFe></soap:Body></soap:Envelope>`)...)
 captured,err:=ExtractProtocolXML(soap)
 if err!=nil {t.Fatal(err)}
 if !bytes.Equal(captured,p) {t.Fatalf("SEFAZ protocol bytes changed")}
 result,err:=BuildAuthorizedNFeProc(signed,captured,input.Reservation.AccessKey,"131260000000001",at)
 if err!=nil {t.Fatal(err)}
 if !bytes.Contains(result,bytes.TrimSpace(stripXMLHeader(signed))) || !bytes.Contains(result,p) {
  t.Fatal("nfeProc did not preserve signed XML or original SEFAZ protocol")
 }
 if !bytes.HasPrefix(result,[]byte(xml.Header+`<nfeProc xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">`)) {
  t.Fatal("wrong nfeProc root/version")
 }
}

func TestBuildAuthorizedNFeProcFailsClosedOnMismatches(t *testing.T) {
 input:=unsignedLegacyFixture(t)
 unsigned,err:=BuildUnsignedNFCeLegacyCandidate(input);if err!=nil {t.Fatal(err)}
 cert:=testRSACertificate(t,input.Reservation.IssuedAt)
 signed,err:=SignNFCeXML(unsigned,cert,input.Reservation.AccessKey,input.Reservation.IssuedAt);if err!=nil {t.Fatal(err)}
 at:=input.Reservation.IssuedAt.Add(time.Minute)
 valid:=authenticTestProtocol(t,signed,input.Reservation.AccessKey,"2",at)
 cases:=[]struct{name string; signed,proto []byte; protocol string; at time.Time}{
  {"unsigned",unsigned,valid,"131260000000001",at},
  {"missing_protocol",signed,nil,"131260000000001",at},
  {"wrong_environment",signed,bytes.Replace(valid,[]byte("<tpAmb>2"),[]byte("<tpAmb>1"),1),"131260000000001",at},
  {"wrong_key",signed,bytes.Replace(valid,[]byte(input.Reservation.AccessKey),[]byte(testAccessKey),1),"131260000000001",at},
  {"wrong_digest",signed,bytes.Replace(valid,[]byte("<digVal>"),[]byte("<digVal>AAAA"),1),"131260000000001",at},
  {"wrong_status",signed,bytes.Replace(valid,[]byte("<cStat>100"),[]byte("<cStat>539"),1),"131260000000001",at},
  {"wrong_number",signed,valid,"131260000000999",at},
  {"wrong_receipt_time",signed,valid,"131260000000001",at.Add(time.Second)},
  {"malformed_xml",signed,[]byte("<protNFe>"),"131260000000001",at},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   if result,err:=BuildAuthorizedNFeProc(tc.signed,tc.proto,input.Reservation.AccessKey,tc.protocol,tc.at);
    err==nil {t.Fatalf("invalid processed document accepted: %q",result)}
  })
 }
 if _,err:=ExtractProtocolXML([]byte(`<root xmlns="http://www.portalfiscal.inf.br/nfe"><protNFe/><protNFe/></root>`));
  err==nil || !strings.Contains(err.Error(),"multiple") { t.Fatalf("duplicate SEFAZ protocols accepted: %v",err) }
}
