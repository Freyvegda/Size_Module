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
			MaterialSpecID:   r.MaterialSpecID.String(),
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

// --------------------------------------------------------- material specs ---

func (s *Store) ListMaterialSpecs(ctx context.Context) ([]catalog.MaterialSpec, error) {
	rows, err := s.queries.ListMaterialSpecOptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]catalog.MaterialSpec, 0, len(rows))
	for _, r := range rows {
		out = append(out, catalog.MaterialSpec{
			ID:               r.ID.String(),
			MaterialID:       r.MaterialID.String(),
			MaterialCode:     r.MaterialCode,
			MaterialName:     r.MaterialName,
			DimensionProfile: r.DimensionProfile,
			Code:             r.Code,
			Name:             r.Name,
			ThicknessMicron:  r.ThicknessUm,
			Finish:           r.Finish,
			Color:            r.Color,
		})
	}
	return out, nil
}

func (s *Store) CreateMaterialSpec(ctx context.Context, in catalog.CreateMaterialSpecInput) (catalog.MaterialSpec, error) {
	materialID, err := uuid.Parse(in.MaterialID)
	if err != nil {
		return catalog.MaterialSpec{}, fmt.Errorf("materialId is not a valid uuid: %w", err)
	}
	row, err := s.queries.CreateMaterialSpec(ctx, db.CreateMaterialSpecParams{
		MaterialID:  materialID,
		Code:        in.Code,
		Name:        in.Name,
		ThicknessUm: in.ThicknessMicron,
		Finish:      in.Finish,
		Color:       in.Color,
		Attributes:  marshalJSON(nil),
	})
	if err != nil {
		return catalog.MaterialSpec{}, err
	}
	// Enrich with the family so the response mirrors the list shape.
	material, err := s.queries.GetMaterial(ctx, row.MaterialID)
	if err != nil {
		return catalog.MaterialSpec{}, err
	}
	return catalog.MaterialSpec{
		ID:               row.ID.String(),
		MaterialID:       row.MaterialID.String(),
		MaterialCode:     material.Code,
		MaterialName:     material.Name,
		DimensionProfile: material.DimensionProfile,
		Code:             row.Code,
		Name:             row.Name,
		ThicknessMicron:  row.ThicknessUm,
		Finish:           row.Finish,
		Color:            row.Color,
	}, nil
}

func (s *Store) CreateStockFormat(ctx context.Context, in catalog.CreateStockFormatInput) (catalog.StockFormat, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return catalog.StockFormat{}, err
	}
	specID, err := uuid.Parse(in.MaterialSpecID)
	if err != nil {
		return catalog.StockFormat{}, fmt.Errorf("materialSpecId is not a valid uuid: %w", err)
	}
	// A format must match the dimension profile of its material: bars carry a
	// length, sheets a width and a height. Refusing here keeps solvers from
	// receiving a "sheet" with no area or a "bar" with no length.
	profile, ok, err := s.materialSpecProfile(ctx, specID)
	if err != nil {
		return catalog.StockFormat{}, err
	}
	if !ok {
		return catalog.StockFormat{}, errors.New("materialSpecId does not exist")
	}
	switch profile {
	case "1d":
		if in.LengthMicron <= 0 {
			return catalog.StockFormat{}, errors.New("a 1d material needs a length")
		}
	case "2d":
		if in.WidthMicron <= 0 || in.HeightMicron <= 0 {
			return catalog.StockFormat{}, errors.New("a 2d material needs a width and a height")
		}
	default:
		return catalog.StockFormat{}, fmt.Errorf("material profile %q cannot be stocked yet", profile)
	}
	row, err := s.queries.CreateStockFormat(ctx, db.CreateStockFormatParams{
		PlantID:        plant.ID,
		MaterialSpecID: specID,
		Code:           in.Code,
		LengthUm:       in.LengthMicron,
		WidthUm:        in.WidthMicron,
		HeightUm:       in.HeightMicron,
		OnHandQty:      in.OnHandQty,
		CostPerUnit:    in.CostPerUnit,
	})
	if err != nil {
		return catalog.StockFormat{}, err
	}
	detail, err := s.queries.GetStockFormatDetail(ctx, row.ID)
	if err != nil {
		return catalog.StockFormat{}, err
	}
	return catalog.StockFormat{
		ID:               detail.ID.String(),
		Code:             detail.Code,
		MaterialSpecID:   detail.MaterialSpecID.String(),
		MaterialCode:     detail.MaterialCode,
		MaterialName:     detail.MaterialName,
		SpecCode:         detail.SpecCode,
		DimensionProfile: detail.DimensionProfile,
		ThicknessMicron:  detail.ThicknessUm,
		LengthMicron:     detail.LengthUm,
		WidthMicron:      detail.WidthUm,
		HeightMicron:     detail.HeightUm,
		OnHandQty:        detail.OnHandQty,
		CostPerUnit:      detail.CostPerUnit,
	}, nil
}

