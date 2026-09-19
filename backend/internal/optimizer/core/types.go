// Package core defines the language of the optimization engine: the problems it
// accepts and the solutions it produces.
//
// It deliberately has no dependencies outside the standard library so it can be
// reused by CLI tools, tests and (in the future) compiled to WebAssembly for
// instant in-browser previews.
//
// Unit rule: every length in the system is a Dim, i.e. micrometers stored as
// int64. Integers keep the geometry arithmetic exact and the API/DB layers
// convert to millimeters at the edges.
package core

// Dim is a length in micrometers.
type Dim = int64

const (
	Micrometer Dim = 1
	Millimeter Dim = 1_000
	Meter      Dim = 1_000_000
)

// FromMM converts millimeters to micrometers.
func FromMM(mm float64) Dim { return Dim(mm * float64(Millimeter)) }

// ToMM converts micrometers to millimeters.
func ToMM(d Dim) float64 { return float64(d) / float64(Millimeter) }

// AreaM2 returns the area of a w x h rectangle in square meters.
func AreaM2(w, h Dim) float64 { return float64(w) * float64(h) / 1e12 }

// LengthM returns a length in meters.
func LengthM(d Dim) float64 { return float64(d) / 1e6 }

// DimensionProfile tells the engine which geometry family a material belongs
// to: bars/profiles (1d), sheets/panels (2d) or boxes (3d).
type DimensionProfile string

const (
	Profile1D DimensionProfile = "1d"
	Profile2D DimensionProfile = "2d"
	Profile3D DimensionProfile = "3d"
)

// GrainMode expresses a directional requirement of a part or a material.
type GrainMode string

const (
	GrainNone   GrainMode = "none"
	GrainAlongX GrainMode = "along_x"
	GrainAlongY GrainMode = "along_y"
)

// CutMode describes how the material may be divided.
type CutMode string

const (
	// CutGuillotine means every cut must run edge to edge across the piece
	// being cut (panel saws, glass cutters, shears).
	CutGuillotine CutMode = "guillotine"
	// CutFree means arbitrary cutting paths are allowed (CNC routers, lasers,
	// waterjets).
	CutFree CutMode = "free"
)

// Rect is an axis-aligned rectangle in micrometers.
type Rect struct {
	X Dim `json:"x"`
	Y Dim `json:"y"`
	W Dim `json:"w"`
	H Dim `json:"h"`
}

func (r Rect) Right() Dim  { return r.X + r.W }
func (r Rect) Bottom() Dim { return r.Y + r.H }
func (r Rect) Area() Dim   { return r.W * r.H }

// Inset shrinks the rectangle by m on every side. It may return a degenerate
// rectangle (zero or negative size) when m is too large.
func (r Rect) Inset(m Dim) Rect {
	return Rect{
		X: r.X + m,
		Y: r.Y + m,
		W: r.W - 2*m,
		H: r.H - 2*m,
	}
}

// Contains reports whether o lies fully inside r.
func (r Rect) Contains(o Rect) bool {
	return o.X >= r.X && o.Y >= r.Y && o.Right() <= r.Right() && o.Bottom() <= r.Bottom()
}

// Part is a single thing that must be produced by cutting stock material.
type Part struct {
	ID             string `json:"id"`
	Code           string `json:"code"`
	MaterialSpecID string `json:"materialSpecId,omitempty"`
	// Cut size. Allowances (grinding, edge deletion, ...) must already be
	// applied by the parts module before the part reaches the solver.
	Length   Dim       `json:"length,omitempty"` // 1d parts
	Width    Dim       `json:"width,omitempty"`  // 2d/3d parts
	Height   Dim       `json:"height,omitempty"` // 2d/3d parts
	Quantity int       `json:"quantity"`
	Grain    GrainMode `json:"grain,omitempty"`
	// AllowRotate lets the solver rotate this part 90° on a sheet.
	AllowRotate bool `json:"allowRotate"`
	// Priority: 1 is the most important.
	Priority int    `json:"priority,omitempty"`
	Group    string `json:"group,omitempty"` // e.g. an order number, used for reporting
}

// StockItem is an available piece of material: either a catalog format with an
// on-hand quantity, or a physical remnant.
type StockItem struct {
	ID          string  `json:"id"`
	FormatID    string  `json:"formatId,omitempty"`
	Code        string  `json:"code"`
	Label       string  `json:"label,omitempty"`
	// MaterialSpecID is the material this piece belongs to. It is descriptive
	// for most callers but campaigns use it to keep an item's parts out of
	// another material's stock.
	MaterialSpecID string `json:"materialSpecId,omitempty"`
	Length      Dim     `json:"length,omitempty"`
	Width       Dim     `json:"width,omitempty"`
	Height      Dim     `json:"height,omitempty"`
	Quantity    int     `json:"quantity"`
	CostPerUnit float64 `json:"costPerUnit,omitempty"`
	IsRemnant   bool    `json:"isRemnant,omitempty"`
	// Defects are regions of this physical piece that cannot be used.
	Defects []Rect `json:"defects,omitempty"`
}

