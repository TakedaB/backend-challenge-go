// Package money implements Money as an immutable value object.
//
// Money never uses float32/float64. The internal representation is an
// int64 counting the minimal unit of the currency (cents, for BRL),
// which keeps arithmetic exact and avoids floating point rounding
// errors in financial calculations.
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Sentinel errors classifiable via errors.Is.
var (
	// ErrEmptyAmount is returned when the amount string is empty or blank.
	ErrEmptyAmount = errors.New("money: amount is empty")
	// ErrInvalidFormat is returned when the amount string is not a plain
	// decimal number (e.g. scientific notation, NaN, Infinity, letters).
	ErrInvalidFormat = errors.New("money: amount has invalid format")
	// ErrExcessiveScale is returned when the amount has more than two
	// decimal digits (e.g. "25.001").
	ErrExcessiveScale = errors.New("money: amount has more than two decimal places")
	// ErrNegativeNotAllowed is returned when a negative amount is supplied
	// where the caller explicitly disallows it (external financial inputs).
	ErrNegativeNotAllowed = errors.New("money: negative amount not allowed here")
	// ErrInvalidCurrency is returned when the currency code is not a
	// well-formed ISO 4217 alphabetic code.
	ErrInvalidCurrency = errors.New("money: invalid currency code")
	// ErrCurrencyMismatch is returned when an operation is attempted
	// between two Money values of different currencies.
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	// ErrOverflow is returned when an arithmetic operation would overflow
	// the int64 minor-unit representation.
	ErrOverflow = errors.New("money: arithmetic overflow")
)

// scaleFactor is the number of minor units per major unit for a
// two-decimal-place currency (e.g. 1 BRL = 100 centavos).
const scaleFactor int64 = 100

// Money is an immutable value object representing an exact monetary
// amount in a specific currency. The zero value is not valid; use
// NewFromString or Zero to construct one.
//
// Representation: amount is stored as int64 minor units (e.g. cents).
// This supports exact values from -92,233,720,368,547,758.08 to
// 92,233,720,368,547,758.07 in major units, which is far beyond any
// realistic wallet balance for this domain, and all arithmetic below
// checks for overflow explicitly rather than relying on wraparound.
type Money struct {
	minorUnits int64
	currency   string
}

// NewFromString parses a decimal string amount (e.g. "25.00") and an
// ISO 4217 currency code into a Money value.
//
// allowNegative controls whether a leading '-' is accepted. External
// financial inputs (BET, WIN, REFUND, ROLLBACK payloads, wallet initial
// balance) must call this with allowNegative=false; internal
// computations that may legitimately produce negative differences may
// call it with allowNegative=true.
//
// Rejected inputs: empty/blank strings, NaN, Infinity, scientific
// notation (e.g. "1e10"), more than two decimal digits, non-numeric
// characters, and (when allowNegative is false) a leading '-'.
// No silent rounding is ever performed: an amount with more than two
// decimal digits is rejected rather than truncated.
func NewFromString(amount string, currency string, allowNegative bool) (Money, error) {
	cur, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}

	trimmed := strings.TrimSpace(amount)
	if trimmed == "" {
		return Money{}, ErrEmptyAmount
	}

	minor, err := parseDecimalToMinorUnits(trimmed, allowNegative)
	if err != nil {
		return Money{}, err
	}

	return Money{minorUnits: minor, currency: cur}, nil
}

// Zero returns a zero-value Money in the given currency. Zero balances
// and zero LOSS amounts are valid in this domain.
func Zero(currency string) (Money, error) {
	cur, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{minorUnits: 0, currency: cur}, nil
}

// FromMinorUnits constructs a Money directly from minor units (cents).
// This is intended for repository/persistence code that reads the
// exact stored int64 back out of the database — it performs no string
// parsing, only currency validation.
func FromMinorUnits(minorUnits int64, currency string) (Money, error) {
	cur, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{minorUnits: minorUnits, currency: cur}, nil
}

// normalizeCurrency validates and uppercases an ISO 4217 alphabetic
// currency code (exactly 3 letters).
func normalizeCurrency(currency string) (string, error) {
	trimmed := strings.TrimSpace(currency)
	if len(trimmed) != 3 {
		return "", ErrInvalidCurrency
	}
	upper := strings.ToUpper(trimmed)
	for _, r := range upper {
		if r < 'A' || r > 'Z' {
			return "", ErrInvalidCurrency
		}
	}
	return upper, nil
}

