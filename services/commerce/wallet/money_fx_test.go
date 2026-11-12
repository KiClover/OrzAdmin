package wallet

import (
	"errors"
	"math"
	"testing"
)

func TestMoneyFXParseRateAcceptsCanonicalDecimals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw       string
		numerator int64
		denom     int64
		scaled    int64
		canonical string
	}{
		{raw: "7.2", numerator: 36, denom: 5, scaled: 7_200_000, canonical: "7.200000"},
		{raw: "7.200000", numerator: 36, denom: 5, scaled: 7_200_000, canonical: "7.200000"},
		{raw: "0.1", numerator: 1, denom: 10, scaled: 100_000, canonical: "0.100000"},
		{raw: "100", numerator: 100, denom: 1, scaled: 100_000_000, canonical: "100.000000"},
		{raw: "12.3456", numerator: 7716, denom: 625, scaled: 12_345_600, canonical: "12.345600"},
	}
	for _, tc := range cases {
		rate, err := ParseRate(tc.raw)
		if err != nil {
			t.Fatalf("ParseRate(%q) 返回错误: %v", tc.raw, err)
		}
		if rate.Numerator != tc.numerator || rate.Denominator != tc.denom {
			t.Fatalf("ParseRate(%q) = %d/%d，期望 %d/%d", tc.raw, rate.Numerator, rate.Denominator, tc.numerator, tc.denom)
		}
		if rate.Scaled != tc.scaled || rate.Canonical != tc.canonical {
			t.Fatalf("ParseRate(%q) 定点 = %d %q，期望 %d %q", tc.raw, rate.Scaled, rate.Canonical, tc.scaled, tc.canonical)
		}
	}
}

func TestMoneyFXParseRateRejectsAmbiguousInput(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"", " ", " 7.2", "7.2 ", "+7.2", "-7.2", "7.", ".5", "1e3", "07.2",
		"7.1234567", "0", "0.09", "100.000001", "101", "abc", "7..2", "7,2",
	} {
		if _, err := ParseRate(raw); err == nil {
			t.Fatalf("ParseRate(%q) 应报错", raw)
		}
	}
}

func TestMoneyFXRateFromFractionRoundsDisplayAndKeepsFraction(t *testing.T) {
	t.Parallel()

	rate, err := RateFromFraction(36, 5)
	if err != nil {
		t.Fatalf("RateFromFraction 返回错误: %v", err)
	}
	if rate.Scaled != 7_200_000 || rate.Canonical != "7.200000" {
		t.Fatalf("定点/展示 = %d %q，期望 7200000 7.200000", rate.Scaled, rate.Canonical)
	}

	rate, err = RateFromFraction(1, 3)
	if err != nil {
		t.Fatalf("RateFromFraction(1,3) 返回错误: %v", err)
	}
	if rate.Canonical != "0.333333" {
		t.Fatalf("1/3 展示 = %q，期望 0.333333", rate.Canonical)
	}
	if rate.Numerator != 1 || rate.Denominator != 3 {
		t.Fatalf("换算仍应使用原始分数: %d/%d", rate.Numerator, rate.Denominator)
	}

	for _, tc := range []struct{ numerator, denominator int64 }{
		{0, 5}, {-1, 5}, {5, 0}, {5, -1}, {1001, 10},
		{1, 3_000_000},
	} {
		if _, err := RateFromFraction(tc.numerator, tc.denominator); err == nil {
			t.Fatalf("RateFromFraction(%d,%d) 应报错", tc.numerator, tc.denominator)
		}
	}
}

func TestMoneyFXFormatRateIsStable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		numerator, denominator int64
		want                   string
	}{
		{numerator: 36, denominator: 5, want: "7.200000"},
		{numerator: 3, denominator: 40, want: ""},
		{numerator: 5, denominator: 2, want: "2.500000"},
		{numerator: 100, denominator: 1, want: "100.000000"},
		{numerator: 1, denominator: 10, want: "0.100000"},
	}
	for _, tc := range cases {
		got, err := FormatRate(tc.numerator, tc.denominator)
		if tc.want == "" {
			if err == nil {
				t.Fatalf("FormatRate(%d,%d) 低于汇率下限应报错", tc.numerator, tc.denominator)
			}
			continue
		}
		if err != nil {
			t.Fatalf("FormatRate(%d,%d) 返回错误: %v", tc.numerator, tc.denominator, err)
		}
		if got != tc.want {
			t.Fatalf("FormatRate(%d,%d) = %q，期望 %q", tc.numerator, tc.denominator, got, tc.want)
		}
	}
}

func TestMoneyFXQuoteProviderAmountRoundsUp(t *testing.T) {
	t.Parallel()

	rate, err := ParseRate("7.2")
	if err != nil {
		t.Fatalf("ParseRate 返回错误: %v", err)
	}

	cases := []struct {
		walletAmount int64
		want         int64
	}{
		{walletAmount: 10_000, want: 1_389},
		{walletAmount: 720, want: 100},
		{walletAmount: 719, want: 100},
		{walletAmount: 360, want: 50},
		{walletAmount: 359, want: 50},
	}
	for _, tc := range cases {
		got, err := QuoteProviderAmount(tc.walletAmount, rate)
		if err != nil {
			t.Fatalf("QuoteProviderAmount(%d) 返回错误: %v", tc.walletAmount, err)
		}
		if got != tc.want {
			t.Fatalf("QuoteProviderAmount(%d) = %d，期望 %d", tc.walletAmount, got, tc.want)
		}
	}
}

func TestMoneyFXQuoteProviderAmountRejectsOutOfRange(t *testing.T) {
	t.Parallel()

	rate, err := ParseRate("7.2")
	if err != nil {
		t.Fatalf("ParseRate 返回错误: %v", err)
	}
	// 350 分 / 7.2 = 48.6 美分 → 49，低于 Creem custom_price 下限。
	if _, err := QuoteProviderAmount(350, rate); !errors.Is(err, errProviderAmountRange) {
		t.Fatalf("低于下限应返回 errProviderAmountRange，实际 %v", err)
	}
	if _, err := QuoteProviderAmount(0, rate); err == nil {
		t.Fatal("金额 0 应报错")
	}
	if _, err := QuoteProviderAmount(-1, rate); err == nil {
		t.Fatal("负金额应报错")
	}
	if _, err := QuoteProviderAmount(math.MaxInt64, rate); !errors.Is(err, errProviderAmountRange) {
		t.Fatalf("溢出应返回 errProviderAmountRange，实际 %v", err)
	}
	if _, err := QuoteProviderAmount(1, Rate{}); err == nil {
		t.Fatal("零汇率应报错")
	}
}

func TestMoneyFXQuoteProviderAmountForSmallRate(t *testing.T) {
	t.Parallel()

	rate, err := ParseRate("0.1")
	if err != nil {
		t.Fatalf("ParseRate 返回错误: %v", err)
	}
	got, err := QuoteProviderAmount(10_000, rate)
	if err != nil {
		t.Fatalf("QuoteProviderAmount 返回错误: %v", err)
	}
	if got != 100_000 {
		t.Fatalf("10000 分 / 0.1 = %d，期望 100000", got)
	}
}
