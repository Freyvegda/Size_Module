package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// GoldenTolerancePct is the default allowed regression of the mean objective
// score before -check fails.
const GoldenTolerancePct = 0.5

// Golden stores the accepted mean objective score per solver, plus waste and
// fulfilment for context. It is committed so solver changes are measured
// against the same bar.
type Golden struct {
	Version  int                `json:"version"`
	Score    map[string]float64 `json:"score"`
	WastePct map[string]float64 `json:"wastePct"`
	FillPct  map[string]float64 `json:"fillPct"`
}

func LoadGolden(path string) (Golden, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Golden{}, err
	}
	var g Golden
	if err := json.Unmarshal(data, &g); err != nil {
		return Golden{}, fmt.Errorf("%s: %w", path, err)
	}
	if g.Score == nil {
		g.Score = map[string]float64{}
	}
	if g.WastePct == nil {
		g.WastePct = map[string]float64{}
	}
	if g.FillPct == nil {
		g.FillPct = map[string]float64{}
	}
	return g, nil
}

func SaveGolden(path string, totals map[string]Totals) error {
	g := Golden{
		Version:  1,
		Score:    map[string]float64{},
		WastePct: map[string]float64{},
		FillPct:  map[string]float64{},
	}
	for name, t := range totals {
		if t.Failures > 0 {
			continue
		}
		g.Score[name] = t.MeanScore
		g.WastePct[name] = t.MeanWastePct
		g.FillPct[name] = t.MeanFillPct
	}
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Compare returns one message per problem: a run failure or validation error,
// a solver missing from the golden file, a solver that produced no rows, or a
// mean score regression beyond tolerance.
func Compare(g Golden, totals map[string]Totals, tolerancePct float64) []string {
	if tolerancePct <= 0 {
		tolerancePct = GoldenTolerancePct
	}
	var problems []string

	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		t := totals[name]
		if t.Failures > 0 {
			problems = append(problems, fmt.Sprintf("%s: %d run(s) failed", name, t.Failures))
		}
		if t.Violations > 0 {
			problems = append(problems, fmt.Sprintf("%s: %d validation error(s)", name, t.Violations))
		}
		want, ok := g.Score[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no golden value recorded", name))
			continue
		}
		if t.MeanScore < want-tolerancePct {
			problems = append(problems, fmt.Sprintf(
				"%s: mean score %.1f is worse than golden %.1f (tolerance %.2f)",
				name, t.MeanScore, want, tolerancePct))
		}
	}

	for _, name := range sortedKeys(g.Score) {
		if _, ok := totals[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: golden value exists but this run produced no rows", name))
		}
	}
	return problems
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
