package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/size-module/backend/internal/modules/catalog"
	"github.com/size-module/backend/internal/modules/jobs"
	"github.com/size-module/backend/internal/modules/parts"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

// Store implements the module store interfaces on top of PostgreSQL.
type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, queries: db.New(pool)}
}

// ---------------------------------------------------------------- catalog ---

func (s *Store) ListMaterials(ctx context.Context) ([]catalog.Material, error) {
	rows, err := s.queries.ListMaterials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.Material, 0, len(rows))
	for _, m := range rows {
		out = append(out, catalog.Material{
			ID:               m.ID.String(),
			Code:             m.Code,
			Name:             m.Name,
			DimensionProfile: m.DimensionProfile,
			IsActive:         m.IsActive,
		})
	}
	return out, nil
}

func (s *Store) CreateMaterial(ctx context.Context, in catalog.CreateMaterialInput) (catalog.Material, error) {
	plant, err := s.queries.GetDefaultPlant(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.Material{}, errors.New("no plant configured; run db/scripts/seed.ps1")
		}
		return catalog.Material{}, err
	}
	row, err := s.queries.CreateMaterial(ctx, db.CreateMaterialParams{
		PlantID:          plant.ID,
		Code:             in.Code,
		Name:             in.Name,
		DimensionProfile: in.DimensionProfile,
		Attributes:       marshalJSON(in.Attributes),
	})
	if err != nil {
		return catalog.Material{}, err
	}
	return catalog.Material{
		ID:               row.ID.String(),
		Code:             row.Code,
		Name:             row.Name,
		DimensionProfile: row.DimensionProfile,
		IsActive:         row.IsActive,
	}, nil
}

func (s *Store) ListStockFormats(ctx context.Context) ([]catalog.StockFormat, error) {
	rows, err := s.queries.ListStockFormats(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.StockFormat, 0, len(rows))
	for _, r := range rows {
		out = append(out, catalog.StockFormat{
			ID:               r.ID.String(),
			Code:             r.Code,
			MaterialCode:     r.MaterialCode,
			MaterialName:     r.MaterialName,
			SpecCode:         r.SpecCode,
			DimensionProfile: r.DimensionProfile,
			ThicknessMicron:  r.ThicknessUm,
			LengthMicron:     r.LengthUm,
			WidthMicron:      r.WidthUm,
			HeightMicron:     r.HeightUm,
			OnHandQty:        r.OnHandQty,
			CostPerUnit:      r.CostPerUnit,
		})
	}
	return out, nil
}

// ------------------------------------------------------------------ parts ---

func (s *Store) ListParts(ctx context.Context) ([]parts.Part, error) {
	rows, err := s.queries.ListParts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]parts.Part, 0, len(rows))
	for _, p := range rows {
		out = append(out, toPartDTO(p))
	}
	return out, nil
}

func (s *Store) CreatePart(ctx context.Context, in parts.CreatePartInput) (parts.Part, error) {
	plant, err := s.queries.GetDefaultPlant(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return parts.Part{}, errors.New("no plant configured; run db/scripts/seed.ps1")
		}
		return parts.Part{}, err
	}

	var specID pgtype.UUID
	if in.MaterialSpecID != "" {
		parsed, err := uuid.Parse(in.MaterialSpecID)
		if err != nil {
			return parts.Part{}, fmt.Errorf("materialSpecId is not a valid uuid: %w", err)
		}
		specID = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	allowRotate := true
	if in.AllowRotate != nil {
		allowRotate = *in.AllowRotate
	}

	row, err := s.queries.CreatePart(ctx, db.CreatePartParams{
		PlantID:          plant.ID,
		MaterialSpecID:   specID,
		Code:             in.Code,
		Name:             in.Name,
		FinishedLengthUm: in.FinishedLengthMicron,
		FinishedWidthUm:  in.FinishedWidthMicron,
		FinishedHeightUm: in.FinishedHeightMicron,
		Grain:            in.Grain,
		AllowRotate:      allowRotate,
		Priority:         in.Priority,
		Attributes:       marshalJSON(nil),
	})
	if err != nil {
		return parts.Part{}, err
	}
	return toPartDTO(row), nil
}

// ------------------------------------------------------------------- jobs ---

// SaveRun archives an optimization run and its plan in one transaction.
func (s *Store) SaveRun(ctx context.Context, req jobs.SaveRunRequest) (jobs.SavedRun, error) {
	plant, err := s.queries.GetDefaultPlant(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return jobs.SavedRun{}, errors.New("no plant configured; run db/scripts/seed.ps1")
		}
		return jobs.SavedRun{}, err
	}

	inputJSON := marshalJSON(req.Problem)
	resultJSON := marshalJSON(req.Result)

	budget := req.Problem.BudgetMS
	if budget <= 0 {
		budget = 5000
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return jobs.SavedRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	job, err := q.CreateJob(ctx, db.CreateJobParams{
		PlantID:   plant.ID,
		Solver:    req.Solver,
		Input:     inputJSON,
		Seed:      int64(req.Problem.Seed),
		BudgetMs:  int32(budget),
		CreatedBy: pgtype.UUID{},
	})
	if err != nil {
		return jobs.SavedRun{}, err
	}
	if err := q.MarkJobRunning(ctx, job.ID); err != nil {
		return jobs.SavedRun{}, err
	}

	planID, err := writePlan(ctx, q, plant.ID, job.ID, req.Problem, req.Result)
	if err != nil {
		return jobs.SavedRun{}, err
	}

	if err := q.MarkJobDone(ctx, db.MarkJobDoneParams{ID: job.ID, Result: resultJSON}); err != nil {
		return jobs.SavedRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return jobs.SavedRun{}, err
	}
	return jobs.SavedRun{JobID: job.ID.String(), PlanID: planID.String()}, nil
}

// ---------------------------------------------------------------- helpers ---

func toPartDTO(p db.Part) parts.Part {
	out := parts.Part{
		ID:                   p.ID.String(),
		Code:                 p.Code,
		Name:                 p.Name,
		FinishedLengthMicron: p.FinishedLengthUm,
		FinishedWidthMicron:  p.FinishedWidthUm,
		FinishedHeightMicron: p.FinishedHeightUm,
		Grain:                p.Grain,
		AllowRotate:          p.AllowRotate,
		Priority:             p.Priority,
	}
	if p.MaterialSpecID.Valid {
		out.MaterialSpecID = uuid.UUID(p.MaterialSpecID.Bytes).String()
	}
	return out
}

func marshalJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return data
}

func orEmptyStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func orEmptyRects(v []core.Rect) []core.Rect {
	if v == nil {
		return []core.Rect{}
	}
	return v
}
