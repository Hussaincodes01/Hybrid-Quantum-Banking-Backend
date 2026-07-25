package healthscore

import "math"

type Input struct {
	EmergencyFundMonths     float64   `json:"emergencyFundMonths"`
	DebtToIncomeRatio       float64   `json:"debtToIncomeRatio"`
	SavingsRate             float64   `json:"savingsRate"`
	DiversificationScore    float64   `json:"diversificationScore"`
	InsuranceCoverageScore  float64   `json:"insuranceCoverageScore"`
	GoalOnTrackRatio        float64   `json:"goalOnTrackRatio"`
	BehaviourQualityScore   float64   `json:"behaviourQualityScore"`
	MonthlyHistoricalScores []float64 `json:"monthlyHistoricalScores"`
}

type Pillar struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Score  float64 `json:"score"`
}

type Result struct {
	Score300To900 int      `json:"score300To900"`
	Band          string   `json:"band"`
	Pillars       []Pillar `json:"pillars"`
}

type Calculator struct{}

func NewCalculator() Calculator {
	return Calculator{}
}

func (Calculator) Calculate(input Input) Result {
	pillars := []Pillar{
		{Name: "Liquidity", Weight: 0.18, Score: liquidityScore(input.EmergencyFundMonths)},
		{Name: "DebtHealth", Weight: 0.18, Score: debtScore(input.DebtToIncomeRatio)},
		{Name: "SavingsBehaviour", Weight: 0.15, Score: ratioScore(input.SavingsRate, 0.35)},
		{Name: "InvestmentQuality", Weight: 0.15, Score: clamp(input.DiversificationScore, 0, 100)},
		{Name: "ProtectionCoverage", Weight: 0.15, Score: clamp(input.InsuranceCoverageScore, 0, 100)},
		{Name: "GoalAlignment", Weight: 0.12, Score: ratioScore(input.GoalOnTrackRatio, 1.0)},
		{Name: "FinancialBehaviour", Weight: 0.07, Score: clamp(input.BehaviourQualityScore, 0, 100)},
	}

	weighted := 0.0
	for _, p := range pillars {
		weighted += p.Score * p.Weight
	}

	modifier := resilienceModifier(input.MonthlyHistoricalScores)
	weighted = clamp(weighted*modifier, 0, 100)

	rawScore := int(math.Round(300 + (weighted / 100 * 600)))
	band := "red"
	if rawScore >= 750 {
		band = "green"
	} else if rawScore >= 550 {
		band = "amber"
	}

	return Result{Score300To900: rawScore, Band: band, Pillars: pillars}
}

func liquidityScore(months float64) float64 {
	return ratioScore(months, 6)
}

func debtScore(dti float64) float64 {
	if dti <= 0 {
		return 100
	}
	if dti >= 1 {
		return 0
	}
	return clamp((1-dti)*100, 0, 100)
}

func ratioScore(value, target float64) float64 {
	if target <= 0 {
		return 0
	}
	return clamp((value/target)*100, 0, 100)
}

func resilienceModifier(scores []float64) float64 {
	if len(scores) == 0 {
		return 1
	}
	weights := []float64{0.05, 0.08, 0.10, 0.15, 0.25, 0.37}

	start := 0
	if len(scores) > 6 {
		start = len(scores) - 6
	}
	trimmed := scores[start:]

	if len(trimmed) < 6 {
		padded := make([]float64, 6)
		offset := 6 - len(trimmed)
		for i := 0; i < offset; i++ {
			padded[i] = trimmed[0]
		}
		copy(padded[offset:], trimmed)
		trimmed = padded
	}

	weighted := 0.0
	for i := 0; i < 6; i++ {
		weighted += clamp(trimmed[i], 0, 100) * weights[i]
	}

	if weighted >= 80 {
		return 1.05
	}
	if weighted >= 65 {
		return 1.02
	}
	if weighted <= 40 {
		return 0.94
	}
	return 1
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
