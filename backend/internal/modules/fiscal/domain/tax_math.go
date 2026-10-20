package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/example/sistemaemgo/internal/platform"
)

// TaxRate stores a percentage with four decimal places.
// Example: 0.1000% is represented internally as 1000.
type TaxRate int64

const (
	taxRateScale    int64 = 10_000
	taxPercentDenom int64 = 100 * taxRateScale
	MaxTaxRate            = TaxRate(taxPercentDenom) // 100.0000%
)

func ParseTaxRate(raw string) (TaxRate, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("empty tax rate")
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return 0, fmt.Errorf("tax rate must be unsigned")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid tax rate %q", raw)
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid tax rate %q", raw)
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if frac == "" || len(frac) > 4 {
			return 0, fmt.Errorf("tax rate %q must have at most four decimal places", raw)
		}
	}
	for len(frac) < 4 {
		frac += "0"
	}
	fracValue := int64(0)
	if frac != "" {
		fracValue, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid tax rate %q", raw)
		}
	}
	if whole > 100 {
		return 0, fmt.Errorf("tax rate exceeds 100%%")
	}
	rate := TaxRate(whole*taxRateScale + fracValue)
	if rate < 0 || rate > MaxTaxRate {
		return 0, fmt.Errorf("tax rate exceeds 100%%")
	}
	return rate, nil
}

func (r TaxRate) String() string {
	value := int64(r)
	return fmt.Sprintf("%d.%04d", value/taxRateScale, value%taxRateScale)
}

func (r TaxRate) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *TaxRate) UnmarshalJSON(content []byte) error {
	var raw string
	if err := json.Unmarshal(content, &raw); err != nil {
		return fmt.Errorf("tax rate must be a decimal string: %w", err)
	}
	value, err := ParseTaxRate(raw)
	if err != nil {
		return err
	}
	*r = value
	return nil
}

func ApplyTaxRate(base platform.Money, rate TaxRate) (platform.Money, error) {
	if base < 0 {
		return 0, fmt.Errorf("tax base must not be negative")
	}
	if rate < 0 || rate > MaxTaxRate {
		return 0, fmt.Errorf("invalid tax rate")
	}
	cents, err := platform.MulDivRound(base.Cents(), int64(rate), taxPercentDenom)
	if err != nil {
		return 0, err
	}
	return platform.NewMoneyCents(cents), nil
}

func EffectiveTaxRate(nominal, reduction TaxRate) (TaxRate, error) {
	if nominal < 0 || nominal > MaxTaxRate {
		return 0, fmt.Errorf("invalid nominal tax rate")
	}
	if reduction < 0 || reduction > MaxTaxRate {
		return 0, fmt.Errorf("invalid tax reduction")
	}
	value, err := platform.MulDivRound(
		int64(nominal),
		int64(MaxTaxRate-reduction),
		int64(MaxTaxRate),
	)
	if err != nil {
		return 0, err
	}
	return TaxRate(value), nil
}

type RegularTaxComponentInput struct {
	Rate      TaxRate `json:"rate"`
	Reduction TaxRate `json:"reduction"`
}

type RegularTaxComponentResult struct {
	Rate           TaxRate        `json:"rate"`
	Reduction      TaxRate        `json:"reduction"`
	EffectiveRate  TaxRate        `json:"effective_rate"`
	OperationValue platform.Money `json:"operation_value"`
	Value          platform.Money `json:"value"`
}

type RegularIBSCBSInput struct {
	CST            string                   `json:"cst"`
	Classification string                   `json:"classification"`
	Base           platform.Money           `json:"base"`
	IBSUF          RegularTaxComponentInput `json:"ibs_uf"`
	IBSMunicipal   RegularTaxComponentInput `json:"ibs_municipal"`
	CBS            RegularTaxComponentInput `json:"cbs"`
}

