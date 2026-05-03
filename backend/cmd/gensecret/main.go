package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

type format string

const (
	formatHex       format = "hex"
	formatBase64    format = "base64"
	formatBase64URL format = "base64url"
)

func main() {
	var (
		bytesN int
		outFmt string
	)
	flag.IntVar(&bytesN, "bytes", 32, "number of random bytes")
	flag.StringVar(&outFmt, "format", string(formatBase64URL), "output format: hex|base64|base64url")
	flag.Parse()

	if bytesN < 16 {
		fmt.Fprintln(os.Stderr, "-bytes must be >= 16")
		os.Exit(2)
	}

	b := make([]byte, bytesN)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintln(os.Stderr, "failed to read randomness:", err)
		os.Exit(1)
	}

	switch format(outFmt) {
	case formatHex:
		fmt.Println(hex.EncodeToString(b))
	case formatBase64:
		fmt.Println(base64.StdEncoding.EncodeToString(b))
	case formatBase64URL:
		fmt.Println(base64.RawURLEncoding.EncodeToString(b))
	default:
		fmt.Fprintln(os.Stderr, "invalid -format; use hex|base64|base64url")
		os.Exit(2)
	}
}
