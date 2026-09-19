package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/modules/assemblies"
	"github.com/size-module/backend/internal/platform/db"
)

// ListAssemblies returns every product with its components attached.
func (s *Store) ListAssemblies(ctx context.Context) ([]assemblies.Assembly, error) {
	rows, err := s.queries.ListAssemblies(ctx)
	if err != nil {
		return nil, err
	}
	comps, err := s.queries.ListAllAssemblyComponents(ctx)
	if err != nil {
		return nil, err
	}
	byAssembly := make(map[uuid.UUID][]assemblies.Component, len(rows))
	for _, c := range comps {
		byAssembly[c.AssemblyID] = append(byAssembly[c.AssemblyID], toComponentDTO(c))
	}
	out := make([]assemblies.Assembly, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAssemblyDTO(a, byAssembly[a.ID]))
	}
	return out, nil
}

// GetAssembly returns one product and its ordered components.
func (s *Store) GetAssembly(ctx context.Context, id string) (assemblies.Assembly, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return assemblies.Assembly{}, assemblies.ErrNotFound
	}
	row, err := s.queries.GetAssembly(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return assemblies.Assembly{}, assemblies.ErrNotFound
	}
	if err != nil {
		return assemblies.Assembly{}, err
	}
	comps, err := s.queries.ListAssemblyComponents(ctx, parsed)
	if err != nil {
		return assemblies.Assembly{}, err
	}
	dtos := make([]assemblies.Component, 0, len(comps))
	for _, c := range comps {
		dtos = append(dtos, toComponentDTO(c))
	}
	return toAssemblyDTO(row, dtos), nil
}

// CreateAssembly writes the product and its subparts in one transaction.
func (s *Store) CreateAssembly(ctx context.Context, in assemblies.CreateAssemblyInput) (assemblies.Assembly, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return assemblies.Assembly{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return assemblies.Assembly{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	row, err := q.CreateAssembly(ctx, db.CreateAssemblyParams{
		PlantID:        plant.ID,
		MaterialSpecID: specUUID(in.MaterialSpecID),
		Code:           in.Code,
		Name:           in.Name,
		Kind:           in.Kind,
		WidthUm:        in.WidthMicron,
		HeightUm:       in.HeightMicron,
		DepthUm:        in.DepthMicron,
		Attributes:     marshalJSON(nil),
	})
	if err != nil {
		return assemblies.Assembly{}, err
	}

	components := make([]assemblies.Component, 0, len(in.Components))
	for _, c := range in.Components {
		if c.MaterialSpecID != "" {
			if _, err := uuid.Parse(c.MaterialSpecID); err != nil {
				return assemblies.Assembly{}, fmt.Errorf("component materialSpecId is not a valid uuid: %w", err)
			}
		}
		created, err := q.CreateAssemblyComponent(ctx, db.CreateAssemblyComponentParams{
			AssemblyID:     row.ID,
			Seq:            c.Seq,
			Role:           c.Role,
			Kind:           c.Kind,
			Name:           c.Name,
			MaterialSpecID: specUUID(c.MaterialSpecID),
			Quantity:       c.Quantity,
			WidthUm:        c.WidthMicron,
			HeightUm:       c.HeightMicron,
			DepthUm:        c.DepthMicron,
			OffsetXUm:      c.OffsetXMicron,
			OffsetYUm:      c.OffsetYMicron,
			OffsetZUm:      c.OffsetZMicron,
			Attributes:     marshalJSON(nil),
		})
		if err != nil {
			return assemblies.Assembly{}, err
		}
		components = append(components, toComponentDTO(created))
	}

	if err := tx.Commit(ctx); err != nil {
		return assemblies.Assembly{}, err
	}
	return toAssemblyDTO(row, components), nil
}

func toAssemblyDTO(a db.Assembly, components []assemblies.Component) assemblies.Assembly {
	if components == nil {
		components = []assemblies.Component{}
	}
	out := assemblies.Assembly{
		ID:           a.ID.String(),
		Code:         a.Code,
		Name:         a.Name,
		Kind:         a.Kind,
		WidthMicron:  a.WidthUm,
		HeightMicron: a.HeightUm,
		DepthMicron:  a.DepthUm,
		Components:   components,
	}
	if a.MaterialSpecID.Valid {
		out.MaterialSpecID = uuid.UUID(a.MaterialSpecID.Bytes).String()
	}
	return out
}

func toComponentDTO(c db.AssemblyComponent) assemblies.Component {
	out := assemblies.Component{
		ID:            c.ID.String(),
		Seq:           c.Seq,
		Role:          c.Role,
		Kind:          c.Kind,
		Name:          c.Name,
		Quantity:      c.Quantity,
		WidthMicron:   c.WidthUm,
		HeightMicron:  c.HeightUm,
		DepthMicron:   c.DepthUm,
		OffsetXMicron: c.OffsetXUm,
		OffsetYMicron: c.OffsetYUm,
		OffsetZMicron: c.OffsetZUm,
	}
	if c.MaterialSpecID.Valid {
		out.MaterialSpecID = uuid.UUID(c.MaterialSpecID.Bytes).String()
	}
	return out
}

// specUUID parses an optional material spec id into the pgtype form sqlc uses.
func specUUID(id string) pgtype.UUID {
	if id == "" {
		return pgtype.UUID{}
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}
