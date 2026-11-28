// Command lan-tls-preflight checks a proposed local HTTPS identity without
// changing any operating system resource. Inputs must be protected by the
// calling maintenance adapter; this command never logs their contents.
package main

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/lanrotation"
)

func readInput(path string) ([]byte, error) {
	const limit = 1024 * 1024
	if !filepath.IsAbs(path) {
		return nil, errors.New("absolute input path required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("input unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || info.Size() == 0 {
		return nil, errors.New("invalid input file")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(b) > limit || len(b) == 0 {
		return nil, errors.New("invalid input size")
	}
	return b, nil
}

func run(args []string, now time.Time) error {
	fs := flag.NewFlagSet("lan-tls-preflight", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	origin := fs.String("origin", "", "")
	port := fs.Int("port", 0, "")
	cert := fs.String("cert-file", "", "")
	key := fs.String("key-file", "", "")
	root := fs.String("root-file", "", "")
	leafPin := fs.String("leaf-sha256", "", "")
	rootPin := fs.String("root-file-sha256", "", "")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errors.New("invalid parameters")
	}
	rootBytes, err := readInput(*root)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(rootBytes)
	if !strings.EqualFold(hex.EncodeToString(hash[:]), *rootPin) {
		return errors.New("approved root file pin mismatch")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootBytes) {
		return errors.New("invalid approved root PEM")
	}
	certBytes, err := readInput(*cert)
	if err != nil {
		return err
	}
	keyBytes, err := readInput(*key)
	if err != nil {
		return err
	}
	defer clear(keyBytes)
	return lanrotation.Validate(*origin, *port, certBytes, keyBytes, *leafPin, roots, now)
}

func main() {
	if err := run(os.Args[1:], time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "REFUSED: proposed LAN HTTPS identity failed preflight")
		os.Exit(1)
	}
	fmt.Println("PASS: proposed LAN HTTPS identity verified offline")
}
