package postgres

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/size-module/backend/internal/modules/kpis"
	"github.com/size-module/backend/internal/platform/db"
)

// kpiSeriesLimit caps the trend line returned to the dashboard.
const kpiSeriesLimit = 120

// KPIs aggregates plan metrics over a window: realized numbers from accepted
// plans and pipeline numbers from every plan created in the window.
func (s *Store) KPIs(ctx context.Context, from, to time.Time) (kpis.Report, error) {
	window := db.PlanKPIsParams{
		From: pgtype.Timestamptz{Time: from, Valid: true},
		To:   pgtype.Timestamptz{Time: to, Valid: true},
	}
	row, err := s.queries.PlanKPIs(ctx, window)
	if err != nil {
		return kpis.Report{}, err
	}

	report := kpis.Report{
		From: from,
		To:   to,
		Realized: kpiBucket(
			row.AcceptedPlans, row.AcceptedSheets, row.AcceptedParts, row.AcceptedRemnantSheets,
			row.AcceptedStockAreaM2, row.AcceptedPartAreaM2, row.AcceptedScrapAreaM2,
			row.AcceptedOffcutAreaM2, row.AcceptedCost,
		),
		Created: kpiBucket(
			row.TotalPlans, row.TotalSheets, row.TotalPartsPlaced, row.TotalRemnantSheets,
			row.TotalStockAreaM2, row.TotalPartAreaM2, row.TotalScrapAreaM2,
			row.TotalOffcutAreaM2, row.TotalCost,
		),
	}

	series, err := s.queries.PlanYieldSeries(ctx, db.PlanYieldSeriesParams{
		From:  window.From,
		To:    window.To,
		Limit: kpiSeriesLimit,
	})
	if err != nil {
		return kpis.Report{}, err
	}
	report.Series = make([]kpis.SeriesPoint, 0, len(series))
	for _, point := range series {
		report.Series = append(report.Series, kpis.SeriesPoint{
			PlanID:      point.ID.String(),
			CreatedAt:   point.CreatedAt.Time,
			YieldPct:    point.YieldPct,
			WastePct:    point.WastePct,
			Cost:        point.Cost,
			Sheets:      int(point.Sheets),
			PartsPlaced: int(point.PartsPlaced),
		})
	}
	return report, nil
}

func kpiBucket(plans int64, sheets, parts, remnantSheets int64, stock, part, scrap, offcut, cost float64) kpis.Bucket {
	bucket := kpis.Bucket{
		Plans:         int(plans),
		Sheets:        int(sheets),
		PartsPlaced:   int(parts),
		RemnantSheets: int(remnantSheets),
		StockAreaM2:   round2(stock),
		PartAreaM2:    round2(part),
		ScrapAreaM2:   round2(scrap),
		OffcutAreaM2:  round2(offcut),
		Cost:          round2(cost),
	}
	if stock > 0 {
		bucket.YieldPct = round2(100 * part / stock)
		bucket.WastePct = round2(100 * (1 - part/stock))
	}
	if parts > 0 {
		bucket.CostPerPart = round2(cost / float64(parts))
	}
	return bucket
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
