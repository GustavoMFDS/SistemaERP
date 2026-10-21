package main

// This standalone standard-library-only utility is compiled into the signed
// Windows installer bundle. It generates a NEW, host-local CA and PostgreSQL
// server certificate; the CA signing private key is never persisted.
// Windows ACLs must be restricted by the signed installer before execution.
import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func makeSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 159)
	return rand.Int(rand.Reader, limit)
}

func generateFiles(directory string) error {
	if !filepath.IsAbs(directory) {
		return errors.New("absolute local output directory required")
	}
	clean := filepath.Clean(directory)
	if strings.HasPrefix(clean, "\\\\") {
		return errors.New("network output path prohibited")
	}
	for p := clean; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			return fmt.Errorf("output ancestor unavailable: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("output path has a symbolic link or junction")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("output directory must be empty, refusing to replace files")
	}
	now := time.Now().UTC()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	caSerial, err := makeSerial()
	if err != nil {
		return err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "Esterium Local PostgreSQL Root"},
		NotBefore:             now.Add(-10 * time.Minute),
		NotAfter:              now.AddDate(2, 0, 0),
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDer, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caCertificate, err := x509.ParseCertificate(caDer)
	if err != nil {
		return err
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return err
	}
	serial, err := makeSerial()
	if err != nil {
		return err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now.Add(-10 * time.Minute),
		NotAfter:              now.AddDate(0, 10, 0),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:            []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	serverDer, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCertificate, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	serverBytes, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return err
	}
	files := []struct {
		name    string
		mode    os.FileMode
		payload []byte
	}{
		{"server.key", 0600, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverBytes})},
		{"server.crt", 0600, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDer})},
		{"root.crt", 0600, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDer})},
	}
	var created []string
	committed := false
	defer func() {
		if !committed {
			for _, p := range created {
				_ = os.Remove(p)
			}
		}
	}()
	for _, entry := range files {
		target := filepath.Join(clean, entry.name)
		fd, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, entry.mode)
		if err != nil {
			return fmt.Errorf("cannot create new protected certificate file: %w", err)
		}
		created = append(created, target)
		_, writeErr := fd.Write(entry.payload)
		syncErr := fd.Sync()
		closeErr := fd.Close()
		if writeErr != nil {
			return writeErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	committed = true
	return nil
}

func main() {
	out := flag.String("out-dir", "", "new empty protected directory, no network shares")
	flag.Parse()
	if flag.NArg() != 0 || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: esterium-pg-tls-certgen -out-dir ABSOLUTE_EMPTY_DIRECTORY")
		os.Exit(2)
	}
	if err := generateFiles(*out); err != nil {
		fmt.Fprintln(os.Stderr, "local certificate generation failed:", err)
		os.Exit(1)
	}
	fmt.Println("PASS: new localhost PostgreSQL server certificate, private key and local root created (root signing key discarded).")
}