// Rules is one complete set of manufacturing constraints. Rules profiles are
// stored in the database and are the only place vertical specifics live: there
// is no glass or wood code path in the engine, only different rule values.
type Rules struct {
	// Kerf consumed by the tool on every cut.
	Kerf Dim `json:"kerf"`
	// Trim is the minimum distance between a part and the stock edge.
	Trim Dim `json:"trim"`
	// AllowRotate permits 90°/180° rotation globally.
	AllowRotate bool `json:"allowRotate"`
	// GrainMode of the stock material.
	GrainMode GrainMode `json:"grainMode"`
	// CutMode is the machine capability.
	CutMode CutMode `json:"cutMode"`
	// MaxCutStages limits multi-stage guillotine cuts (0 = unlimited).
	MaxCutStages int `json:"maxCutStages"`
	// OffcutMinW/OffcutMinH: leftovers at least this big are reusable offcuts
	// instead of scrap. This is what separates "waste" from "future stock".
	OffcutMinW Dim `json:"offcutMinW"`
	OffcutMinH Dim `json:"offcutMinH"`
	// OffcutMinLength is the 1D counterpart: leftovers at least this long are
	// reusable offcuts.
	OffcutMinLength Dim `json:"offcutMinLength"`
	// MinPartDim rejects parts that are too small for the machine.
	MinPartDim Dim `json:"minPartDim"`
	// MaxPartsPerSheet limits how many pieces may be cut from one sheet.
	MaxPartsPerSheet int `json:"maxPartsPerSheet"`
	// PreferRemnants offers physical remnants to the solver before fresh
	// catalog stock (leftovers are already paid for). See PrioritizeRemnants.
	PreferRemnants bool `json:"preferRemnants"`
	// OversAllowedPct permits cutting extra pieces (e.g. 0.05 = up to 5%).
	OversAllowedPct float64 `json:"oversAllowedPct"`
}

// Weights is the scalar objective. All terms are minimised except FillPriority,
// which is maximised.
type Weights struct {
	FillPriority  float64 `json:"fillPriority"`
	MinSheets     float64 `json:"minSheets"`
	MinScrap      float64 `json:"minScrap"`
	MinPatterns   float64 `json:"minPatterns"`
	MinOffcutArea float64 `json:"minOffcutArea"`
	Cost          float64 `json:"cost"`
}

// Objective combines optional ordered goals with numeric weights. The weight
// vector is what the engine actually optimises; Priorities documents intent for
// the UI and for future lexicographic solving.
type Objective struct {
	Weights    Weights  `json:"weights"`
	Priorities []string `json:"priorities,omitempty"`
}

// DefaultWeights is the out-of-the-box objective: fill demand first, then use
// as few sheets as possible, then limit unusable scrap and pattern spread.
func DefaultWeights() Weights {
	return Weights{
		FillPriority:  1000,
		MinSheets:     1,
		MinScrap:      1,
		MinPatterns:   1,
		MinOffcutArea: 0.25,
		Cost:          1,
	}
}

// DefaultRules returns generic starting rules: 4 mm kerf, 10 mm edge trim and
// leftovers from 300x300 mm (or 300 mm for bars) treated as reusable offcuts.
// Vertical specifics (glass, wood, metal) belong in stored rules profiles.
func DefaultRules() Rules {
	return Rules{
		Kerf:            4 * Millimeter,
		Trim:            10 * Millimeter,
		AllowRotate:     true,
		GrainMode:       GrainNone,
		CutMode:         CutGuillotine,
		OffcutMinW:      300 * Millimeter,
		OffcutMinH:      300 * Millimeter,
		OffcutMinLength: 300 * Millimeter,
		PreferRemnants:  true,
	}
}

// Normalize fills in defaults for a problem that was submitted without rules
// or objective, so every solver receives a complete description.
func Normalize(p Problem) Problem {
	if p.Rules.Kerf == 0 && p.Rules.Trim == 0 && p.Rules.CutMode == "" {
		p.Rules = DefaultRules()
	}
	if p.Rules.CutMode == "" {
		p.Rules.CutMode = CutGuillotine
	}
	if p.Rules.GrainMode == "" {
		p.Rules.GrainMode = GrainNone
	}
	if p.Objective.Weights == (Weights{}) {
		p.Objective.Weights = DefaultWeights()
	}
	if p.BudgetMS <= 0 {
		p.BudgetMS = 5000
	}
	for i := range p.Parts {
		if p.Parts[i].Quantity <= 0 {
			p.Parts[i].Quantity = 1
		}
	}
	for i := range p.Stocks {
		if p.Stocks[i].Quantity <= 0 {
			p.Stocks[i].Quantity = 1
		}
	}
	// Remnant-first allocation: offer leftovers before fresh stock.
	PrioritizeRemnants(&p)
	return p
}

