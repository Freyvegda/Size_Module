package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

// pgtypeUUID converts a google/uuid into the form sqlc emits for uuid columns;
// the zero uuid means NULL.
func pgtypeUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// physicalStockItem reports the stock_items id behind a stock reference, if
// there is one. Format ids are uuids too, so a physical piece is only linked
// after the pool confirms it exists.
func physicalStockItem(ctx context.Context, q *db.Queries, stockID string) (pgtype.UUID, error) {
	if stockID == "" {
		return pgtype.UUID{}, nil
	}
	parsed, err := uuid.Parse(stockID)
	if err != nil {
		return pgtype.UUID{}, nil
	}
	if _, err := q.GetStockItem(ctx, parsed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

// planParams are the row-level fields of a plan version.
type planParams struct {
	PlantID      uuid.UUID
	JobID        pgtype.UUID
	ParentPlanID pgtype.UUID
	Version      int32
	Status       string
	Name         string
}

// writePlan stores one finished optimization result as a plan with its sheets
// and placements. It is shared by the synchronous archive path (SaveRun), the
// asynchronous worker (Complete) and plan versioning (SaveVersion), so all
// paths produce identical rows.
func writePlan(ctx context.Context, q *db.Queries, params planParams, problem core.Problem, result optimizer.Result) (uuid.UUID, error) {
	plan, err := q.CreatePlan(ctx, db.CreatePlanParams{
		PlantID:       params.PlantID,
		JobID:         params.JobID,
		ParentPlanID:  params.ParentPlanID,
		Version:       params.Version,
		Status:        params.Status,
		Name:          params.Name,
		Solver:        result.Solution.Solver,
		SolverVersion: result.Solution.SolverVersion,
		Seed:          int64(result.Solution.Seed),
		Rules:         marshalJSON(problem.Rules),
		Metrics:       marshalJSON(result.Solution.Metrics),
		Notes:         marshalJSON(orEmptyStrings(result.Solution.Notes)),
	})
	if err != nil {
		return uuid.Nil, err
	}

	for _, sheet := range result.Solution.Sheets {
		stockItemID, err := physicalStockItem(ctx, q, sheet.StockID)
		if err != nil {
			return uuid.Nil, err
		}
		sheetRow, err := q.CreatePlanSheet(ctx, db.CreatePlanSheetParams{
			PlanID:      plan.ID,
			SheetIndex:  int32(sheet.Index),
			StockCode:   sheet.StockCode,
			StockID:     sheet.StockID,
			StockItemID: stockItemID,
			Label:       sheet.Label,
			WidthUm:     sheet.Width,
			HeightUm:    sheet.Height,
			Offcuts:     marshalJSON(orEmptyRects(sheet.Offcuts)),
			CutSteps:    marshalJSON(orEmptyStrings(sheet.CutSteps)),
		})
		if err != nil {
			return uuid.Nil, err
		}
		for i, pl := range sheet.Placements {
			if _, err := q.CreatePlacement(ctx, db.CreatePlacementParams{
				PlanSheetID: sheetRow.ID,
				PartID:      pgtypeUUID(uuid.Nil),
				PartCode:    pl.PartCode,
				XUm:         pl.X,
				YUm:         pl.Y,
				WUm:         pl.W,
				HUm:         pl.H,
				Rotated:     pl.Rotated,
				Locked:      pl.Locked,
				Seq:         int32(i),
			}); err != nil {
				return uuid.Nil, err
			}
		}
	}
	return plan.ID, nil
}
