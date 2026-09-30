package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	NFCeModel              = 65
	NFCeNormalEmissionType = 1
)

var ufCodes = map[string]string{
	"RO": "11",
	"AC": "12",
	"AM": "13",
	"RR": "14",
	"PA": "15",
	"AP": "16",
	"TO": "17",
	"MA": "21",
	"PI": "22",
	"CE": "23",
	"RN": "24",
	"PB": "25",
	"PE": "26",
	"AL": "27",
	"SE": "28",
	"BA": "29",
	"MG": "31",
	"ES": "32",
	"RJ": "33",
	"SP": "35",
	"PR": "41",
	"SC": "42",
	"RS": "43",
	"MS": "50",
	"MT": "51",
	"GO": "52",
	"DF": "53",
}

type NFCeAccessKeyInput struct {
	UF           string
	IssuedAt     time.Time
	CNPJ         string
	Series       int
	Number       int64
	NumericCode  string
	EmissionType int
}

func BuildNFCeAccessKey(in NFCeAccessKeyInput) (string, error) {
	ufCode, ok := ufCodes[strings.ToUpper(strings.TrimSpace(in.UF))]
	if !ok {
		return "", fmt.Errorf("invalid issuer UF")
	}
	if in.IssuedAt.IsZero() {
		return "", fmt.Errorf("issuance timestamp is required")
	}
	if !isDigits(in.CNPJ, 14) {
		return "", fmt.Errorf("issuer CNPJ must contain 14 digits")
	}
	if in.Series < 0 || in.Series > 889 {
		return "", fmt.Errorf("NFC-e series must be between 0 and 889")
	}
	if in.Number < 1 || in.Number > 999999999 {
		return "", fmt.Errorf("NFC-e number must be between 1 and 999999999")
	}
	if in.EmissionType != NFCeNormalEmissionType {
		return "", fmt.Errorf("only normal NFC-e emission type 1 is supported by this foundation")
	}
	if !isDigits(in.NumericCode, 8) {
		return "", fmt.Errorf("numeric code must contain 8 digits")
	}

	base := fmt.Sprintf(
		"%s%s%s%02d%03d%09d%d%s",
		ufCode,
		in.IssuedAt.Format("0601"),
		in.CNPJ,
		NFCeModel,
		in.Series,
		in.Number,
		in.EmissionType,
		in.NumericCode,
	)
	dv, err := AccessKeyCheckDigit(base)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", base, dv), nil
}

func AccessKeyCheckDigit(base string) (int, error) {
	if !isDigits(base, 43) {
		return 0, fmt.Errorf("access-key base must contain 43 digits")
	}

	sum := 0
	weight := 2
	for i := len(base) - 1; i >= 0; i-- {
		sum += int(base[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}

	remainder := sum % 11
	if remainder == 0 || remainder == 1 {
		return 0, nil
	}
	return 11 - remainder, nil
}

func isDigits(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
