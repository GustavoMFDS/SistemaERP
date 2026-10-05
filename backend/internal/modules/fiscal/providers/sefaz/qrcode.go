package sefaz

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

const QRCodeVersion3 = "3"

func BuildOnlineQRCodeV3URL(baseURL string, env Environment, accessKey string) (string, error) {
	accessKey = strings.TrimSpace(accessKey)
	if err := fisc.ValidateNFCeAccessKey(accessKey); err != nil {
		return "", fmt.Errorf("QR Code access key: %w", err)
	}
	ambient, err := tpAmb(env)
	if err != nil {
		return "", err
	}

	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid QR Code base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("QR Code base URL must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid QR Code base URL")
	}
	if parsed.Query().Has("p") {
		return "", fmt.Errorf("QR Code base URL must not already contain parameter p")
	}

	value := accessKey + "|" + QRCodeVersion3 + "|" + ambient
	if parsed.RawQuery == "" {
		parsed.RawQuery = "p=" + value
	} else {
		parsed.RawQuery += "&p=" + value
	}
	return parsed.String(), nil
}

func BuildOfflineQRCodeV3Payload(
	env Environment,
	accessKey string,
	issuedAt time.Time,
	total platform.Money,
) (string, error) {
	accessKey = strings.TrimSpace(accessKey)
	if err := fisc.ValidateNFCeAccessKey(accessKey); err != nil {
		return "", fmt.Errorf("QR Code access key: %w", err)
	}
	if accessKey[34] != '9' {
		return "", fmt.Errorf("offline QR Code requires tpEmis=9 access key")
	}
	if issuedAt.IsZero() {
		return "", fmt.Errorf("offline QR Code issuance time is required")
	}
	if total <= 0 {
		return "", fmt.Errorf("offline QR Code total must be positive")
	}
	ambient, err := tpAmb(env)
	if err != nil {
		return "", err
	}
	// Parameters 6 and 7 are intentionally empty while the conservative
	// NFC-e flow supports only an unidentified consumer.
	return strings.Join([]string{
		accessKey,
		QRCodeVersion3,
		ambient,
		issuedAt.Format("02"),
		total.DBString(),
		"",
		"",
	}, "|"), nil
}

func BuildOfflineQRCodeV3URL(
	baseURL string,
	payload string,
	signature string,
) (string, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", fmt.Errorf("offline QR Code payload is required")
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return "", fmt.Errorf("offline QR Code signature is required")
	}
	if _, err := base64.StdEncoding.DecodeString(signature); err != nil {
		return "", fmt.Errorf("offline QR Code signature must be base64: %w", err)
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid QR Code base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("QR Code base URL must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid QR Code base URL")
	}
	if parsed.Query().Has("p") {
		return "", fmt.Errorf("QR Code base URL must not already contain parameter p")
	}
	value := payload + "|" + signature
	if parsed.RawQuery == "" {
		parsed.RawQuery = "p=" + value
	} else {
		parsed.RawQuery += "&p=" + value
	}
	return parsed.String(), nil
}
