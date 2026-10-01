package sefaz

import (
	"fmt"
	"net/url"
	"strings"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
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
