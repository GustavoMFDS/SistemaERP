package platform

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Money stores Brazilian currency values in cents. It marshals as a JSON
// decimal number to keep API compatibility without using float64 internally.
type Money int64

const (
	maxMoneyCents    int64 = 999_999_999_999
	maxQuantityMilli int64 = 99_999_999_999_999
)

func NewMoneyCents(cents int64) Money {
	return Money(cents)
}

func ParseMoney(raw string) (Money, error) {
	cents, err := parseScaled(raw, 2)
	if err != nil {
		return 0, err
	}
	if cents > maxMoneyCents || cents < -maxMoneyCents {
		return 0, fmt.Errorf("money %q exceeds numeric(12,2) range", raw)
	}
	return Money(cents), nil
}

func (m Money) Cents() int64 {
	return int64(m)
}

func (m Money) DBString() string {
	return formatScaled(int64(m), 2)
}

func (m Money) String() string {
	return m.DBString()
}

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(m.DBString()), nil
}

func (m *Money) UnmarshalJSON(b []byte) error {
	raw := strings.TrimSpace(string(b))
	if raw == "null" || raw == "" {
		*m = 0
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		raw = s
	}
	v, err := ParseMoney(raw)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

func (m Money) Add(other Money) Money {
	return m + other
}

func (m Money) AddChecked(other Money) (Money, error) {
	value, ok := checkedAddInt64(int64(m), int64(other))
	if !ok || value > maxMoneyCents || value < -maxMoneyCents {
		return 0, fmt.Errorf("money addition exceeds numeric(12,2) range")
	}
	return Money(value), nil
}

func (m Money) Sub(other Money) Money {
	return m - other
}

func (m Money) SubChecked(other Money) (Money, error) {
	value, ok := checkedSubInt64(int64(m), int64(other))
	if !ok || value > maxMoneyCents || value < -maxMoneyCents {
		return 0, fmt.Errorf("money subtraction exceeds numeric(12,2) range")
	}
	return Money(value), nil
}

func (m Money) Neg() Money {
	return -m
}

func (m Money) MulQty(q Quantity) Money {
	value, err := m.MulQtyChecked(q)
	if err != nil {
		return 0
	}
	return value
}

func (m Money) MulQtyChecked(q Quantity) (Money, error) {
	value, err := MulDivRound(int64(m), q.Milli(), 1000)
	if err != nil || value > maxMoneyCents || value < -maxMoneyCents {
		return 0, fmt.Errorf("money multiplication exceeds numeric(12,2) range")
	}
	return Money(value), nil
}

// Quantity stores inventory quantities in thousandths.
type Quantity int64

func NewQuantityMilli(milli int64) Quantity {
	return Quantity(milli)
}

func ParseQuantity(raw string) (Quantity, error) {
	milli, err := parseScaled(raw, 3)
	if err != nil {
		return 0, err
	}
	if milli > maxQuantityMilli || milli < -maxQuantityMilli {
		return 0, fmt.Errorf("quantity %q exceeds numeric(14,3) range", raw)
	}
	return Quantity(milli), nil
}

func (q Quantity) Milli() int64 {
	return int64(q)
}

func (q Quantity) DBString() string {
	return formatScaled(int64(q), 3)
}

func (q Quantity) String() string {
	return q.DBString()
}

func (q Quantity) MarshalJSON() ([]byte, error) {
	return []byte(q.DBString()), nil
}

func (q *Quantity) UnmarshalJSON(b []byte) error {
	raw := strings.TrimSpace(string(b))
	if raw == "null" || raw == "" {
		*q = 0
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		raw = s
	}
	v, err := ParseQuantity(raw)
	if err != nil {
		return err
	}
	*q = v
	return nil
}

func parseScaled(raw string, scale int) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty decimal")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimPrefix(s, "-")
	} else if strings.HasPrefix(s, "+") {
		s = strings.TrimPrefix(s, "+")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid decimal %q", raw)
	}
	if len(parts) == 2 && parts[1] == "" {
		return 0, fmt.Errorf("invalid decimal %q", raw)
	}
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid decimal %q", raw)
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > scale {
		return 0, fmt.Errorf("decimal %q has more than %d decimal places", raw, scale)
	}
	for len(frac) < scale {
		frac += "0"
	}
	var fraction uint64
	if frac != "" {
		fraction, err = strconv.ParseUint(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid decimal %q", raw)
		}
	}
	factor := uint64(pow10(scale))
	if whole > (uint64(math.MaxInt64)-fraction)/factor {
		return 0, fmt.Errorf("decimal %q exceeds int64 range", raw)
	}
	value := int64(whole*factor + fraction)
	if neg {
		value = -value
	}
	return value, nil
}

func formatScaled(v int64, scale int) string {
	neg := v < 0
	var magnitude uint64
	if neg {
		magnitude = uint64(-(v + 1)) + 1
	} else {
		magnitude = uint64(v)
	}
	div := uint64(pow10(scale))
	out := fmt.Sprintf("%d.%0*d", magnitude/div, scale, magnitude%div)
	if neg {
		return "-" + out
	}
	return out
}

func pow10(scale int) int64 {
	n := int64(1)
	for i := 0; i < scale; i++ {
		n *= 10
	}
	return n
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func checkedSubInt64(a, b int64) (int64, bool) {
	if b == math.MinInt64 {
		if a >= 0 {
			return 0, false
		}
		return a - b, true
	}
	return checkedAddInt64(a, -b)
}

// MulDivRound computes a*b/d using arbitrary precision for the intermediate
// product and rounds halves away from zero.
func MulDivRound(a, b, d int64) (int64, error) {
	if d == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	product := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	divisor := big.NewInt(d)
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(product, divisor, remainder)

	absRemainder := new(big.Int).Abs(new(big.Int).Set(remainder))
	absDivisor := new(big.Int).Abs(new(big.Int).Set(divisor))
	twiceRemainder := new(big.Int).Lsh(absRemainder, 1)
	if twiceRemainder.Cmp(absDivisor) >= 0 {
		sameSign := product.Sign() == 0 || (product.Sign() > 0) == (divisor.Sign() > 0)
		if sameSign {
			quotient.Add(quotient, big.NewInt(1))
		} else {
			quotient.Sub(quotient, big.NewInt(1))
		}
	}
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("scaled arithmetic exceeds int64 range")
	}
	return quotient.Int64(), nil
}
