package rules

import (
	"testing"

	"github.com/size-module/backend/internal/optimizer/core"
)

func TestNormalizeAndValidateFillsEngineDefaults(t *testing.T) {
	in := SaveProfileInput{Code: "  GLASS-DEFAULT  "}
	if err := normalizeAndValidate(&in); err != nil {
		t.Fatalf("expected a bare code to be valid, got %v", err)
	}
	if in.Code != "GLASS-DEFAULT" {
		t.Fatalf("code was not trimmed: %q", in.Code)
	}
	if in.Rules != core.DefaultRules() {
		t.Fatalf("empty rules were not defaulted: %+v", in.Rules)
	}
	if in.Objective.Weights != core.DefaultWeights() {
		t.Fatalf("empty objective was not defaulted: %+v", in.Objective.Weights)
	}
}

func TestNormalizeAndValidateRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		in   SaveProfileInput
	}{
		{"empty code", SaveProfileInput{Code: "  "}},
		{"negative kerf", SaveProfileInput{Code: "X", Rules: core.Rules{Kerf: -1}}},
		{"negative trim", SaveProfileInput{Code: "X", Rules: core.Rules{Trim: -1}}},
		{"bad cut mode", SaveProfileInput{Code: "X", Rules: core.Rules{CutMode: "scissors"}}},
		{"bad grain", SaveProfileInput{Code: "X", Rules: core.Rules{GrainMode: "diagonal"}}},
		{"negative overs", SaveProfileInput{Code: "X", Rules: core.Rules{OversAllowedPct: -0.1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			if err := normalizeAndValidate(&in); err == nil {
				t.Fatalf("expected %q to be rejected", tc.name)
			}
		})
	}
}

func TestNormalizeAndValidateKeepsExplicitRules(t *testing.T) {
	// A profile that only overrides cut mode must keep the rest of the engine
	// defaults, because Normalize only fills defaults when the whole object is
	// empty and the frontend's free-cutting path sends a complete rules set.
	in := SaveProfileInput{
		Code:  "CNC",
		Name:  "Free cutting",
		Rules: core.DefaultRules(),
	}
	in.Rules.CutMode = core.CutFree
	if err := normalizeAndValidate(&in); err != nil {
		t.Fatalf("valid free-cutting profile rejected: %v", err)
	}
	if in.Rules.CutMode != core.CutFree {
		t.Fatalf("cut mode was not preserved: %q", in.Rules.CutMode)
	}
	if in.Rules.Kerf != core.DefaultRules().Kerf {
		t.Fatalf("kerf was dropped when overriding cut mode: %d", in.Rules.Kerf)
	}
}