// Problem is a complete, self-contained description of an optimization task.
// Jobs snapshot a Problem so results stay reproducible even after master data
// changes.
type Problem struct {
	Parts     []Part      `json:"parts"`
	Stocks    []StockItem `json:"stocks"`
	Rules     Rules       `json:"rules"`
	Objective Objective   `json:"objective"`
	// Pinned holds planner-locked sheet layouts a re-solve must preserve. A
	// solver without Capabilities.Pinned must not serve a problem with pins.
	Pinned []PinnedSheet `json:"pinned,omitempty"`
	// BudgetMS is the soft time budget. Solvers must return their best
	// solution when the budget expires instead of failing.
	BudgetMS int    `json:"budgetMs,omitempty"`
	Seed     uint64 `json:"seed,omitempty"`
}

// Placement is one part placed on one sheet.
type Placement struct {
	// ID identifies a stored placement when a plan is read back for editing;
	// solvers leave it empty.
	ID       string `json:"id,omitempty"`
	PartID   string `json:"partId"`
	PartCode string `json:"partCode"`
	X        Dim    `json:"x"`
	Y        Dim    `json:"y"`
	W        Dim    `json:"w"`
	H        Dim    `json:"h"`
	Rotated  bool   `json:"rotated"`
	Priority int    `json:"priority,omitempty"`
	// Locked marks a placement a planner pinned: a re-solve must keep it
	// exactly where it is.
	Locked bool `json:"locked,omitempty"`
}

// SheetPlan is the cutting layout of a single stock sheet.
type SheetPlan struct {
	Index      int         `json:"index"`
	StockID    string      `json:"stockId"`
	StockCode  string      `json:"stockCode"`
	Label      string      `json:"label,omitempty"`
	Width      Dim         `json:"width"`
	Height     Dim         `json:"height"`
	Placements []Placement `json:"placements"`
	// Offcuts are reusable remnants produced by this sheet.
	Offcuts []Rect `json:"offcuts"`
	// CutSteps is the human/machine readable cutting sequence.
	CutSteps []string `json:"cutSteps,omitempty"`
}

// UnplacedPart explains demand the solver could not satisfy.
type UnplacedPart struct {
	PartID   string `json:"partId"`
	PartCode string `json:"partCode"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
}

// PinnedSheet is a sheet whose pieces a planner locked: a re-solve must keep
// these placements exactly where they are and pack the remaining demand around
// them. The solver materialises one sheet per pinned entry, so a pinned sheet
// occupies one copy of its stock.
type PinnedSheet struct {
	StockID   string `json:"stockId,omitempty"`
	StockCode string `json:"stockCode,omitempty"`
	Label     string `json:"label,omitempty"`
	Width     Dim    `json:"width"`
	Height    Dim    `json:"height"`
	// Placements are the locked pieces. They must already honour the kerf,
	// bounds and guillotine rules of the problem; re-solving never moves them.
	Placements []Placement `json:"placements"`
}

// Metrics is the scorecard of a solution. Areas are in square meters, lengths
// in meters, so they are directly presentable in a UI.
type Metrics struct {
	SheetCount     int     `json:"sheetCount"`
	PartsPlaced    int     `json:"partsPlaced"`
	PartsRequested int     `json:"partsRequested"`
	StockAreaM2    float64 `json:"stockAreaM2"`
	PartAreaM2     float64 `json:"partAreaM2"`
	KerfAreaM2     float64 `json:"kerfAreaM2"`
	TrimAreaM2     float64 `json:"trimAreaM2"`
	ScrapAreaM2    float64 `json:"scrapAreaM2"`
	OffcutAreaM2   float64 `json:"offcutAreaM2"`
	YieldPct       float64 `json:"yieldPct"`
	WastePct       float64 `json:"wastePct"`
	PatternCount   int     `json:"patternCount"`
	Cost           float64 `json:"cost"`
	// RemnantSheets counts how many sheets were cut from a physical remnant
	// instead of fresh stock. The objective does not charge for them: they are
	// already paid-for capacity (see Score).
	RemnantSheets int     `json:"remnantSheets,omitempty"`
	StockLengthM  float64 `json:"stockLengthM,omitempty"` // 1d only
	UsedLengthM   float64 `json:"usedLengthM,omitempty"`  // 1d only
	ElapsedMS     int64   `json:"elapsedMs"`
}

// Solution is what a solver returns: layouts, leftovers, metrics and notes.
type Solution struct {
	Solver        string         `json:"solver"`
	SolverVersion string         `json:"solverVersion"`
	Seed          uint64         `json:"seed"`
	Sheets        []SheetPlan    `json:"sheets"`
	Unplaced      []UnplacedPart `json:"unplaced"`
	Metrics       Metrics        `json:"metrics"`
	Notes         []string       `json:"notes,omitempty"`
}
