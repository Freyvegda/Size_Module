package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/modules/plans"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/optimizer/validator"
	"github.com/size-module/backend/internal/platform/db"
)

// ListPlans returns archived plans, newest first.
func (s *Store) ListPlans(ctx context.Context, limit, offset int) ([]plans.Summary, error) {
	rows, err := s.queries.ListPlans(ctx, db.ListPlansParams{Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := make([]plans.Summary, 0, len(rows))
	for _, r := range rows {
		out = append(out, planSummary(r))
	}
	return out, nil
}

// GetPlan rebuilds an archived plan into the same shape the optimizer returns,
// so the plan viewer can render a stored run without a second endpoint. When
// the linked job input is available the layout is re-validated and re-scored.
func (s *Store) GetPlan(ctx context.Context, planID string) (plans.Detail, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return plans.Detail{}, plans.ErrNotFound
	}
	row, err := s.queries.GetPlan(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return plans.Detail{}, plans.ErrNotFound
	}
	if err != nil {
		return plans.Detail{}, err
	}

	solution := core.Solution{
		Solver:        row.Solver,
		SolverVersion: row.SolverVersion,
		Seed:          uint64(row.Seed),
	}
	if len(row.Metrics) > 0 {
		_ = json.Unmarshal(row.Metrics, &solution.Metrics)
	}
	if len(row.Notes) > 0 {
		_ = json.Unmarshal(row.Notes, &solution.Notes)
	}

	sheets, err := s.queries.ListPlanSheets(ctx, id)
	if err != nil {
		return plans.Detail{}, err
	}
	for _, sh := range sheets {
		plan := core.SheetPlan{
			Index:     int(sh.SheetIndex),
			StockID:   sh.StockID,
			StockCode: sh.StockCode,
			Label:     sh.Label,
			Width:     sh.WidthUm,
			Height:    sh.HeightUm,
		}
		if len(sh.Offcuts) > 0 {
			_ = json.Unmarshal(sh.Offcuts, &plan.Offcuts)
		}
		if len(sh.CutSteps) > 0 {
			_ = json.Unmarshal(sh.CutSteps, &plan.CutSteps)
		}
		placements, err := s.queries.ListPlacements(ctx, sh.ID)
		if err != nil {
			return plans.Detail{}, err
		}
		for _, p := range placements {
			placement := core.Placement{
				PartCode: p.PartCode,
				X:        p.XUm,
				Y:        p.YUm,
				W:        p.WUm,
				H:        p.HUm,
				Rotated:  p.Rotated,
			}
			if p.PartID.Valid {
				placement.PartID = uuid.UUID(p.PartID.Bytes).String()
			}
			plan.Placements = append(plan.Placements, placement)
		}
		solution.Sheets = append(solution.Sheets, plan)
	}

	result := optimizer.Result{Solution: solution}
	problem, archived, hasSnapshot := s.planSnapshot(ctx, s.queries, row.JobID)
	if hasSnapshot && archived != nil {
		// The archived result is the exact answer the solver produced
		// (including unplaced demand and violations); prefer it over a
		// reconstruction, which cannot recover everything.
		return plans.Detail{Summary: planSummary(row), Result: *archived}, nil
	}
	if hasSnapshot {
		resolvePartIDs(solution.Sheets, problem)
		result.Violations = validator.Validate(problem, solution)
		result.Score = core.Score(problem, solution.Metrics)
	}
	return plans.Detail{Summary: planSummary(row), Result: result}, nil
}

// resolvePartIDs fills placement part ids from the problem snapshot by part
// code; archived placements only store the code, and the validator needs the
// id to check orientation and demand.
func resolvePartIDs(sheets []core.SheetPlan, problem core.Problem) {
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

// AcceptPlan freezes a plan and applies it to the shop: physical pieces are
// consumed, catalog on-hand quantities drop, and every reusable offcut becomes
// a labelled remnant in the stock pool. Everything happens in one transaction:
// either the shop state matches the accepted plan, or nothing changed.
func (s *Store) AcceptPlan(ctx context.Context, planID string) (plans.AcceptResult, error) {
	id, err := uuid.Parse(planID)
	if err != nil {
		return plans.AcceptResult{}, plans.ErrNotFound
	}
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return plans.AcceptResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plans.AcceptResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	planRow, err := q.GetPlanForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return plans.AcceptResult{}, plans.ErrNotFound
	}
	if err != nil {
		return plans.AcceptResult{}, err
	}
	if planRow.Status != "draft" && planRow.Status != "approved" {
		return plans.AcceptResult{}, plans.ErrNotAcceptable
	}

	problem, _, _ := s.planSnapshot(ctx, q, planRow.JobID)
	stockByID := map[string]core.StockItem{}
	for _, item := range problem.Stocks {
		stockByID[item.ID] = item
	}
	rules := core.DefaultRules()
	if len(planRow.Rules) > 0 {
		var stored core.Rules
		if err := json.Unmarshal(planRow.Rules, &stored); err == nil && stored.CutMode != "" {
			rules = stored
		}
	}

	sheets, err := q.ListPlanSheets(ctx, id)
	if err != nil {
		return plans.AcceptResult{}, err
	}

	result := plans.AcceptResult{PlanID: planID, Sheets: len(sheets)}
	for _, sh := range sheets {
		is1D := sheetIs1D(sh, stockByID, planRow.Solver)

		format, err := s.sheetFormat(ctx, q, sh)
		if err != nil {
			return plans.AcceptResult{}, err
		}

		if sh.StockItemID.Valid {
			item, err := q.GetStockItem(ctx, sh.StockItemID.Bytes)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return plans.AcceptResult{}, err
			}
			consumed, err := q.ConsumeStockItem(ctx, db.ConsumeStockItemParams{
				ID:               sh.StockItemID.Bytes,
				ConsumedByPlanID: pgtypeUUID(id),
			})
			if err != nil {
				return plans.AcceptResult{}, err
			}
			if consumed > 0 {
				result.StockConsumed = append(result.StockConsumed, item.Label)
			}
		} else if format != nil {
			if format.OnHandQty > 0 {
				if err := q.TakeStockOnHand(ctx, format.ID); err != nil {
					return plans.AcceptResult{}, err
				}
				result.FormatDecrements++
			} else {
				result.FormatShortages++
			}
		}

		var offcuts []core.Rect
		if len(sh.Offcuts) > 0 {
			if err := json.Unmarshal(sh.Offcuts, &offcuts); err != nil {
				return plans.AcceptResult{}, fmt.Errorf("sheet %d: invalid offcuts: %w", sh.SheetIndex, err)
			}
		}
		for i, off := range offcuts {
			if !offcutReusable(off, is1D, rules) {
				continue
			}
			dims := remnantDims(off, is1D, format)
			row, err := q.CreateStockItem(ctx, db.CreateStockItemParams{
				PlantID:          plant.ID,
				FormatID:         formatUUID(format),
				Code:             sh.StockCode,
				Label:            offcutLabel(planID, int(sh.SheetIndex), i),
				LengthUm:         dims.length,
				WidthUm:          dims.width,
				HeightUm:         dims.height,
				IsRemnant:        true,
				Status:           "available",
				CostPerUnit:      remnantCost(off, is1D, format),
				ParentPlanID:     pgtypeUUID(id),
				ParentSheetIndex: int32Ptr(sh.SheetIndex),
			})
			if err != nil {
				return plans.AcceptResult{}, err
			}
			result.RemnantsCreated = append(result.RemnantsCreated, plans.RemnantRef{
				ID:           row.ID.String(),
				Label:        row.Label,
				LengthMicron: row.LengthUm,
				WidthMicron:  row.WidthUm,
				HeightMicron: row.HeightUm,
			})
		}
	}

	updated, err := q.AcceptPlan(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return plans.AcceptResult{}, plans.ErrNotAcceptable
	}
	if err != nil {
		return plans.AcceptResult{}, err
	}
	result.Status = updated.Status

	if err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		PlantID:  pgtypeUUID(plant.ID),
		Action:   "plan.accept",
		Entity:   "plan",
		EntityID: planID,
		Payload:  marshalJSON(result),
	}); err != nil {
		return plans.AcceptResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plans.AcceptResult{}, err
	}
	return result, nil
}