// parseDecimalToMinorUnits parses a plain decimal string (optionally
// signed, at most 2 fractional digits) into minor units, rejecting
// NaN/Infinity/scientific notation and any non [0-9.-] character.
func parseDecimalToMinorUnits(s string, allowNegative bool) (int64, error) {
	negative := false
	rest := s

	if strings.HasPrefix(rest, "-") {
		if !allowNegative {
			return 0, ErrNegativeNotAllowed
		}
		negative = true
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "+") {
		rest = rest[1:]
	}

	if rest == "" {
		return 0, ErrInvalidFormat
	}

	// Reject anything that isn't strictly digits with an optional single
	// '.' — this alone excludes NaN, Infinity, "1e10", etc.
	dotSeen := false
	var intPart, fracPart strings.Builder
	for _, r := range rest {
		switch {
		case r >= '0' && r <= '9':
			if dotSeen {
				fracPart.WriteRune(r)
			} else {
				intPart.WriteRune(r)
			}
		case r == '.' && !dotSeen:
			dotSeen = true
		default:
			return 0, ErrInvalidFormat
		}
	}

	if intPart.Len() == 0 && fracPart.Len() == 0 {
		return 0, ErrInvalidFormat
	}

	if fracPart.Len() > 2 {
		return 0, ErrExcessiveScale
	}

	// Pad the fractional part to exactly 2 digits.
	frac := fracPart.String()
	for len(frac) < 2 {
		frac += "0"
	}

	intStr := intPart.String()
	if intStr == "" {
		intStr = "0"
	}

	intValue, err := strconv.ParseInt(intStr, 10, 64)
	if err != nil {
		// Overflow of the integer part itself.
		return 0, ErrOverflow
	}
	fracValue, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, ErrInvalidFormat
	}

	minor, ok := mulOverflows(intValue, scaleFactor)
	if !ok {
		return 0, ErrOverflow
	}
	minor, ok = addOverflows(minor, fracValue)
	if !ok {
		return 0, ErrOverflow
	}

	if negative {
		minor = -minor
	}
	return minor, nil
}

// mulOverflows multiplies a*b, returning (result, true) on success or
// (0, false) if the multiplication would overflow int64.
func mulOverflows(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	result := a * b
	if result/b != a {
		return 0, false
	}
	return result, true
}

// addOverflows adds a+b, returning (result, true) on success or
// (0, false) if the addition would overflow int64.
func addOverflows(a, b int64) (int64, bool) {
	result := a + b
	// Overflow occurred if the sign of the result is inconsistent with
	// the signs of the operands.
	if (b > 0 && result < a) || (b < 0 && result > a) {
		return 0, false
	}
	return result, true
}

// Currency returns the ISO 4217 currency code.
func (m Money) Currency() string {
	return m.currency
}

// IsNegative reports whether the amount is strictly less than zero.
func (m Money) IsNegative() bool {
	return m.minorUnits < 0
}

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool {
	return m.minorUnits == 0
}

// IsPositive reports whether the amount is strictly greater than zero.
// This is the check external financial inputs like BET, WIN, REFUND
// and ROLLBACK use, since the README requires those to be > 0 (only
// LOSS and an initial balance may legitimately be zero).
func (m Money) IsPositive() bool {
	return m.minorUnits > 0
}

// MinorUnits returns the exact amount in minor units (e.g. cents),
// intended for persistence as BIGINT.
func (m Money) MinorUnits() int64 {
	return m.minorUnits
}

// String renders the amount as a fixed-scale decimal string, e.g.
// "25.00" or "-25.00". This is the canonical external representation.
func (m Money) String() string {
	neg := m.minorUnits < 0
	abs := m.minorUnits
	if neg {
		abs = -abs
	}
	whole := abs / scaleFactor
	frac := abs % scaleFactor
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%02d", sign, whole, frac)
}

// Add returns m + other. Both must share the same currency.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	sum, ok := addOverflows(m.minorUnits, other.minorUnits)
	if !ok {
		return Money{}, ErrOverflow
	}
	return Money{minorUnits: sum, currency: m.currency}, nil
}

// Sub returns m - other. Both must share the same currency. The result
// may be negative (e.g. when computing a difference); callers enforcing
// a non-negative wallet balance must check IsNegative() themselves.
func (m Money) Sub(other Money) (Money, error) {
	neg, err := other.Negate()
	if err != nil {
		return Money{}, err
	}
	return m.Add(neg)
}

// Negate returns -m in the same currency.
func (m Money) Negate() (Money, error) {
	if m.minorUnits == minInt64Guard {
		return Money{}, ErrOverflow
	}
	return Money{minorUnits: -m.minorUnits, currency: m.currency}, nil
}

// minInt64Guard is math.MinInt64; negating it would overflow int64.
const minInt64Guard = -9223372036854775808

// Equal reports whether m and other have the same currency and amount.
func (m Money) Equal(other Money) bool {
	return m.currency == other.currency && m.minorUnits == other.minorUnits
}

// Compare returns -1, 0 or 1 as m is less than, equal to, or greater
// than other. Both must share the same currency.
func (m Money) Compare(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, ErrCurrencyMismatch
	}
	switch {
	case m.minorUnits < other.minorUnits:
		return -1, nil
	case m.minorUnits > other.minorUnits:
		return 1, nil
	default:
		return 0, nil
	}
}

// MarshalJSON serializes Money using the external contract shape:
// {"amount":"25.00","currency":"BRL"}.
func (m Money) MarshalJSON() ([]byte, error) {
	type wire struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}
	return json.Marshal(wire{Amount: m.String(), Currency: m.currency})
}

// UnmarshalJSON parses the external contract shape
// {"amount":"25.00","currency":"BRL"} into m. Negative amounts are
// rejected here because this path is used for external financial
// inputs; internal code that needs negative values should use
// NewFromString directly with allowNegative=true.
func (m *Money) UnmarshalJSON(data []byte) error {
	type wire struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	}
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return ErrInvalidFormat
	}
	parsed, err := NewFromString(w.Amount, w.Currency, false)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
