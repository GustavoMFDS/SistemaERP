package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	NFCeModel                     = 65
	NFCeNormalEmissionType        = 1
	NFCeOfflineContingencyEmissionType = 9
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

func IsValidUF(value string) bool {
	_, ok := ufCodes[strings.ToUpper(strings.TrimSpace(value))]
	return ok
}

func UFCode(value string) (string, bool) {
	code, ok := ufCodes[strings.ToUpper(strings.TrimSpace(value))]
	return code, ok
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

func GenerateNFCeNumericCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", fmt.Errorf("generate NFC-e numeric code: %w", err)
	}
	return fmt.Sprintf("%08d", value.Int64()), nil
}

func BuildNFCeAccessKey(in NFCeAccessKeyInput) (string, error) {
	ufCode, ok := ufCodes[strings.ToUpper(strings.TrimSpace(in.UF))]
	if !ok {
		return "", fmt.Errorf("invalid issuer UF")
	}
	if in.IssuedAt.IsZero() {
		return "", fmt.Errorf("issuance timestamp is required")
	}
	cnpj := normalizeCNPJ(in.CNPJ)
	if err := ValidateCNPJ(cnpj); err != nil {
		return "", fmt.Errorf("invalid issuer CNPJ: %w", err)
	}
	if in.Series < 0 || in.Series > 889 {
		return "", fmt.Errorf("NFC-e series must be between 0 and 889")
	}
	if in.Number < 1 || in.Number > 999999999 {
		return "", fmt.Errorf("NFC-e number must be between 1 and 999999999")
	}
	if in.EmissionType != NFCeNormalEmissionType &&
		in.EmissionType != NFCeOfflineContingencyEmissionType {
		return "", fmt.Errorf("NFC-e emission type must be 1 (normal) or 9 (offline contingency)")
	}
	if !isDigits(in.NumericCode, 8) {
		return "", fmt.Errorf("numeric code must contain 8 digits")
	}

	base := fmt.Sprintf(
		"%s%s%s%02d%03d%09d%d%s",
		ufCode,
		in.IssuedAt.Format("0601"),
		cnpj,
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

// AccessKeyCheckDigit calculates cDV for the 43-character access-key base.
//
// Since the CNPJ alphanumeric transition, characters are converted to their
// ASCII code minus 48 before the same right-to-left modulo-11 weighting is
// applied. Numeric-only keys therefore retain the historical result.
func ValidateCNPJ(value string) error {
	cnpj := normalizeCNPJ(value)
	if !isCurrentCNPJFormat(cnpj) {
		return fmt.Errorf("CNPJ must contain 14 characters matching [A-Z0-9]{12}[0-9]{2}")
	}
	if isRepeatedCNPJ(cnpj) {
		return fmt.Errorf("CNPJ must not be a repeated-character placeholder")
	}

	body := cnpj[:12]
	first := cnpjCheckDigit(body, []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2})
	if int(cnpj[12]-'0') != first {
		return fmt.Errorf("first CNPJ check digit mismatch")
	}
	second := cnpjCheckDigit(
		body+string(byte('0'+first)),
		[]int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2},
	)
	if int(cnpj[13]-'0') != second {
		return fmt.Errorf("second CNPJ check digit mismatch")
	}
	return nil
}

func cnpjCheckDigit(value string, weights []int) int {
	sum := 0
	for i := 0; i < len(value); i++ {
		sum += (int(value[i]) - 48) * weights[i]
	}
	remainder := sum % 11
	if remainder == 0 || remainder == 1 {
		return 0
	}
	return 11 - remainder
}

func ValidateNFCeAccessKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) != 44 {
		return fmt.Errorf("access key must contain 44 characters")
	}
	if !isAccessKeyBaseFormat(key[:43]) || !isDigit(key[43]) {
		return fmt.Errorf("access key has invalid format")
	}
	if key[20:22] != "65" {
		return fmt.Errorf("access key model must be 65")
	}
	if err := ValidateCNPJ(key[6:20]); err != nil {
		return fmt.Errorf("access key CNPJ: %w", err)
	}
	dv, err := AccessKeyCheckDigit(key[:43])
	if err != nil {
		return err
	}
	if int(key[43]-'0') != dv {
		return fmt.Errorf("access key check digit mismatch")
	}
	return nil
}

func AccessKeyCheckDigit(base string) (int, error) {
	if !isAccessKeyBaseFormat(base) {
		return 0, fmt.Errorf("access-key base must match [0-9]{6}[A-Z0-9]{12}[0-9]{25}")
	}

	sum := 0
	weight := 2
	for i := len(base) - 1; i >= 0; i-- {
		value, err := accessKeyCharValue(base[i])
		if err != nil {
			return 0, err
		}
		sum += value * weight
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

func normalizeCNPJ(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isRepeatedCNPJ(value string) bool {
	if len(value) == 0 {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] != value[0] {
			return false
		}
	}
	return true
}

func isCurrentCNPJFormat(value string) bool {
	if len(value) != 14 {
		return false
	}
	for i := 0; i < 12; i++ {
		if !isUpperAlphaNumeric(value[i]) {
			return false
		}
	}
	return isDigit(value[12]) && isDigit(value[13])
}

func isAccessKeyBaseFormat(value string) bool {
	if len(value) != 43 {
		return false
	}
	for i := 0; i < 6; i++ {
		if !isDigit(value[i]) {
			return false
		}
	}
	for i := 6; i < 18; i++ {
		if !isUpperAlphaNumeric(value[i]) {
			return false
		}
	}
	for i := 18; i < 43; i++ {
		if !isDigit(value[i]) {
			return false
		}
	}
	return true
}

func accessKeyCharValue(value byte) (int, error) {
	if !isUpperAlphaNumeric(value) {
		return 0, fmt.Errorf("invalid access-key character %q", value)
	}
	return int(value) - 48, nil
}

func isUpperAlphaNumeric(value byte) bool {
	return isDigit(value) || (value >= 'A' && value <= 'Z')
}

func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func isDigits(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := 0; i < len(value); i++ {
		if !isDigit(value[i]) {
			return false
		}
	}
	return true
}