type RegularIBSCBSResult struct {
	CST            string                    `json:"cst"`
	Classification string                    `json:"classification"`
	Base           platform.Money            `json:"base"`
	IBSUF          RegularTaxComponentResult `json:"ibs_uf"`
	IBSMunicipal   RegularTaxComponentResult `json:"ibs_municipal"`
	IBSTotal       platform.Money            `json:"ibs_total"`
	CBS            RegularTaxComponentResult `json:"cbs"`
}

func CalculateRegularIBSCBS(input RegularIBSCBSInput) (RegularIBSCBSResult, error) {
	input.CST = strings.TrimSpace(input.CST)
	input.Classification = strings.TrimSpace(input.Classification)
	if len(input.CST) != 3 || !digitsOnly(input.CST) {
		return RegularIBSCBSResult{}, fmt.Errorf("IBS/CBS CST must contain three digits")
	}
	if len(input.Classification) != 6 || !digitsOnly(input.Classification) ||
		!strings.HasPrefix(input.Classification, input.CST) {
		return RegularIBSCBSResult{}, fmt.Errorf("cClassTrib must contain six digits and start with CST")
	}
	if input.Base < 0 {
		return RegularIBSCBSResult{}, fmt.Errorf("IBS/CBS base must not be negative")
	}

	uf, err := calculateRegularComponent(input.Base, input.IBSUF)
	if err != nil {
		return RegularIBSCBSResult{}, fmt.Errorf("IBS UF: %w", err)
	}
	municipal, err := calculateRegularComponent(input.Base, input.IBSMunicipal)
	if err != nil {
		return RegularIBSCBSResult{}, fmt.Errorf("IBS municipal: %w", err)
	}
	cbs, err := calculateRegularComponent(input.Base, input.CBS)
	if err != nil {
		return RegularIBSCBSResult{}, fmt.Errorf("CBS: %w", err)
	}
	ibsTotal, err := uf.Value.AddChecked(municipal.Value)
	if err != nil {
		return RegularIBSCBSResult{}, err
	}
	return RegularIBSCBSResult{
		CST:            input.CST,
		Classification: input.Classification,
		Base:           input.Base,
		IBSUF:          uf,
		IBSMunicipal:   municipal,
		IBSTotal:       ibsTotal,
		CBS:            cbs,
	}, nil
}

var (
	NFCe2026IBSUFRate        = mustReferenceTaxRate("0.1000")
	NFCe2026IBSMunicipalRate = mustReferenceTaxRate("0.0000")
	NFCe2026CBSRate          = mustReferenceTaxRate("0.9000")
)

func ValidateNFCeReferenceRates2026(result RegularIBSCBSResult) error {
	if result.IBSUF.Rate != NFCe2026IBSUFRate {
		return fmt.Errorf("2026 NFC-e IBS UF nominal rate must be %s", NFCe2026IBSUFRate.String())
	}
	if result.IBSMunicipal.Rate != NFCe2026IBSMunicipalRate {
		return fmt.Errorf("2026 NFC-e IBS municipal nominal rate must be %s", NFCe2026IBSMunicipalRate.String())
	}
	if result.CBS.Rate != NFCe2026CBSRate {
		return fmt.Errorf("2026 NFC-e CBS nominal rate must be %s", NFCe2026CBSRate.String())
	}
	return nil
}

func mustReferenceTaxRate(raw string) TaxRate {
	rate, err := ParseTaxRate(raw)
	if err != nil {
		panic(err)
	}
	return rate
}

func calculateRegularComponent(
	base platform.Money,
	input RegularTaxComponentInput,
) (RegularTaxComponentResult, error) {
	effective, err := EffectiveTaxRate(input.Rate, input.Reduction)
	if err != nil {
		return RegularTaxComponentResult{}, err
	}
	operationValue, err := ApplyTaxRate(base, input.Rate)
	if err != nil {
		return RegularTaxComponentResult{}, err
	}
	value, err := ApplyTaxRate(base, effective)
	if err != nil {
		return RegularTaxComponentResult{}, err
	}
	return RegularTaxComponentResult{
		Rate:           input.Rate,
		Reduction:      input.Reduction,
		EffectiveRate:  effective,
		OperationValue: operationValue,
		Value:          value,
	}, nil
}

func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
