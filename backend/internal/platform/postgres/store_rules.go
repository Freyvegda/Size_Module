package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/size-module/backend/internal/modules/rules"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

// --------------------------------------------------------- rules profiles ---

func (s *Store) ListProfiles(ctx context.Context) ([]rules.Profile, error) {
	rows, err := s.queries.ListRulesProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]rules.Profile, 0, len(rows))
	for _, r := range rows {
		out = append(out, profileFromJSON(r.ID.String(), r.Code, r.Name, r.Rules, r.Objective, r.IsDefault))
	}
	return out, nil
}

func (s *Store) GetProfile(ctx context.Context, id string) (rules.Profile, error) {
	profileID, err := uuid.Parse(id)
	if err != nil {
		return rules.Profile{}, rules.ErrNotFound
	}
	row, err := s.queries.GetRulesProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rules.Profile{}, rules.ErrNotFound
		}
		return rules.Profile{}, err
	}
	return profileFromJSON(row.ID.String(), row.Code, row.Name, row.Rules, row.Objective, row.IsDefault), nil
}

func (s *Store) CreateProfile(ctx context.Context, in rules.SaveProfileInput) (rules.Profile, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return rules.Profile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return rules.Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	row, err := q.CreateRulesProfile(ctx, db.CreateRulesProfileParams{
		PlantID:   plant.ID,
		Code:      in.Code,
		Name:      in.Name,
		Rules:     marshalJSON(in.Rules),
		Objective: marshalJSON(in.Objective),
		IsDefault: in.IsDefault,
	})
	if err != nil {
		return rules.Profile{}, err
	}
	if in.IsDefault {
		if err := q.ClearDefaultRulesProfiles(ctx, db.ClearDefaultRulesProfilesParams{PlantID: plant.ID, ID: row.ID}); err != nil {
			return rules.Profile{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return rules.Profile{}, err
	}
	return profileFromJSON(row.ID.String(), row.Code, row.Name, row.Rules, row.Objective, row.IsDefault), nil
}

func (s *Store) UpdateProfile(ctx context.Context, id string, in rules.SaveProfileInput) (rules.Profile, error) {
	profileID, err := uuid.Parse(id)
	if err != nil {
		return rules.Profile{}, rules.ErrNotFound
	}
	current, err := s.queries.GetRulesProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rules.Profile{}, rules.ErrNotFound
		}
		return rules.Profile{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return rules.Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	row, err := q.UpdateRulesProfile(ctx, db.UpdateRulesProfileParams{
		ID:        profileID,
		Name:      in.Name,
		Rules:     marshalJSON(in.Rules),
		Objective: marshalJSON(in.Objective),
		IsDefault: in.IsDefault,
	})
	if err != nil {
		return rules.Profile{}, err
	}
	if in.IsDefault {
		if err := q.ClearDefaultRulesProfiles(ctx, db.ClearDefaultRulesProfilesParams{PlantID: current.PlantID, ID: profileID}); err != nil {
			return rules.Profile{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return rules.Profile{}, err
	}
	// Code is immutable, so keep the stored one rather than trusting the body.
	return profileFromJSON(row.ID.String(), row.Code, row.Name, row.Rules, row.Objective, row.IsDefault), nil
}

// RulesProfile resolves a stored profile into engine rules and objective for the
// jobs module (jobs.RulesSource). The rules are then snapshotted into the job's
// problem, so a later profile edit cannot rewrite history.
func (s *Store) RulesProfile(ctx context.Context, id string) (core.Rules, core.Objective, error) {
	profile, err := s.GetProfile(ctx, id)
	if err != nil {
		return core.Rules{}, core.Objective{}, err
	}
	return profile.Rules, profile.Objective, nil
}

func profileFromJSON(id, code, name string, rulesJSON, objectiveJSON []byte, isDefault bool) rules.Profile {
	profile := rules.Profile{ID: id, Code: code, Name: name, IsDefault: isDefault}
	if len(rulesJSON) > 0 {
		if err := json.Unmarshal(rulesJSON, &profile.Rules); err != nil {
			// A malformed stored profile must not take the whole list down;
			// callers fall back to core defaults at solve time.
			profile.Rules = core.Rules{}
		}
	}
	if len(objectiveJSON) > 0 {
		if err := json.Unmarshal(objectiveJSON, &profile.Objective); err != nil {
			profile.Objective = core.Objective{}
		}
	}
	return profile
}
