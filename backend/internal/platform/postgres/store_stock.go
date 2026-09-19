package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/modules/stock"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

// ListItems returns the physical stock pool of the default plant.
func (s *Store) ListItems(ctx context.Context, filter stock.ListFilter) ([]stock.Item, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return nil, err
	}
	params := db.ListStockItemsParams{PlantID: plant.ID}
	if filter.Status != "" {
		params.Status = &filter.Status
	}
	params.IsRemnant = filter.IsRemnant
	rows, err := s.queries.ListStockItems(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]stock.Item, 0, len(rows))
	for _, r := range rows {
		out = append(out, toStockItem(r.ID, r.FormatID, r.Code, r.Label, r.LengthUm, r.WidthUm, r.HeightUm,
			r.IsRemnant, r.Status, r.Location, r.CostPerUnit, r.Notes, r.ParentPlanID, r.ParentSheetIndex,
			r.ConsumedByPlanID, r.ConsumedAt, r.CreatedAt, r.FormatCode, r.SpecCode, r.MaterialCode, r.DimensionProfile))
	}
	return out, nil
}

// GetItem returns one physical piece with its catalog lineage.
func (s *Store) GetItem(ctx context.Context, id string) (stock.Item, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return stock.Item{}, stock.ErrNotFound
	}
	r, err := s.queries.GetStockItem(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return stock.Item{}, stock.ErrNotFound
	}
	if err != nil {
		return stock.Item{}, err
	}
	return toStockItem(r.ID, r.FormatID, r.Code, r.Label, r.LengthUm, r.WidthUm, r.HeightUm,
		r.IsRemnant, r.Status, r.Location, r.CostPerUnit, r.Notes, r.ParentPlanID, r.ParentSheetIndex,
		r.ConsumedByPlanID, r.ConsumedAt, r.CreatedAt, r.FormatCode, r.SpecCode, r.MaterialCode, r.DimensionProfile), nil
}

// CreateItem registers a physical piece. When a format is referenced, missing
// dimensions, code and cost are inherited from the catalog size.
func (s *Store) CreateItem(ctx context.Context, in stock.CreateInput) (stock.Item, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return stock.Item{}, err
	}

	formatID := pgtype.UUID{}
	code := in.Code
	cost := in.CostPerUnit
	length, width, height := in.LengthMicron, in.WidthMicron, in.HeightMicron
	if in.FormatID != "" {
		parsed, err := uuid.Parse(in.FormatID)
		if err != nil {
			return stock.Item{}, fmt.Errorf("formatId is not a valid uuid: %w", err)
		}
		format, err := s.queries.GetStockFormat(ctx, parsed)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return stock.Item{}, fmt.Errorf("unknown formatId %s", in.FormatID)
			}
			return stock.Item{}, err
		}
		formatID = pgtype.UUID{Bytes: parsed, Valid: true}
		if code == "" {
			code = format.Code
		}
		if length == 0 {
			length = format.LengthUm
		}
		if width == 0 {
			width = format.WidthUm
		}
		if height == 0 {
			height = format.HeightUm
		}
		if cost == 0 {
			cost = format.CostPerUnit
		}
	}
	if code == "" {
		code = "MANUAL"
	}

	isRemnant := false
	if in.IsRemnant != nil {
		isRemnant = *in.IsRemnant
	}

	row, err := s.queries.CreateStockItem(ctx, db.CreateStockItemParams{
		PlantID:     plant.ID,
		FormatID:    formatID,
		Code:        code,
		Label:       in.Label,
		LengthUm:    length,
		WidthUm:     width,
		HeightUm:    height,
		IsRemnant:   isRemnant,
		Status:      stock.StatusAvailable,
		Location:    in.Location,
		CostPerUnit: cost,
		Notes:       in.Notes,
	})
	if err != nil {
		return stock.Item{}, err
	}
	return s.GetItem(ctx, row.ID.String())
}