// ------------------------------------------------------------------ parts ---

func (s *Store) ListParts(ctx context.Context) ([]parts.Part, error) {
	rows, err := s.queries.ListParts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]parts.Part, 0, len(rows))
	for _, p := range rows {
		routings, err := s.partRoutings(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, toPartDTO(p, routings))
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
		// Reject an unknown spec here so the caller gets a clear error instead
		// of a foreign-key violation.
		if _, ok, err := s.materialSpecProfile(ctx, parsed); err != nil {
			return parts.Part{}, err
		} else if !ok {
			return parts.Part{}, errors.New("materialSpecId does not exist")
		}
		specID = pgtype.UUID{Bytes: parsed, Valid: true}
	}

	allowRotate := true
	if in.AllowRotate != nil {
		allowRotate = *in.AllowRotate
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return parts.Part{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	row, err := q.CreatePart(ctx, db.CreatePartParams{
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

	routings := make([]parts.Routing, 0, len(in.Routings))
	for _, rt := range in.Routings {
		seq := rt.Seq
		if seq <= 0 {
			seq = int32(len(routings) + 1)
		}
		if _, err := q.CreatePartRouting(ctx, db.CreatePartRoutingParams{
			PartID:           row.ID,
			Seq:              seq,
			Operation:        rt.Operation,
			AllowanceUm:      rt.AllowanceMicron,
			AllowancePerEdge: rt.AllowancePerEdge,
			Notes:            rt.Notes,
		}); err != nil {
			return parts.Part{}, err
		}
		routings = append(routings, parts.Routing{
			Seq:              seq,
			Operation:        rt.Operation,
			AllowanceMicron:  rt.AllowanceMicron,
			AllowancePerEdge: rt.AllowancePerEdge,
			Notes:            rt.Notes,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return parts.Part{}, err
	}
	return toPartDTO(row, routings), nil
}

// partRoutings loads a part's operations in sequence order.
func (s *Store) partRoutings(ctx context.Context, partID uuid.UUID) ([]parts.Routing, error) {
	rows, err := s.queries.ListPartRoutings(ctx, partID)
	if err != nil {
		return nil, err
	}
	out := make([]parts.Routing, 0, len(rows))
	for _, r := range rows {
		out = append(out, parts.Routing{
			Seq:              r.Seq,
			Operation:        r.Operation,
			AllowanceMicron:  r.AllowanceUm,
			AllowancePerEdge: r.AllowancePerEdge,
			Notes:            r.Notes,
		})
	}
	return out, nil
}

// materialSpecProfile looks up a material spec's dimension profile. The second
// return value is false when the spec does not exist.
func (s *Store) materialSpecProfile(ctx context.Context, specID uuid.UUID) (string, bool, error) {
	specs, err := s.queries.ListMaterialSpecOptions(ctx)
	if err != nil {
		return "", false, err
	}
	for _, spec := range specs {
		if spec.ID == specID {
			return spec.DimensionProfile, true, nil
		}
	}
	return "", false, nil
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

	planID, err := writePlan(ctx, q, planParams{
		PlantID: plant.ID,
		JobID:   pgtypeUUID(job.ID),
		Version: 1,
		Status:  "draft",
		Name:    "Optimization run",
	}, req.Problem, req.Result)
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

func toPartDTO(p db.Part, routings []parts.Routing) parts.Part {
	cutLength, cutWidth, cutHeight := parts.CutSize(
		p.FinishedLengthUm, p.FinishedWidthUm, p.FinishedHeightUm, routings)
	out := parts.Part{
		ID:                   p.ID.String(),
		Code:                 p.Code,
		Name:                 p.Name,
		FinishedLengthMicron: p.FinishedLengthUm,
		FinishedWidthMicron:  p.FinishedWidthUm,
		FinishedHeightMicron: p.FinishedHeightUm,
		CutLengthMicron:      cutLength,
		CutWidthMicron:       cutWidth,
		CutHeightMicron:      cutHeight,
		Grain:                p.Grain,
		AllowRotate:          p.AllowRotate,
		Priority:             p.Priority,
		Routings:             routings,
	}
	if out.Routings == nil {
		out.Routings = []parts.Routing{}
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
