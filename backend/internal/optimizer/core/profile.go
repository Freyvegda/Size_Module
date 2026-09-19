package core

// DetectProfile inspects the parts to decide which dimension profile a problem
// belongs to. A problem with any 2D part is a 2D problem; length-only parts are
// 1D. Empty or unusual problems default to 2D, the richer profile.
func DetectProfile(p Problem) DimensionProfile {
	has2D := false
	has1D := false
	for _, part := range p.Parts {
		switch {
		case part.Width > 0 && part.Height > 0:
			has2D = true
		case part.Length > 0:
			has1D = true
		}
	}
	switch {
	case has2D:
		return Profile2D
	case has1D:
		return Profile1D
	default:
		return Profile2D
	}
}

// CutModeCompatible reports whether a solver whose capability is `have` can
// serve a problem that asks for `want`. A guillotine layout is always valid on
// a machine that accepts free cutting; the reverse is not true.
func CutModeCompatible(have, want CutMode) bool {
	if want == "" || have == "" {
		return true
	}
	if have == want {
		return true
	}
	return want == CutFree && have == CutGuillotine
}