// UpdateItem patches label, location, status and notes of a piece.
func (s *Store) UpdateItem(ctx context.Context, id string, in stock.UpdateInput) (stock.Item, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return stock.Item{}, stock.ErrNotFound
	}
	if _, err := s.queries.GetStockItem(ctx, parsed); errors.Is(err, pgx.ErrNoRows) {
		return stock.Item{}, stock.ErrNotFound
	} else if err != nil {
		return stock.Item{}, err
	}
	if _, err := s.queries.UpdateStockItem(ctx, db.UpdateStockItemParams{
		ID:       parsed,
		Label:    in.Label,
		Location: in.Location,
		Status:   in.Status,
		Notes:    in.Notes,
	}); err != nil {
		return stock.Item{}, err
	}
	return s.GetItem(ctx, id)
}

// ListRemnants returns the available labelled remnants of the default plant as
// solver-ready stock items: quantity one each, flagged as remnants.
func (s *Store) ListRemnants(ctx context.Context, materialSpecID string) ([]core.StockItem, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return nil, err
	}
	specID := pgtype.UUID{}
	if materialSpecID != "" {
		parsed, err := uuid.Parse(materialSpecID)
		if err != nil {
			return nil, fmt.Errorf("materialSpecId is not a valid uuid: %w", err)
		}
		specID = pgtype.UUID{Bytes: parsed, Valid: true}
	}
	rows, err := s.queries.ListAvailableRemnants(ctx, db.ListAvailableRemnantsParams{
		PlantID:        plant.ID,
		MaterialSpecID: specID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]core.StockItem, 0, len(rows))
	for _, r := range rows {
		item := core.StockItem{
			ID:          r.ID.String(),
			Code:        r.Code,
			Label:       r.Label,
			Length:      r.LengthUm,
			Width:       r.WidthUm,
			Height:      r.HeightUm,
			Quantity:    1,
			CostPerUnit: r.CostPerUnit,
			IsRemnant:   true,
		}
		if r.FormatID.Valid {
			item.FormatID = uuid.UUID(r.FormatID.Bytes).String()
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) defaultPlant(ctx context.Context) (db.Plant, error) {
	plant, err := s.queries.GetDefaultPlant(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Plant{}, errors.New("no plant configured; run db/scripts/seed.ps1")
		}
		return db.Plant{}, err
	}
	return plant, nil
}

// toStockItem converts any of the sqlc stock-item rows (they share this shape)
// into the module DTO.
func toStockItem(
	id uuid.UUID,
	formatID pgtype.UUID,
	code, label string,
	length, width, height int64,
	isRemnant bool,
	status, location string,
	costPerUnit float64,
	notes string,
	parentPlanID pgtype.UUID,
	parentSheetIndex *int32,
	consumedByPlanID pgtype.UUID,
	consumedAt, createdAt pgtype.Timestamptz,
	formatCode, specCode, materialCode, dimensionProfile *string,
) stock.Item {
	item := stock.Item{
		ID:           id.String(),
		Code:         code,
		Label:        label,
		LengthMicron: length,
		WidthMicron:  width,
		HeightMicron: height,
		IsRemnant:    isRemnant,
		Status:       status,
		Location:     location,
		CostPerUnit:  costPerUnit,
		Notes:        notes,
		CreatedAt:    createdAt.Time,
	}
	if formatID.Valid {
		item.FormatID = uuid.UUID(formatID.Bytes).String()
	}
	if parentPlanID.Valid {
		item.ParentPlanID = uuid.UUID(parentPlanID.Bytes).String()
	}
	item.ParentSheetIndex = parentSheetIndex
	if consumedByPlanID.Valid {
		item.ConsumedByPlanID = uuid.UUID(consumedByPlanID.Bytes).String()
	}
	if consumedAt.Valid {
		consumed := consumedAt.Time
		item.ConsumedAt = &consumed
	}
	if formatCode != nil {
		item.FormatCode = *formatCode
	}
	if specCode != nil {
		item.SpecCode = *specCode
	}
	if materialCode != nil {
		item.MaterialCode = *materialCode
	}
	if dimensionProfile != nil {
		item.DimensionProfile = *dimensionProfile
	}
	return item
}
