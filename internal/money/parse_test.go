package money

import "testing"

func TestParseCSVDecimalExact(t *testing.T) {
	cases := []struct {
		value, code, style string
		want               int64
	}{
		{"0.29", "USD", "dot", 29}, {"-1,234.56", "USD", "dot", -123456},
		{"(€ 1.234,56)", "EUR", "comma", -123456}, {"1\u202f234,50 EUR", "EUR", "comma", 123450},
		{"+$12.00", "USD", "dot", 1200}, {"-৳ 12.5", "BDT", "dot", -1250},
		{"100.00", "JPY", "dot", 100}, {"1.234", "KWD", "dot", 1234},
		{"90071992547409.91", "USD", "dot", MaxSafeMinor}, {"-90071992547409.91", "USD", "dot", -MaxSafeMinor},
		{"0001.2300", "USD", "dot", 123}, {"0", "JPY", "comma", 0},
	}
	for _, tc := range cases {
		t.Run(tc.value+tc.code, func(t *testing.T) {
			got, err := ParseDecimal(tc.value, tc.code, tc.style)
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestParseCSVDecimalRejectsAmbiguousOrInexact(t *testing.T) {
	for _, value := range []string{"", "1e3", "1.001", "90071992547409.92", "1,23.00", "1234,567.89", "1,234 567.00", "1.", ".5", "--1", "(-1)", "$-12", "€12", "１２", "1.2.3", "NaN", "1 00.00"} {
		t.Run(value, func(t *testing.T) {
			if got, err := ParseDecimal(value, "USD", "dot"); err == nil {
				t.Fatalf("accepted %q as %d", value, got)
			}
		})
	}
	for _, tc := range []struct{ value, code, style string }{{"100.01", "JPY", "dot"}, {"1.2345", "KWD", "dot"}, {"1", "ZZZ", "dot"}, {"1", "USD", "unknown"}} {
		if _, err := ParseDecimal(tc.value, tc.code, tc.style); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}
