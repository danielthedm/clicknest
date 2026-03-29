package query

import (
	"math"
)

// ZTestResult holds the output of a two-proportion Z-test.
type ZTestResult struct {
	ZScore      float64 `json:"z_score"`
	PValue      float64 `json:"p_value"`
	Significant bool    `json:"significant"`
}

// ZTestProportions performs a two-tailed Z-test comparing two conversion rates.
// Returns the Z-score, p-value, and whether p < 0.05.
func ZTestProportions(conversionsA, exposuresA, conversionsB, exposuresB int64) ZTestResult {
	if exposuresA <= 0 || exposuresB <= 0 {
		return ZTestResult{}
	}
	p1 := float64(conversionsA) / float64(exposuresA)
	p2 := float64(conversionsB) / float64(exposuresB)
	pPooled := float64(conversionsA+conversionsB) / float64(exposuresA+exposuresB)

	se := math.Sqrt(pPooled * (1 - pPooled) * (1.0/float64(exposuresA) + 1.0/float64(exposuresB)))
	if se == 0 {
		return ZTestResult{}
	}

	z := (p1 - p2) / se
	// Two-tailed p-value using the complementary error function.
	p := math.Erfc(math.Abs(z) / math.Sqrt2)

	return ZTestResult{
		ZScore:      math.Round(z*1000) / 1000,
		PValue:      math.Round(p*10000) / 10000,
		Significant: p < 0.05,
	}
}

// WilsonConfidenceInterval computes the Wilson score interval for a proportion.
// Returns the lower and upper bounds at the given confidence level (e.g. 0.95).
func WilsonConfidenceInterval(conversions, exposures int64, confidence float64) (low, high float64) {
	if exposures <= 0 {
		return 0, 0
	}
	n := float64(exposures)
	p := float64(conversions) / n

	// Z-value for confidence level (two-tailed).
	alpha := 1 - confidence
	z := zQuantile(1 - alpha/2)

	z2 := z * z
	denom := 1 + z2/n
	centre := p + z2/(2*n)
	spread := z * math.Sqrt((p*(1-p)+z2/(4*n))/n)

	low = (centre - spread) / denom
	high = (centre + spread) / denom

	// Clamp to [0, 1].
	if low < 0 {
		low = 0
	}
	if high > 1 {
		high = 1
	}
	return low, high
}

// RequiredSampleSize estimates the per-variant sample size needed for a given
// baseline conversion rate, minimum detectable effect (relative), significance
// level (e.g. 0.05), and power (e.g. 0.80).
func RequiredSampleSize(baselineRate, minimumDetectableEffect, significance, power float64) int64 {
	if baselineRate <= 0 || baselineRate >= 1 || minimumDetectableEffect <= 0 {
		return 0
	}
	p1 := baselineRate
	p2 := baselineRate * (1 + minimumDetectableEffect)
	if p2 >= 1 {
		p2 = 0.99
	}

	zAlpha := zQuantile(1 - significance/2)
	zBeta := zQuantile(power)

	numerator := math.Pow(zAlpha+zBeta, 2) * (p1*(1-p1) + p2*(1-p2))
	denominator := math.Pow(p1-p2, 2)
	if denominator == 0 {
		return 0
	}

	return int64(math.Ceil(numerator / denominator))
}

// ChiSquaredResult holds the output of a chi-squared test.
type ChiSquaredResult struct {
	ChiSquared  float64 `json:"chi_squared"`
	PValue      float64 `json:"p_value"`
	Significant bool    `json:"significant"`
}

// VariantCounts holds the exposure/conversion counts for one variant.
type VariantCounts struct {
	Exposures   int64
	Conversions int64
}

