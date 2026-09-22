package money

import (
	"errors"
	"testing"
)

func TestNewFromString_ValidAmounts(t *testing.T) {
	cases := []struct {
		name   string
		amount string
		want   int64
	}{
		{"whole number", "25", 2500},
		{"two decimals", "25.00", 2500},
		{"one decimal padded", "25.5", 2550},
		{"zero", "0.00", 0},
		{"leading plus", "+10.00", 1000},
		{"large value", "1000000.00", 100000000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewFromString(tc.amount, "BRL", false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.MinorUnits() != tc.want {
				t.Errorf("MinorUnits() = %d, want %d", m.MinorUnits(), tc.want)
			}
			if m.Currency() != "BRL" {
				t.Errorf("Currency() = %s, want BRL", m.Currency())
			}
		})
	}
}

func TestNewFromString_RejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name    string
		amount  string
		wantErr error
	}{
		{"empty string", "", ErrEmptyAmount},
		{"blank string", "   ", ErrEmptyAmount},
		{"NaN", "NaN", ErrInvalidFormat},
		{"Infinity", "Infinity", ErrInvalidFormat},
		{"scientific notation", "1e10", ErrInvalidFormat},
		{"letters", "abc", ErrInvalidFormat},
		{"excessive scale", "25.001", ErrExcessiveScale},
		{"multiple dots", "25.00.00", ErrInvalidFormat},
		{"negative when disallowed", "-25.00", ErrNegativeNotAllowed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewFromString(tc.amount, "BRL", false)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("NewFromString(%q) error = %v, want %v", tc.amount, err, tc.wantErr)
			}
		})
	}
}

func TestNewFromString_AllowsNegativeWhenPermitted(t *testing.T) {
	m, err := NewFromString("-25.00", "BRL", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.IsNegative() {
		t.Errorf("expected negative amount")
	}
	if m.MinorUnits() != -2500 {
		t.Errorf("MinorUnits() = %d, want -2500", m.MinorUnits())
	}
}

func TestNewFromString_InvalidCurrency(t *testing.T) {
	cases := []string{"", "BR", "BRLL", "12L", "brl "}
	for _, cur := range cases {
		t.Run(cur, func(t *testing.T) {
			_, err := NewFromString("10.00", cur, false)
			// "brl " with trailing space trims to "brl" which is valid
			// after normalization (3 letters) — skip that one explicitly.
			if cur == "brl " {
				if err != nil {
					t.Errorf("expected lowercase currency to normalize, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidCurrency) {
				t.Errorf("NewFromString currency=%q error = %v, want ErrInvalidCurrency", cur, err)
			}
		})
	}
}

func TestZero(t *testing.T) {
	m, err := Zero("BRL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.IsZero() {
		t.Errorf("expected zero amount")
	}
	if m.String() != "0.00" {
		t.Errorf("String() = %s, want 0.00", m.String())
	}
}

func TestAdd(t *testing.T) {
	a, _ := NewFromString("25.00", "BRL", false)
	b, _ := NewFromString("10.50", "BRL", false)

	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum.String() != "35.50" {
		t.Errorf("Add() = %s, want 35.50", sum.String())
	}
}

func TestSub_CanGoNegative(t *testing.T) {
	a, _ := NewFromString("10.00", "BRL", false)
	b, _ := NewFromString("25.00", "BRL", false)

	diff, err := a.Sub(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.String() != "-15.00" {
		t.Errorf("Sub() = %s, want -15.00", diff.String())
	}
	if !diff.IsNegative() {
		t.Errorf("expected negative result")
	}
}

func TestNegate(t *testing.T) {
	a, _ := NewFromString("25.00", "BRL", false)
	neg, err := a.Negate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if neg.String() != "-25.00" {
		t.Errorf("Negate() = %s, want -25.00", neg.String())
	}
}

func TestCurrencyMismatch(t *testing.T) {
	brl, _ := NewFromString("25.00", "BRL", false)
	usd, _ := NewFromString("25.00", "USD", false)

	if _, err := brl.Add(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add across currencies error = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := brl.Sub(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Sub across currencies error = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := brl.Compare(usd); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Compare across currencies error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestCompare(t *testing.T) {
	small, _ := NewFromString("10.00", "BRL", false)
	big, _ := NewFromString("25.00", "BRL", false)
	equal, _ := NewFromString("10.00", "BRL", false)

	if c, _ := small.Compare(big); c != -1 {
		t.Errorf("small.Compare(big) = %d, want -1", c)
	}
	if c, _ := big.Compare(small); c != 1 {
		t.Errorf("big.Compare(small) = %d, want 1", c)
	}
	if c, _ := small.Compare(equal); c != 0 {
		t.Errorf("small.Compare(equal) = %d, want 0", c)
	}
}

func TestEqual(t *testing.T) {
	a, _ := NewFromString("25.00", "BRL", false)
	b, _ := NewFromString("25.00", "BRL", false)
	c, _ := NewFromString("25.00", "USD", false)

	if !a.Equal(b) {
		t.Errorf("expected a.Equal(b)")
	}
	if a.Equal(c) {
		t.Errorf("expected a not to equal c (different currency)")
	}
}

func TestOverflow_OnAdd(t *testing.T) {
	max, _ := FromMinorUnits(9223372036854775807, "BRL")
	one, _ := NewFromString("0.01", "BRL", false)

	if _, err := max.Add(one); !errors.Is(err, ErrOverflow) {
		t.Errorf("Add() error = %v, want ErrOverflow", err)
	}
}

func TestOverflow_OnParseHugeAmount(t *testing.T) {
	// A decimal amount whose integer part alone overflows int64 when
	// multiplied by the scale factor (100).
	_, err := NewFromString("99999999999999999999.00", "BRL", false)
	if err == nil {
		t.Fatalf("expected an error for an amount this large")
	}
}

func TestMarshalUnmarshalJSON(t *testing.T) {
	original, _ := NewFromString("25.00", "BRL", false)

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	want := `{"amount":"25.00","currency":"BRL"}`
	if string(data) != want {
		t.Errorf("MarshalJSON() = %s, want %s", data, want)
	}

	var decoded Money
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if !decoded.Equal(original) {
		t.Errorf("decoded = %v, want %v", decoded, original)
	}
}

func TestUnmarshalJSON_RejectsNegativeExternalInput(t *testing.T) {
	var m Money
	err := m.UnmarshalJSON([]byte(`{"amount":"-25.00","currency":"BRL"}`))
	if !errors.Is(err, ErrNegativeNotAllowed) {
		t.Errorf("UnmarshalJSON() error = %v, want ErrNegativeNotAllowed", err)
	}
}

func TestUnmarshalJSON_RejectsMalformedJSON(t *testing.T) {
	var m Money
	err := m.UnmarshalJSON([]byte(`not-json`))
	if !errors.Is(err, ErrInvalidFormat) {
		t.Errorf("UnmarshalJSON() error = %v, want ErrInvalidFormat", err)
	}
}
