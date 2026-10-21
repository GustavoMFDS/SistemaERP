package main

import (
 "crypto/rsa"
 "crypto/x509"
 "encoding/pem"
 "os"
 "path/filepath"
 "testing"
)

func parsePEM(t *testing.T, path, typ string) []byte {
 t.Helper()
 data,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 block,rest:=pem.Decode(data)
 if block==nil||block.Type!=typ||len(rest)!=0{t.Fatalf("invalid certificate PEM %s",filepath.Base(path))}
 return block.Bytes
}

func TestLocalTLSCertificate(t *testing.T){
 dir:=t.TempDir()
 if err:=generateFiles(dir);err!=nil{t.Fatal(err)}
 entries,err:=os.ReadDir(dir)
 if err!=nil{t.Fatal(err)}
 if len(entries)!=3{t.Fatalf("expected server.crt server.key root.crt, got %d files",len(entries))}
 root,err:=x509.ParseCertificate(parsePEM(t,filepath.Join(dir,"root.crt"),"CERTIFICATE"))
 if err!=nil{t.Fatal(err)}
 cert,err:=x509.ParseCertificate(parsePEM(t,filepath.Join(dir,"server.crt"),"CERTIFICATE"))
 if err!=nil{t.Fatal(err)}
 keyAny,err:=x509.ParsePKCS8PrivateKey(parsePEM(t,filepath.Join(dir,"server.key"),"PRIVATE KEY"))
 if err!=nil{t.Fatal(err)}
 key,ok:=keyAny.(*rsa.PrivateKey)
 if !ok{t.Fatal("server key must be RSA")}
 if cert.IsCA||!root.IsCA{t.Fatal("incorrect certificate authority roles")}
 if cert.PublicKey.(*rsa.PublicKey).N.Cmp(key.PublicKey.N)!=0{t.Fatal("server private key does not match certificate")}
 pool:=x509.NewCertPool()
 pool.AddCert(root)
 for _,name:=range []string{"localhost","127.0.0.1","::1"}{
  if _,err:=cert.Verify(x509.VerifyOptions{Roots:pool,DNSName:name,KeyUsages:[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}});err!=nil{
   t.Fatalf("certificate must validate for %s: %v",name,err)
  }
 }
 if _,err:=cert.Verify(x509.VerifyOptions{Roots:pool,DNSName:"other-host"});err==nil{
  t.Fatal("certificate unexpectedly valid for LAN host")
 }
 if _,err:=cert.Verify(x509.VerifyOptions{Roots:pool,KeyUsages:[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}});err==nil{
  t.Fatal("server certificate must not work as client authentication identity")
 }
 before,err:=os.ReadFile(filepath.Join(dir,"server.key"))
 if err!=nil{t.Fatal(err)}
 if err:=generateFiles(dir);err==nil{t.Fatal("must reject overwrite of existing certs")}
 after,err:=os.ReadFile(filepath.Join(dir,"server.key"))
 if err!=nil{t.Fatal(err)}
 if string(before)!=string(after){t.Fatal("refused overwrite modified existing private key")}
}

func TestCertGenRefusesUnsafeDestinations(t *testing.T){
 root:=t.TempDir()
 if err:=generateFiles("relative");err==nil{t.Fatal("relative dir accepted")}
 if err:=os.WriteFile(filepath.Join(root,"important.txt"),[]byte("do not touch"),0600);err!=nil{t.Fatal(err)}
 if err:=generateFiles(root);err==nil{t.Fatal("nonempty dir accepted")}
 data,err:=os.ReadFile(filepath.Join(root,"important.txt"))
 if err!=nil||string(data)!="do not touch"{t.Fatal("nonempty dir modified")}
 if err:=generateFiles(filepath.Join(root,"missing"));err==nil{t.Fatal("missing destination accepted")}
}