// ChiSquaredTest performs a chi-squared test of independence across multiple variants.
// Tests whether conversion rates differ significantly across variants.
func ChiSquaredTest(variants []VariantCounts) ChiSquaredResult {
	k := len(variants)
	if k < 2 {
		return ChiSquaredResult{}
	}

	var totalExposures, totalConversions int64
	for _, v := range variants {
		totalExposures += v.Exposures
		totalConversions += v.Conversions
	}
	if totalExposures == 0 {
		return ChiSquaredResult{}
	}

	var chiSq float64
	for _, v := range variants {
		if v.Exposures == 0 {
			continue
		}
		// Expected conversions and non-conversions for this variant.
		expectedConv := float64(v.Exposures) * float64(totalConversions) / float64(totalExposures)
		expectedNonConv := float64(v.Exposures) * float64(totalExposures-totalConversions) / float64(totalExposures)

		actualNonConv := float64(v.Exposures - v.Conversions)
		actualConv := float64(v.Conversions)

		if expectedConv > 0 {
			chiSq += math.Pow(actualConv-expectedConv, 2) / expectedConv
		}
		if expectedNonConv > 0 {
			chiSq += math.Pow(actualNonConv-expectedNonConv, 2) / expectedNonConv
		}
	}

	df := float64(k - 1)
	pValue := 1 - chiSquaredCDF(chiSq, df)

	return ChiSquaredResult{
		ChiSquared:  math.Round(chiSq*1000) / 1000,
		PValue:      math.Round(pValue*10000) / 10000,
		Significant: pValue < 0.05,
	}
}

// --- Internal math helpers ---

// zQuantile returns the Z-value for a given cumulative probability p
// using the rational approximation (Abramowitz & Stegun 26.2.23).
func zQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	if p == 0.5 {
		return 0
	}
	if p > 0.5 {
		return -zQuantile(1 - p)
	}

	t := math.Sqrt(-2 * math.Log(p))
	// Rational approximation constants.
	c0 := 2.515517
	c1 := 0.802853
	c2 := 0.010328
	d1 := 1.432788
	d2 := 0.189269
	d3 := 0.001308

	return -(t - (c0+c1*t+c2*t*t)/(1+d1*t+d2*t*t+d3*t*t*t))
}

// chiSquaredCDF computes P(X <= x) for a chi-squared distribution with df degrees of freedom
// using the regularized lower incomplete gamma function.
func chiSquaredCDF(x, df float64) float64 {
	if x <= 0 {
		return 0
	}
	return lowerIncompleteGammaReg(df/2, x/2)
}

// lowerIncompleteGammaReg computes the regularized lower incomplete gamma function P(a, x)
// using series expansion when x < a+1, and the continued fraction representation otherwise.
func lowerIncompleteGammaReg(a, x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		return 0
	}
	logGammaA := lgamma(a)

	if x < a+1 {
		// Series expansion.
		return gammaSeriesP(a, x, logGammaA)
	}
	// Continued fraction.
	return 1 - gammaCFQ(a, x, logGammaA)
}

// gammaSeriesP computes P(a, x) via series expansion.
func gammaSeriesP(a, x, logGammaA float64) float64 {
	if x == 0 {
		return 0
	}
	ap := a
	sum := 1.0 / a
	del := sum
	for i := 0; i < 200; i++ {
		ap++
		del *= x / ap
		sum += del
		if math.Abs(del) < math.Abs(sum)*1e-14 {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-logGammaA)
}

// gammaCFQ computes Q(a, x) = 1 - P(a, x) via continued fraction (Lentz's method).
func gammaCFQ(a, x, logGammaA float64) float64 {
	const tiny = 1e-30
	f := tiny
	c := f
	d := 0.0
	for i := 1; i <= 200; i++ {
		an := float64(-i) * (float64(i) - a)
		bn := x + 2*float64(i) + 1 - a
		d = bn + an*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = bn + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1.0 / d
		delta := c * d
		f *= delta
		if math.Abs(delta-1) < 1e-14 {
			break
		}
	}
	return f * math.Exp(-x+a*math.Log(x)-logGammaA)
}

// lgamma wraps math.Lgamma dropping the sign.
func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}
