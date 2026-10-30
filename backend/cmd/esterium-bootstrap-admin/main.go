package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/bootstrapadmin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		// run returns only fixed, input-free diagnostics. Underlying database
		// errors are deliberately mapped before reaching this boundary.
		fmt.Fprintf(os.Stderr, "FAIL: first administrator was not created: %s\n", err)
		os.Exit(1)
	}
	fmt.Println("PASS: first administrator created for the new company.")
}

func run() error {
	databaseURLFile := flag.String("database-url-file", "", "protected file containing the local PostgreSQL URL")
	passwordStdin := flag.Bool("password-stdin", false, "read the new password from stdin; never pass it as an argument")
	allowInsecureCI := flag.Bool("allow-insecure-ci", false, "allow non-TLS URL only in an explicitly disposable GitHub-hosted runner")
	flag.Parse()
	if flag.NArg() != 0 || *databaseURLFile == "" || !*passwordStdin {
		return errors.New("required arguments missing")
	}
	stdinInfo, err := os.Stdin.Stat()
	if err != nil || stdinInfo.Mode()&os.ModeCharDevice != 0 {
		return errors.New("password input must come from the protected installer pipe")
	}
	input := bootstrapadmin.Input{
		LegalName:  os.Getenv("ESTERIUM_BOOTSTRAP_LEGAL_NAME"),
		TradeName:  os.Getenv("ESTERIUM_BOOTSTRAP_TRADE_NAME"),
		CNPJ:       os.Getenv("ESTERIUM_BOOTSTRAP_CNPJ"),
		AdminName:  os.Getenv("ESTERIUM_BOOTSTRAP_ADMIN_NAME"),
		AdminEmail: os.Getenv("ESTERIUM_BOOTSTRAP_ADMIN_EMAIL"),
	}
	passwordBytes, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	defer clear(passwordBytes)
	password := string(passwordBytes)
	defer func() { password = "" }()
	if err := bootstrapadmin.Validate(input, password); err != nil {
		return err
	}

	urlPath, err := filepath.Abs(*databaseURLFile)
	if err != nil {
		return errors.New("database URL file path is invalid")
	}
	if strings.HasPrefix(urlPath, `\\`) {
		return errors.New("database URL file must be local")
	}
	fileInfo, err := os.Lstat(urlPath)
	if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 || fileInfo.Size() < 1 || fileInfo.Size() > 1<<20 {
		return errors.New("database URL file is invalid")
	}
	urlBytes, err := os.ReadFile(urlPath)
	if err != nil {
		return errors.New("database URL file could not be read")
	}
	defer clear(urlBytes)
	databaseURL := strings.TrimSpace(string(urlBytes))
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.Scheme != "postgres" || parsed.User == nil || parsed.User.Username() == "" {
		return errors.New("database URL is invalid")
	}
	if parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1" {
		return errors.New("first-admin setup only supports a local database")
	}
	ciAllowed := *allowInsecureCI && os.Getenv("GITHUB_ACTIONS") == "true" &&
		os.Getenv("RUNNER_ENVIRONMENT") == "github-hosted" &&
		os.Getenv("ESTERIUM_ADMIN_BOOTSTRAP_DISPOSABLE_CI") == "1"
	if *allowInsecureCI && !ciAllowed {
		return errors.New("insecure database mode is only available on the disposable GitHub-hosted CI runner")
	}
	if parsed.Query().Get("sslmode") != "verify-full" && !ciAllowed {
		return errors.New("local database URL must verify TLS")
	}
	if !ciAllowed {
		rootCA := parsed.Query().Get("sslrootcert")
		if rootCA == "" {
			return errors.New("local database URL must name its PostgreSQL root CA")
		}
		if caInfo, err := os.Stat(rootCA); err != nil || !caInfo.Mode().IsRegular() {
			return errors.New("PostgreSQL root CA file is missing or invalid")
		}
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("database URL options are invalid")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return errors.New("database pool configuration is invalid")
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		return errors.New("database connection failed")
	}
	return bootstrapadmin.Create(ctx, pool, input, password)
}

func readPassword(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 1025))
	if err != nil || len(data) == 0 || len(data) > 1024 {
		clear(data)
		return nil, errors.New("password input is missing or too long")
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
		if len(data) > 0 && data[len(data)-1] == '\r' {
			data = data[:len(data)-1]
		}
	}
	if strings.ContainsAny(string(data), "\r\n\x00") {
		clear(data)
		return nil, errors.New("password input must be one line")
	}
	return data, nil
}

func clear(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