// sheetFormat resolves the catalog format behind a plan sheet: a physical piece
// points at its own format, a format-based sheet points at the stock id.
func (s *Store) sheetFormat(ctx context.Context, q *db.Queries, sh db.PlanSheet) (*db.StockFormat, error) {
	if sh.StockItemID.Valid {
		item, err := q.GetStockItem(ctx, sh.StockItemID.Bytes)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !item.FormatID.Valid {
			return nil, nil
		}
		format, err := q.GetStockFormat(ctx, item.FormatID.Bytes)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &format, nil
	}
	if sh.StockID == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(sh.StockID)
	if err != nil {
		return nil, nil
	}
	format, err := q.GetStockFormat(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &format, nil
}

// planSnapshot loads the job behind a plan: its problem snapshot and, when the
// result was archived, the original optimizer result.
func (s *Store) planSnapshot(ctx context.Context, q *db.Queries, jobID pgtype.UUID) (core.Problem, *optimizer.Result, bool) {
	if !jobID.Valid {
		return core.Problem{}, nil, false
	}
	job, err := q.GetJob(ctx, jobID.Bytes)
	if err != nil {
		return core.Problem{}, nil, false
	}
	problem, err := decodeProblem(job.Input)
	if err != nil {
		return core.Problem{}, nil, false
	}
	var archived optimizer.Result
	if len(job.Result) > 0 {
		if err := json.Unmarshal(job.Result, &archived); err == nil && len(archived.Solution.Sheets) > 0 {
			return problem, &archived, true
		}
	}
	return problem, nil, true
}

func planSummary(row db.Plan) plans.Summary {
	summary := plans.Summary{
		ID:            row.ID.String(),
		Status:        row.Status,
		Version:       int(row.Version),
		Name:          row.Name,
		Solver:        row.Solver,
		SolverVersion: row.SolverVersion,
		Seed:          uint64(row.Seed),
		CreatedAt:     row.CreatedAt.Time,
	}
	if row.JobID.Valid {
		summary.JobID = uuid.UUID(row.JobID.Bytes).String()
	}
	if len(row.Metrics) > 0 {
		_ = json.Unmarshal(row.Metrics, &summary.Metrics)
	}
	if row.AcceptedAt.Valid {
		accepted := row.AcceptedAt.Time
		summary.AcceptedAt = &accepted
	}
	return summary
}

// sheetIs1D decides whether a sheet is a bar. The problem snapshot is
// authoritative (1d stock carries a Length), the solver name is the next best
// hint; without either, a very shallow and very wide sheet is a bar, which is
// how the 1d solver represents them.
func sheetIs1D(sh db.PlanSheet, stocks map[string]core.StockItem, solver string) bool {
	if stock, ok := stocks[sh.StockID]; ok {
		return stock.Length > 0
	}
	if strings.HasSuffix(solver, "-1d") {
		return true
	}
	return sh.HeightUm <= 2*core.Millimeter && sh.WidthUm > 10*sh.HeightUm
}

type remnantShape struct {
	length, width, height int64
}

// remnantDims maps an offcut rectangle to a physical piece. 1d offcuts keep the
// bar width for display; 2d offcuts keep width x height.
func remnantDims(off core.Rect, is1D bool, format *db.StockFormat) remnantShape {
	if is1D {
		shape := remnantShape{length: off.W}
		if format != nil {
			shape.width = format.WidthUm
		}
		return shape
	}
	return remnantShape{width: off.W, height: off.H}
}

// offcutReusable re-applies the offcut policy stored on the plan, so accepting
// a plan whose rules changed cannot turn scrap into fake stock. A zero minimum
// means "any positive leftover".
func offcutReusable(off core.Rect, is1D bool, rules core.Rules) bool {
	if off.W <= 0 || off.H <= 0 {
		return false
	}
	if is1D {
		return off.W >= rules.OffcutMinLength
	}
	return off.W >= rules.OffcutMinW && off.H >= rules.OffcutMinH
}

// remnantCost prorates the format cost onto the leftover, so the optimizer can
// value remnant stock honestly instead of treating it as free.
func remnantCost(off core.Rect, is1D bool, format *db.StockFormat) float64 {
	if format == nil || format.CostPerUnit <= 0 {
		return 0
	}
	var share float64
	if is1D {
		if format.LengthUm <= 0 {
			return 0
		}
		share = float64(off.W) / float64(format.LengthUm)
	} else {
		area := float64(format.WidthUm) * float64(format.HeightUm)
		if area <= 0 {
			return 0
		}
		share = float64(off.W) * float64(off.H) / area
	}
	if share > 1 {
		share = 1
	}
	return format.CostPerUnit * share
}

// offcutLabel is the shop-floor identifier of a remnant: unique per plan,
// sheet and offcut, and short enough to write on the piece.
func offcutLabel(planID string, sheetIndex, offcutIndex int) string {
	short := planID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("OFF-%s-%02d-%d", strings.ToUpper(short), sheetIndex+1, offcutIndex+1)
}

func formatUUID(format *db.StockFormat) pgtype.UUID {
	if format == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: format.ID, Valid: true}
}

func int32Ptr(v int32) *int32 { return &v }
