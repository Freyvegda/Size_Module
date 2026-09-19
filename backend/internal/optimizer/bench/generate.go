package bench

import (
	"fmt"
	"math/rand"

	"github.com/size-module/backend/internal/optimizer/core"
)

// DefaultSeed is the seed used for generated benchmark instances unless the
// caller overrides it. It is part of the golden gate: the committed golden file
// was produced with this seed, so tests and CI reproduce exactly the same
// instances.
const DefaultSeed uint64 = 7

// Generate builds a deterministic random instance. The same seed and index
// always produce exactly the same problem, which is what makes the golden file
// a meaningful regression gate.
//
// The distribution is loosely modelled on shop reality: a few large pieces
// close to the stock width, a long tail of medium pieces, some small fillers,
// and one to two stock formats.
func Generate(seed uint64, index int) Instance {
	rng := rand.New(rand.NewSource(int64(seed*1_000_003) + int64(index)*7919 + 17))

	formats := []core.StockItem{
		{ID: "fmt-a", Code: "SHEET-3210x2250", Width: core.FromMM(3210), Height: core.FromMM(2250), CostPerUnit: 62.5},
		{ID: "fmt-b", Code: "SHEET-2440x1220", Width: core.FromMM(2440), Height: core.FromMM(1220), CostPerUnit: 28},
		{ID: "fmt-c", Code: "SHEET-2500x1250", Width: core.FromMM(2500), Height: core.FromMM(1250), CostPerUnit: 33},
	}

	stockCount := 1
	if rng.Intn(3) == 0 {
		stockCount = 2
	}
	stocks := make([]core.StockItem, 0, stockCount)
	used := map[int]bool{}
	for i := 0; i < stockCount; i++ {
		idx := rng.Intn(len(formats))
		if used[idx] {
			idx = (idx + 1) % len(formats)
		}
		used[idx] = true
		stock := formats[idx]
		stock.Quantity = 3 + rng.Intn(6)
		stocks = append(stocks, stock)
	}

	nParts := 10 + rng.Intn(13)
	parts := make([]core.Part, 0, nParts)
	for i := 0; i < nParts; i++ {
		var widthMM, heightMM float64
		switch {
		case i%7 == 3: // a long pane close to the stock width
			widthMM = 1700 + float64(rng.Intn(40))*10
			heightMM = 600 + float64(rng.Intn(60))*10
		case rng.Intn(3) == 0: // small fillers
			widthMM = 300 + float64(rng.Intn(50))*10
			heightMM = 250 + float64(rng.Intn(40))*10
		default:
			widthMM = 400 + float64(rng.Intn(90))*10
			heightMM = 400 + float64(rng.Intn(80))*10
		}
		quantity := 1 + rng.Intn(4)
		priority := 2
		if i < nParts/3 {
			priority = 1
		}
		parts = append(parts, core.Part{
			ID:          fmt.Sprintf("p%02d", i),
			Code:        fmt.Sprintf("PART-%04.0fx%04.0f", widthMM, heightMM),
			Width:       core.FromMM(widthMM),
			Height:      core.FromMM(heightMM),
			Quantity:    quantity,
			AllowRotate: true,
			Priority:    priority,
		})
	}

	return Instance{
		Name:   fmt.Sprintf("random-%02d", index),
		Parts:  parts,
		Stocks: stocks,
		Rules:  core.DefaultRules(),
	}
}
