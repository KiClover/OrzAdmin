package wallet

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// 固定结算汇率与 CNY→USD 换算。
//
// 规则见 WalletDesign §5.1：汇率是十进制字符串（1 USD = N CNY），服务端规范化为
// 正整数分子/分母；钱包金额是 CNY 分，Provider 金额是 USD 美分，两者都保留两位
// 小数，因此 provider_amount = ceil(wallet_amount * denominator / numerator)。
//
// 这里只使用整数与 math/big：不接受 float32/float64，也不接受科学计数法、
// 正负号或 "7." / ".5" 这类不完整写法，避免浮点误差与解析歧义。

const (
	// rateDecimalScale 是汇率小数的定点放大倍数：6 位小数。
	rateDecimalScale int64 = 1_000_000
	// rateMinScaled 是允许的最小汇率 0.100000。
	rateMinScaled int64 = 100_000
	// rateMaxScaled 是允许的最大汇率 100.000000。
	rateMaxScaled int64 = 100_000_000

	// providerAmountMin 是 Creem custom_price 的最小值（50 美分）。
	providerAmountMin int64 = 50
	// providerAmountMax 是本项目允许的 USD 单笔上限（1,000,000.00 美元），
	// 远低于 int64 与 Creem 上限，用于尽早拒绝异常报价。
	providerAmountMax int64 = 100_000_000

	// rateCanonicalDecimals 是规范化汇率字符串固定保留的小数位数。
	rateCanonicalDecimals = 6
)

var (
	// errRateMalformed 表示汇率不是要求的十进制定价格式。
	errRateMalformed = errors.New("rate must be a decimal number")
	// errRateRange 表示汇率超出允许区间。
	errRateRange = errors.New("rate is out of range")
	// errProviderAmountRange 表示换算结果超出 Provider 接受区间。
	errProviderAmountRange = errors.New("provider amount is out of range")
)

// Rate 是约分后的固定汇率：1 USD = Numerator/Denominator CNY。
type Rate struct {
	Numerator   int64
	Denominator int64
	// Scaled 是规范化后的 6 位定点值，便于比较与测试。
	Scaled int64
	// Canonical 是固定 6 位小数的展示字符串。
	Canonical string
}

// ParseRate 把十进制字符串解析成定点汇率并约分。
//
// 合法示例："7.2"、"7.200000"、"0.1"、"100"；拒绝："+7.2"、"7."、".5"、
// "1e3"、"07.2"、"7.1234567"、"0"、"0.09"、"101"。
func ParseRate(raw string) (Rate, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return Rate{}, errRateMalformed
	}
	integerPart, fractionPart, hasDot := strings.Cut(raw, ".")
	if integerPart == "" {
		return Rate{}, errRateMalformed
	}
	if strings.HasPrefix(integerPart, "+") || strings.HasPrefix(integerPart, "-") {
		return Rate{}, errRateMalformed
	}
	if len(integerPart) > 1 && integerPart[0] == '0' {
		return Rate{}, errRateMalformed
	}
	for i := 0; i < len(integerPart); i++ {
		if integerPart[i] < '0' || integerPart[i] > '9' {
			return Rate{}, errRateMalformed
		}
	}
	if hasDot {
		if fractionPart == "" || len(fractionPart) > 6 {
			return Rate{}, errRateMalformed
		}
		for i := 0; i < len(fractionPart); i++ {
			if fractionPart[i] < '0' || fractionPart[i] > '9' {
				return Rate{}, errRateMalformed
			}
		}
	}
	integer, err := strconv.ParseInt(integerPart, 10, 64)
	if err != nil {
		return Rate{}, errRateMalformed
	}
	if integer > 100 {
		return Rate{}, errRateRange
	}
	fraction := int64(0)
	if hasDot {
		padded := fractionPart + strings.Repeat("0", 6-len(fractionPart))
		fraction, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return Rate{}, errRateMalformed
		}
	}
	scaled := integer*rateDecimalScale + fraction
	if scaled < rateMinScaled || scaled > rateMaxScaled {
		return Rate{}, errRateRange
	}
	numerator, denominator := reduceRate(scaled, rateDecimalScale)
	return Rate{
		Numerator:   numerator,
		Denominator: denominator,
		Scaled:      scaled,
		Canonical:   formatRateScaled(scaled),
	}, nil
}

