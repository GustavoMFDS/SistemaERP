package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/redis/go-redis/v9"
)

func TestPinnedLocalGarnetCertificate(t *testing.T) {
	now := time.Now()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(19),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "garnet-local.cer")
	if err := os.WriteFile(file, der, 0600); err != nil {
		t.Fatal(err)
	}
	url := "rediss://esteriumcache:synthetic-password@localhost:6379/0"
	options, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := configureRedisTLS(config.Config{RedisURL: url, RedisCACertFile: file}, options); err != nil {
		t.Fatal(err)
	}
	if options.TLSConfig == nil || options.TLSConfig.RootCAs == nil ||
		options.TLSConfig.ServerName != "localhost" ||
		options.TLSConfig.MinVersion < tls.VersionTLS12 || options.TLSConfig.InsecureSkipVerify {
		t.Fatal("local Garnet certificate trust is not bound to verified TLS identity")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{
		Roots: options.TLSConfig.RootCAs, DNSName: "localhost",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{
		Roots: options.TLSConfig.RootCAs, DNSName: "remote.example",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err == nil {
		t.Fatal("pinned certificate must not trust another server hostname")
	}
	pemFile := filepath.Join(dir, "garnet-local.pem")
	if err := os.WriteFile(pemFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := configureRedisTLS(config.Config{RedisURL: url, RedisCACertFile: pemFile}, options); err != nil {
		t.Fatal("PEM version of same certificate should validate: ", err)
	}
	if err := configureRedisTLS(config.Config{RedisURL: "redis://localhost:6379", RedisCACertFile: file}, options); err == nil {
		t.Fatal("plaintext Redis URL with a TLS trust file must be rejected")
	}
	if err := os.WriteFile(pemFile, []byte("not-a-certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := configureRedisTLS(config.Config{RedisURL: url, RedisCACertFile: pemFile}, options); err == nil {
		t.Fatal("malformed Garnet local trust must reject startup")
	}
}
