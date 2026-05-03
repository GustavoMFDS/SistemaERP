package platform

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Money stores Brazilian currency values in cents. It marshals as a JSON
// decimal number to keep API compatibility without using float64 internally.
type Money int64

func NewMoneyCents(cents int64) Money {
	return Money(cents)
}

func ParseMoney(raw string) (Money, error) {
	cents, err := parseScaled(raw, 2)
	return Money(cents), err
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

func (m Money) Sub(other Money) Money {
	return m - other
}

func (m Money) Neg() Money {
	return -m
}

func (m Money) MulQty(q Quantity) Money {
	return Money(roundDiv(int64(m)*q.Milli(), 1000))
}

// Quantity stores inventory quantities in thousandths.
type Quantity int64

func NewQuantityMilli(milli int64) Quantity {
	return Quantity(milli)
}

func ParseQuantity(raw string) (Quantity, error) {
	milli, err := parseScaled(raw, 3)
	return Quantity(milli), err
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
	whole, err := strconv.ParseInt(parts[0], 10, 64)
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
	n := int64(0)
	if frac != "" {
		n, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid decimal %q", raw)
		}
	}
	value := whole*pow10(scale) + n
	if neg {
		value = -value
	}
	return value, nil
}

func formatScaled(v int64, scale int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	div := pow10(scale)
	out := fmt.Sprintf("%d.%0*d", v/div, scale, v%div)
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

func roundDiv(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	if n < 0 {
		return -roundDiv(-n, d)
	}
	return (n + d/2) / d
}
