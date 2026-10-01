package sefaz

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultPEMSecretFileLimit = 1 << 20

var secretRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type PEMDirectoryCertificateResolver struct {
	baseDir string
	maxBytes int64
}

func NewPEMDirectoryCertificateResolver(baseDir string) (*PEMDirectoryCertificateResolver, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("certificate secret directory is required")
	}
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve certificate secret directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat certificate secret directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("certificate secret path is not a directory")
	}
	return &PEMDirectoryCertificateResolver{baseDir: abs, maxBytes: defaultPEMSecretFileLimit}, nil
}

func (r *PEMDirectoryCertificateResolver) ResolveClientCertificate(
	ctx context.Context,
	secretRef string,
) (tls.Certificate, error) {
	if err := ctx.Err(); err != nil {
		return tls.Certificate{}, err
	}
	secretRef = strings.TrimSpace(secretRef)
	if !secretRefPattern.MatchString(secretRef) || strings.Contains(secretRef, "..") {
		return tls.Certificate{}, fmt.Errorf("invalid certificate secret reference")
	}

	certPEM, err := readSecretFile(
		ctx,
		filepath.Join(r.baseDir, secretRef+".crt.pem"),
		r.maxBytes,
	)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("read certificate PEM: %w", err)
	}
	keyPEM, err := readSecretFile(
		ctx,
		filepath.Join(r.baseDir, secretRef+".key.pem"),
		r.maxBytes,
	)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("read private-key PEM: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load certificate key pair: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, fmt.Errorf("certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse certificate leaf: %w", err)
	}
	cert.Leaf = leaf
	return cert, nil
}

func readSecretFile(ctx context.Context, path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = defaultPEMSecretFileLimit
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("secret path is not a regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("secret file exceeds %d bytes", limit)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("secret file exceeds %d bytes", limit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return content, nil
}

type XMLSigningService struct {
	resolver CertificateResolver
}

func NewXMLSigningService(resolver CertificateResolver) *XMLSigningService {
	return &XMLSigningService{resolver: resolver}
}

func (s *XMLSigningService) Sign(
	ctx context.Context,
	secretRef string,
	expectedAccessKey string,
	unsignedXML []byte,
) ([]byte, error) {
	cert, err := ResolveAndValidateCertificate(ctx, s.resolver, secretRef, timeNowUTC())
	if err != nil {
		return nil, err
	}
	return SignNFCeXML(unsignedXML, cert, expectedAccessKey, timeNowUTC())
}

var timeNowUTC = func() time.Time {
	return time.Now().UTC()
}
