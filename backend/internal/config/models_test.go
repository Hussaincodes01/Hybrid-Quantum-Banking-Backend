package config

import "testing"

func TestDefaultModelParamsValidate(t *testing.T) {
	p := Default()
	if err := p.Validate(); err != nil {
		t.Fatalf("default params must validate: %v", err)
	}
	if p.Version == "" {
		t.Fatal("default params must carry a version")
	}
	if p.RiskTiers.Medium != 40 || p.RiskTiers.High != 70 {
		t.Fatalf("risk tiers: got %v/%v, want 40/70", p.RiskTiers.Medium, p.RiskTiers.High)
	}
	if p.CoolingOff.BaseMinutes != 240 {
		t.Fatalf("cooling-off base: got %d, want 240", p.CoolingOff.BaseMinutes)
	}
}

func TestLoadModelParamsEnvOverride(t *testing.T) {
	t.Setenv("FINIX_RISK_TIER_HIGH", "80")
	t.Setenv("FINIX_SIM_MU", "0.09")
	p := LoadModelParams()
	if p.RiskTiers.High != 80 {
		t.Fatalf("override high tier: got %v, want 80", p.RiskTiers.High)
	}
	if p.Simulation.Mu != 0.09 {
		t.Fatalf("override mu: got %v, want 0.09", p.Simulation.Mu)
	}
	if p.Version == "srishti-math-v1" {
		t.Fatal("version must be marked as overridden so audit artefacts don't claim pristine v1")
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("overridden params must still validate: %v", err)
	}
}

func TestValidateRejectsInvalidTiers(t *testing.T) {
	p := Default()
	p.RiskTiers.High = 30 // high < medium
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error when high tier <= medium tier")
	}
}

func TestBadEnvOverrideIgnored(t *testing.T) {
	t.Setenv("FINIX_SIM_MU", "not-a-number")
	p := LoadModelParams()
	if p.Simulation.Mu != 0.07 {
		t.Fatalf("bad override should be ignored, got mu=%v", p.Simulation.Mu)
	}
	if p.Version != "srishti-math-v1" {
		t.Fatalf("unparseable override must not mark version overridden, got %q", p.Version)
	}
}
