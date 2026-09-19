package plans

import (
	"fmt"
	"sort"

	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
)

// EditOperation changes one placement of a plan. The placement id comes from
// GET /plans/{id}; all fields are optional except the id, so a client can drag
// (x/y), rotate, lock/unlock or delete a single piece.
type EditOperation struct {
	PlacementID string    `json:"placementId"`
	X           *core.Dim `json:"x,omitempty"`
	Y           *core.Dim `json:"y,omitempty"`
	Rotated     *bool     `json:"rotated,omitempty"`
	Locked      *bool     `json:"locked,omitempty"`
	Delete      bool      `json:"delete,omitempty"`
}

// EditRequest is the body of POST /plans/{id}/edit.
type EditRequest struct {
	Operations []EditOperation `json:"operations"`
	Name       string          `json:"name,omitempty"`
}

// ReoptimizeRequest is the body of POST /plans/{id}/reoptimize. Operations are
// applied before solving, so a client can drag and re-solve in one call.
type ReoptimizeRequest struct {
	Operations []EditOperation `json:"operations,omitempty"`
	Solver     string          `json:"solver,omitempty"`
	BudgetMS   int             `json:"budgetMs,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// ApplyOperations mutates a layout in place. It returns the number of
// operations applied and any rule violations (rotation not permitted, a piece
// dragged into the trim strip). Unknown placement ids are an error: silently
// ignoring them would show the planner a layout the server never stored.
func ApplyOperations(sol *core.Solution, problem core.Problem, ops []EditOperation) (int, []validator.Violation, error) {
	index := map[string]*core.Placement{}
	for si := range sol.Sheets {
		for pi := range sol.Sheets[si].Placements {
			pl := &sol.Sheets[si].Placements[pi]
			if pl.ID != "" {
				index[pl.ID] = pl
			}
		}
	}

	applied := 0
	var violations []validator.Violation

	// Pass 1: moves, rotations and lock changes.
	for _, op := range ops {
		if op.Delete {
			continue
		}
		pl, ok := index[op.PlacementID]
		if !ok {
			return applied, violations, fmt.Errorf("unknown placement id %q", op.PlacementID)
		}
		if op.X != nil {
			pl.X = *op.X
		}
		if op.Y != nil {
			pl.Y = *op.Y
		}
		if op.Rotated != nil && *op.Rotated != pl.Rotated {
			part, known := partFor(problem, *pl)
			switch {
			case !known:
				return applied, violations, fmt.Errorf("placement %q does not match a requested part", op.PlacementID)
			case part.Width <= 0 || part.Height <= 0:
				violations = append(violations, editViolation("not_rotatable", *pl,
					"1d pieces cannot be rotated"))
			case !problem.Rules.AllowRotate || !part.AllowRotate:
				violations = append(violations, editViolation("rotation_not_allowed", *pl,
					"rotation is not permitted for %s by the rules", part.Code))
			case part.Grain != core.GrainNone:
				violations = append(violations, editViolation("rotation_not_allowed", *pl,
					"%s requires grain direction %q and cannot be rotated", part.Code, part.Grain))
			default:
				pl.W, pl.H = pl.H, pl.W
				pl.Rotated = *op.Rotated
			}
		}
		if op.Locked != nil {
			pl.Locked = *op.Locked
		}
		applied++
	}

	// Pass 2: deletions (rebuilding the index avoids stale pointers after a
	// slice shift).
	for _, op := range ops {
		if !op.Delete {
			continue
		}
		found := false
		for si := range sol.Sheets {
			placements := sol.Sheets[si].Placements
			for pi := range placements {
				if placements[pi].ID != op.PlacementID {
					continue
				}
				sol.Sheets[si].Placements = append(placements[:pi], placements[pi+1:]...)
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			return applied, violations, fmt.Errorf("unknown placement id %q", op.PlacementID)
		}
		applied++
	}

	violations = append(violations, trimViolations(problem, *sol)...)
	return applied, violations, nil
}

// partFor finds the requested part behind a placement, by id first and code as
// a fallback for placements archived before part ids were stored.
func partFor(problem core.Problem, pl core.Placement) (core.Part, bool) {
	if pl.PartID != "" {
		for _, part := range problem.Parts {
			if part.ID == pl.PartID {
				return part, true
			}
		}
	}
	for _, part := range problem.Parts {
		if part.Code == pl.PartCode {
			return part, true
		}
	}
	return core.Part{}, false
}

// trimViolations enforces the edge trim on edited layouts. The validator checks
// bounds against the whole sheet; a planner dragging a piece must also keep the
// trim strip clear.
func trimViolations(problem core.Problem, sol core.Solution) []validator.Violation {
	trim := problem.Rules.Trim
	if trim <= 0 {
		return nil
	}
	is2D := core.DetectProfile(problem) == core.Profile2D
	var out []validator.Violation
	for _, sheet := range sol.Sheets {
		for _, pl := range sheet.Placements {
			if pl.X < trim || pl.X+pl.W > sheet.Width-trim {
				out = append(out, editViolation("out_of_trim", pl,
					"%s touches the edge trim (keep pieces between %s and %s mm from the edges)",
					pl.PartCode, mmText(trim), mmText(sheet.Width-trim)))
				continue
			}
			if is2D && (pl.Y < trim || pl.Y+pl.H > sheet.Height-trim) {
				out = append(out, editViolation("out_of_trim", pl,
					"%s touches the edge trim (keep pieces between %s and %s mm from the edges)",
					pl.PartCode, mmText(trim), mmText(sheet.Height-trim)))
			}
		}
	}
	return out
}

func editViolation(code string, pl core.Placement, format string, args ...any) validator.Violation {
	return validator.Violation{
		Severity: "error",
		Code:     code,
		PartCode: pl.PartCode,
		Message:  fmt.Sprintf(format, args...),
	}
}

func mmText(d core.Dim) string {
	whole := d / core.Millimeter
	frac := (d % core.Millimeter) / 10
	if frac == 0 {
		return itoa(whole)
	}
	s := itoa(frac)
	if frac < 10 {
		s = "0" + s
	}
	return itoa(whole) + "." + s
}

func itoa(v core.Dim) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// HasErrors reports whether a violation list contains at least one error.
func HasErrors(violations []validator.Violation) bool {
	for _, v := range violations {
		if v.Severity == "error" {
			return true
		}
	}
	return false
}

// ResolvePartIDs fills placement part ids from the problem snapshot by part
// code; archived placements only store the code, and the validator needs the id
// to check orientation and demand.
func ResolvePartIDs(sheets []core.SheetPlan, problem core.Problem) {
	byCode := map[string]string{}
	for _, part := range problem.Parts {
		if _, exists := byCode[part.Code]; !exists {
			byCode[part.Code] = part.ID
		}
	}
	for si := range sheets {
		for pi := range sheets[si].Placements {
			pl := &sheets[si].Placements[pi]
			if pl.PartID == "" {
				pl.PartID = byCode[pl.PartCode]
			}
		}
	}
}

// SynthesizeUnplaced reports the demand a layout does not produce, so an edited
// or re-solved plan always carries an honest shortfall list.
func SynthesizeUnplaced(problem core.Problem, sol core.Solution) []core.UnplacedPart {
	placed := map[string]int{}
	for _, sheet := range sol.Sheets {
		for _, pl := range sheet.Placements {
			key := pl.PartID
			if key == "" {
				key = pl.PartCode
			}
			placed[key]++
		}
	}
	var out []core.UnplacedPart
	for _, part := range problem.Parts {
		key := part.ID
		if key == "" {
			key = part.Code
		}
		missing := part.Quantity - placed[key]
		if missing <= 0 {
			continue
		}
		out = append(out, core.UnplacedPart{
			PartID:   part.ID,
			PartCode: part.Code,
			Quantity: missing,
			Reason:   "not placed in this plan version",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PartCode < out[j].PartCode })
	return out
}

// Finalize recomputes the demand, metrics and notes of a layout after an edit.
// The solve time of the previous version is kept so the scorecard stays honest.
func Finalize(problem core.Problem, sol *core.Solution) {
	ResolvePartIDs(sol.Sheets, problem)
	elapsed := sol.Metrics.ElapsedMS
	sol.Unplaced = SynthesizeUnplaced(problem, *sol)
	sol.Metrics = core.Summarize(problem, sol.Sheets, sol.Unplaced, elapsed)
}

// BuildPinnedProblem prepares a re-solve: locked placements become pinned
// sheets and the stock copies they occupy are removed from the free pool, so
// the solver fills everything around them.
func BuildPinnedProblem(problem core.Problem, sol core.Solution) core.Problem {
	p := problem
	p.Pinned = nil
	used := map[string]int{}
	for _, sheet := range sol.Sheets {
		var locked []core.Placement
		for _, pl := range sheet.Placements {
			if pl.Locked {
				locked = append(locked, pl)
			}
		}
		if len(locked) == 0 {
			continue
		}
		p.Pinned = append(p.Pinned, core.PinnedSheet{
			StockID:    sheet.StockID,
			StockCode:  sheet.StockCode,
			Label:      sheet.Label,
			Width:      sheet.Width,
			Height:     sheet.Height,
			Placements: locked,
		})
		if sheet.StockID != "" {
			used[sheet.StockID]++
		}
	}
	for i := range p.Stocks {
		if n := used[p.Stocks[i].ID]; n > 0 {
			p.Stocks[i].Quantity -= n
			if p.Stocks[i].Quantity < 0 {
				p.Stocks[i].Quantity = 0
			}
		}
	}
	return p
}