// RateFromFraction 用已约分的分子/分母构造并校验汇率。
func RateFromFraction(numerator, denominator int64) (Rate, error) {
	if numerator <= 0 || denominator <= 0 {
		return Rate{}, errRateRange
	}
	// 还原成 6 位定点数后走与字符串解析相同的区间校验，避免两套边界。
	// 四舍五入到 6 位小数：定点值只用于比较与展示，换算始终使用原始分子/分母。
	scaledBig := new(big.Int).Mul(big.NewInt(numerator), big.NewInt(rateDecimalScale))
	scaledBig.Add(scaledBig, big.NewInt(denominator/2))
	scaledBig.Quo(scaledBig, big.NewInt(denominator))
	if !scaledBig.IsInt64() {
		return Rate{}, errRateRange
	}
	scaled := scaledBig.Int64()
	if scaled < rateMinScaled || scaled > rateMaxScaled {
		return Rate{}, errRateRange
	}
	reducedNumerator, reducedDenominator := reduceRate(numerator, denominator)
	return Rate{
		Numerator:   reducedNumerator,
		Denominator: reducedDenominator,
		Scaled:      scaled,
		Canonical:   formatRateScaled(scaled),
	}, nil
}

// QuoteProviderAmount 按固定汇率把 CNY 分换算成 USD 美分，向上取整。
//
// 例如汇率 7.20、钱包金额 10000 分：ceil(10000 / 7.20) = 1389 美分。
func QuoteProviderAmount(walletAmount int64, rate Rate) (int64, error) {
	if walletAmount <= 0 {
		return 0, errors.New("wallet amount must be positive")
	}
	if rate.Numerator <= 0 || rate.Denominator <= 0 {
		return 0, errRateRange
	}
	// ceil(a/b) = (a + b - 1) / b，用 big.Int 防止中间乘法溢出。
	numerator := new(big.Int).Mul(big.NewInt(walletAmount), big.NewInt(rate.Denominator))
	denominator := big.NewInt(rate.Numerator)
	numerator.Add(numerator, new(big.Int).Sub(denominator, big.NewInt(1)))
	numerator.Quo(numerator, denominator)
	if !numerator.IsInt64() {
		return 0, errProviderAmountRange
	}
	providerAmount := numerator.Int64()
	if providerAmount < providerAmountMin || providerAmount > providerAmountMax {
		return 0, errProviderAmountRange
	}
	return providerAmount, nil
}

// FormatRate 把分子/分母格式化成固定 6 位小数的展示字符串。
func FormatRate(numerator, denominator int64) (string, error) {
	rate, err := RateFromFraction(numerator, denominator)
	if err != nil {
		return "", err
	}
	return rate.Canonical, nil
}

// reduceRate 用最大公约数约分。
func reduceRate(numerator, denominator int64) (int64, int64) {
	divisor := gcdInt64(numerator, denominator)
	if divisor <= 0 {
		return numerator, denominator
	}
	return numerator / divisor, denominator / divisor
}

// gcdInt64 返回两个正整数的最大公约数。
func gcdInt64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// formatRateScaled 把 6 位定点值格式化成固定 6 位小数字符串。
func formatRateScaled(scaled int64) string {
	integer := scaled / rateDecimalScale
	fraction := scaled % rateDecimalScale
	text := strconv.FormatInt(fraction, 10)
	text = strings.Repeat("0", rateCanonicalDecimals-len(text)) + text
	return strconv.FormatInt(integer, 10) + "." + text
}
