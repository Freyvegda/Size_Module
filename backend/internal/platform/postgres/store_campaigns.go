package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/modules/campaigns"
	"github.com/size-module/backend/internal/optimizer"
	"github.com/size-module/backend/internal/optimizer/core"
	"github.com/size-module/backend/internal/platform/db"
)

// ListCampaigns returns campaigns, newest first.
func (s *Store) ListCampaigns(ctx context.Context, limit, offset int) ([]campaigns.Campaign, error) {
	rows, err := s.queries.ListCampaigns(ctx, db.ListCampaignsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]campaigns.Campaign, 0, len(rows))
	for _, row := range rows {
		out = append(out, toCampaign(row))
	}
	return out, nil
}

// GetCampaign returns a campaign with its items and progress.
func (s *Store) GetCampaign(ctx context.Context, id string) (campaigns.Detail, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	row, err := s.queries.GetCampaign(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	if err != nil {
		return campaigns.Detail{}, err
	}
	return s.campaignDetail(ctx, s.queries, row)
}

// CreateCampaign stores the shared planning context and its starting budget.
func (s *Store) CreateCampaign(ctx context.Context, in campaigns.CreateInput) (campaigns.Detail, error) {
	plant, err := s.defaultPlant(ctx)
	if err != nil {
		return campaigns.Detail{}, err
	}
	code := strings.TrimSpace(in.Code)
	if code == "" {
		code = "CMP-" + strings.ToUpper(uuid.NewString()[:8])
	}
	stock := in.Stock
	if stock == nil {
		stock = []core.StockItem{}
	}
	row, err := s.queries.CreateCampaign(ctx, db.CreateCampaignParams{
		PlantID:      plant.ID,
		Code:         code,
		Name:         in.Name,
		Rules:        marshalJSON(in.Rules),
		Objective:    marshalJSON(in.Objective),
		BudgetMs:     int32(in.BudgetMS),
		Seed:         int64(in.Seed),
		Stock:        marshalJSON(stock),
		InitialStock: marshalJSON(stock),
	})
	if err != nil {
		return campaigns.Detail{}, err
	}
	return s.campaignDetail(ctx, s.queries, row)
}

// UpdateCampaign renames a campaign or moves its status.
func (s *Store) UpdateCampaign(ctx context.Context, id string, in campaigns.UpdateInput) (campaigns.Detail, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	if _, err := s.queries.GetCampaign(ctx, parsed); errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Detail{}, campaigns.ErrNotFound
	} else if err != nil {
		return campaigns.Detail{}, err
	}
	row, err := s.queries.UpdateCampaignMeta(ctx, db.UpdateCampaignMetaParams{
		ID:     parsed,
		Name:   in.Name,
		Status: in.Status,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	if err != nil {
		return campaigns.Detail{}, err
	}
	return s.campaignDetail(ctx, s.queries, row)
}

// AddItem appends a job to the campaign, after every existing item.
func (s *Store) AddItem(ctx context.Context, campaignID string, in campaigns.CreateItemInput) (campaigns.Detail, error) {
	parsed, err := uuid.Parse(campaignID)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	row, err := s.queries.GetCampaign(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	if err != nil {
		return campaigns.Detail{}, err
	}
	if !campaigns.Editable(row.Status) {
		return campaigns.Detail{}, campaigns.ErrNotEditable
	}

	items, err := s.queries.ListCampaignItems(ctx, parsed)
	if err != nil {
		return campaigns.Detail{}, err
	}
	seq := int32(1)
	for _, item := range items {
		if item.Seq >= seq {
			seq = item.Seq + 1
		}
	}

	var due pgtype.Date
	if in.DueDate != "" {
		parsedDate, err := time.Parse("2006-01-02", in.DueDate)
		if err != nil {
			return campaigns.Detail{}, errors.New("dueDate must be YYYY-MM-DD")
		}
		due = pgtype.Date{Time: parsedDate, Valid: true}
	}
	priority := in.Priority
	if priority <= 0 {
		priority = 100
	}

	if _, err := s.queries.CreateCampaignItem(ctx, db.CreateCampaignItemParams{
		CampaignID: parsed,
		Seq:        seq,
		Name:       in.Name,
		Parts:      marshalJSON(in.Parts),
		DueDate:    due,
		Priority:   int32(priority),
	}); err != nil {
		return campaigns.Detail{}, err
	}
	return s.campaignDetail(ctx, s.queries, row)
}

// DeleteItem removes a pending item.
func (s *Store) DeleteItem(ctx context.Context, campaignID, itemID string) (campaigns.Detail, error) {
	campaign, err := uuid.Parse(campaignID)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	item, err := uuid.Parse(itemID)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	rows, err := s.queries.DeleteCampaignItem(ctx, db.DeleteCampaignItemParams{
		ID:         item,
		CampaignID: campaign,
	})
	if err != nil {
		return campaigns.Detail{}, err
	}
	if rows == 0 {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	return s.GetCampaign(ctx, campaignID)
}

// NextItem returns the earliest pending item.
func (s *Store) NextItem(ctx context.Context, campaignID string) (campaigns.Item, error) {
	parsed, err := uuid.Parse(campaignID)
	if err != nil {
		return campaigns.Item{}, campaigns.ErrNotFound
	}
	row, err := s.queries.NextCampaignItem(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Item{}, campaigns.ErrNoPendingItem
	}
	if err != nil {
		return campaigns.Item{}, err
	}
	return toCampaignItem(row), nil
}

// CompleteItem archives a solved item as a job plus plan and stores the
// remaining stock budget, all in one transaction under a campaign row lock, so
// two concurrent run-next calls cannot plan the same item twice.
func (s *Store) CompleteItem(ctx context.Context, campaignID, itemID string, problem core.Problem, result optimizer.Result, stock []core.StockItem) (campaigns.Detail, error) {
	campaign, err := uuid.Parse(campaignID)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	item, err := uuid.Parse(itemID)
	if err != nil {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return campaigns.Detail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	row, err := q.GetCampaignForUpdate(ctx, campaign)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	if err != nil {
		return campaigns.Detail{}, err
	}
	if !campaigns.Editable(row.Status) {
		return campaigns.Detail{}, campaigns.ErrNotEditable
	}
	pendingItem, err := q.GetCampaignItem(ctx, item)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaigns.Detail{}, campaigns.ErrNotFound
	}
	if err != nil {
		return campaigns.Detail{}, err
	}
	if pendingItem.CampaignID != campaign || pendingItem.Status != "pending" {
		return campaigns.Detail{}, campaigns.ErrNotEditable
	}

	budget := problem.BudgetMS
	if budget <= 0 {
		budget = 5000
	}
	job, err := q.CreateJob(ctx, db.CreateJobParams{
		PlantID:   row.PlantID,
		Solver:    result.Solution.Solver,
		Input:     marshalJSON(problem),
		Seed:      int64(problem.Seed),
		BudgetMs:  int32(budget),
		CreatedBy: pgtype.UUID{},
	})
	if err != nil {
		return campaigns.Detail{}, err
	}
	if err := q.MarkJobRunning(ctx, job.ID); err != nil {
		return campaigns.Detail{}, err
	}
	planID, err := writePlan(ctx, q, planParams{
		PlantID: row.PlantID,
		JobID:   pgtypeUUID(job.ID),
		Version: 1,
		Status:  "draft",
		Name:    pendingItem.Name,
	}, problem, result)
	if err != nil {
		return campaigns.Detail{}, err
	}
	if err := q.MarkJobDone(ctx, db.MarkJobDoneParams{ID: job.ID, Result: marshalJSON(result)}); err != nil {
		return campaigns.Detail{}, err
	}
	if _, err := q.CompleteCampaignItem(ctx, db.CompleteCampaignItemParams{
		ID:     item,
		Status: "planned",
		PlanID: pgtypeUUID(planID),
		JobID:  pgtypeUUID(job.ID),
	}); err != nil {
		return campaigns.Detail{}, err
	}

	pending, err := q.CountPendingCampaignItems(ctx, campaign)
	if err != nil {
		return campaigns.Detail{}, err
	}
	status := campaigns.StatusActive
	if pending == 0 {
		status = campaigns.StatusCompleted
	}
	if stock == nil {
		stock = []core.StockItem{}
	}
	if _, err := q.UpdateCampaignStock(ctx, db.UpdateCampaignStockParams{
		ID:     campaign,
		Stock:  marshalJSON(stock),
		Status: status,
	}); err != nil {
		return campaigns.Detail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return campaigns.Detail{}, err
	}
	return s.GetCampaign(ctx, campaignID)
}

// campaignDetail loads a campaign's items and derives progress.
func (s *Store) campaignDetail(ctx context.Context, q *db.Queries, row db.Campaign) (campaigns.Detail, error) {
	items, err := q.ListCampaignItems(ctx, row.ID)
	if err != nil {
		return campaigns.Detail{}, err
	}
	detail := campaigns.Detail{Campaign: toCampaign(row)}
	for _, item := range items {
		detail.Items = append(detail.Items, toCampaignItem(item))
		if item.Status == "pending" {
			detail.Progress.Pending++
		} else {
			detail.Progress.Planned++
		}
	}
	detail.Progress.Items = len(detail.Items)
	return detail, nil
}

func toCampaign(row db.Campaign) campaigns.Campaign {
	out := campaigns.Campaign{
		ID:        row.ID.String(),
		Code:      row.Code,
		Name:      row.Name,
		Status:    row.Status,
		BudgetMS:  int(row.BudgetMs),
		Seed:      uint64(row.Seed),
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
	if len(row.Rules) > 0 {
		_ = json.Unmarshal(row.Rules, &out.Rules)
	}
	if len(row.Objective) > 0 {
		_ = json.Unmarshal(row.Objective, &out.Objective)
	}
	if len(row.Stock) > 0 {
		_ = json.Unmarshal(row.Stock, &out.Stock)
	}
	if len(row.InitialStock) > 0 {
		_ = json.Unmarshal(row.InitialStock, &out.InitialStock)
	}
	if row.CompletedAt.Valid {
		completed := row.CompletedAt.Time
		out.CompletedAt = &completed
	}
	return out
}

func toCampaignItem(row db.CampaignItem) campaigns.Item {
	out := campaigns.Item{
		ID:       row.ID.String(),
		Seq:      int(row.Seq),
		Name:     row.Name,
		Priority: int(row.Priority),
		Status:   row.Status,
		Error:    row.Error,
	}
	if len(row.Parts) > 0 {
		_ = json.Unmarshal(row.Parts, &out.Parts)
	}
	if row.DueDate.Valid {
		due := row.DueDate.Time
		out.DueDate = &due
	}
	if row.PlanID.Valid {
		out.PlanID = uuid.UUID(row.PlanID.Bytes).String()
	}
	if row.JobID.Valid {
		out.JobID = uuid.UUID(row.JobID.Bytes).String()
	}
	return out
}
