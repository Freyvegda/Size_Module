// Types mirroring the Go JSON contracts (backend/internal/optimizer/core and
// the module DTOs). All lengths are micrometers unless the name says otherwise.

export type DimensionProfile = '1d' | '2d' | '3d'
export type CutMode = 'guillotine' | 'free'
export type GrainMode = 'none' | 'along_x' | 'along_y'

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface Placement {
  partId: string
  partCode: string
  x: number
  y: number
  w: number
  h: number
  rotated: boolean
  priority?: number
}

export interface SheetPlan {
  index: number
  stockId: string
  stockCode: string
  label?: string
  width: number
  height: number
  placements: Placement[]
  /** Go marshals nil slices as null. */
  offcuts?: Rect[] | null
  cutSteps?: string[]
}

export interface UnplacedPart {
  partId: string
  partCode: string
  quantity: number
  reason: string
}

export interface Metrics {
  sheetCount: number
  partsPlaced: number
  partsRequested: number
  stockAreaM2: number
  partAreaM2: number
  kerfAreaM2: number
  trimAreaM2: number
  scrapAreaM2: number
  offcutAreaM2: number
  yieldPct: number
  wastePct: number
  patternCount: number
  cost: number
  /** Sheets cut from physical remnants; not charged as fresh stock. */
  remnantSheets?: number
  /** 1D plans only. */
  stockLengthM?: number
  /** 1D plans only. */
  usedLengthM?: number
  elapsedMs: number
}

export interface Solution {
  solver: string
  solverVersion: string
  seed: number
  /** Go marshals nil slices as null. */
  sheets: SheetPlan[] | null
  unplaced: UnplacedPart[] | null
  metrics: Metrics
  notes?: string[]
}

export interface Violation {
  severity: 'error' | 'warning'
  code: string
  sheetIndex?: number
  partCode?: string
  message: string
}

export interface OptimizeResult {
  solution: Solution
  violations?: Violation[]
  score: number
}

export interface OptimizeResponse {
  id?: string
  /** Plan id, present when the run was archived in the database. */
  planId?: string
  result: OptimizeResult
}

// ------------------------------------------------------------- async jobs ---

export type JobStatus = 'queued' | 'running' | 'done' | 'failed' | 'cancelled'

export type JobEventType =
  | 'snapshot'
  | 'running'
  | 'progress'
  | 'done'
  | 'failed'
  | 'cancelled'

export interface JobSubmitResponse {
  id: string
  status: 'queued'
}

/** One compact progress frame streamed while a job runs. */
export interface JobProgressEvent {
  sheets: number
  placed: number
  requested: number
  yieldPct: number
  wastePct: number
  elapsedMs: number
}

/** Payload of a terminal `done` event. */
export interface JobDoneEvent {
  jobId: string
  planId: string
  metrics: Metrics
}

export interface JobEvent {
  type: JobEventType
  jobId?: string
  /** Monotonic progress counter. */
  seq?: number
  data?: JobView | JobProgressEvent | JobDoneEvent | { error?: string }
}

export interface JobView {
  id: string
  status: JobStatus
  solver?: string
  error?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
  planId?: string
  metrics?: Metrics
  /** The archived OptimizeResult, present when the job is done. */
  result?: OptimizeResult
}

export function isTerminalJobStatus(status: JobStatus): boolean {
  return status === 'done' || status === 'failed' || status === 'cancelled'
}

export interface Health {
  status: string
  db: string
  version: string
  env: string
  time: string
}

export interface SolverCapabilities {
  Dimension: DimensionProfile
  CutMode: CutMode
  Rotation: boolean
  Grain: boolean
  Remnants: boolean
  MaxParts: number
  /** Lower wins when the caller does not name a solver; 0 means unranked. */
  Rank: number
  Description: string
}

export interface SolverInfo {
  name: string
  version: string
  capabilities: SolverCapabilities
}

export interface Meta {
  name: string
  version: string
  env: string
  solvers: SolverInfo[]
}

// ------------------------------------------------------------- optimizer I/O ---

/** A part as the solver sees it: cut sizes, never finished sizes. */
export interface ProblemPart {
  id: string
  code: string
  materialSpecId?: string
  /** 1D cut length in µm. */
  length?: number
  /** 2D cut width in µm. */
  width?: number
  /** 2D cut height in µm. */
  height?: number
  quantity?: number
  grain?: GrainMode
  allowRotate?: boolean
  /** 1 is the most important. */
  priority?: number
  group?: string
}

export interface StockItem {
  id: string
  formatId?: string
  code: string
  label?: string
  length?: number
  width?: number
  height?: number
  quantity?: number
  costPerUnit?: number
  isRemnant?: boolean
  defects?: Rect[]
}

export interface Rules {
  kerf: number
  trim: number
  allowRotate: boolean
  grainMode: GrainMode
  cutMode: CutMode
  maxCutStages: number
  offcutMinW: number
  offcutMinH: number
  offcutMinLength: number
  minPartDim: number
  maxPartsPerSheet: number
  oversAllowedPct: number
  /** Offer physical remnants before fresh catalog stock. */
  preferRemnants?: boolean
}

export interface Weights {
  fillPriority: number
  minSheets: number
  minScrap: number
  minPatterns: number
  minOffcutArea: number
  cost: number
}

export interface Objective {
  weights: Weights
  priorities?: string[]
}

/** One complete optimization request. */
export interface Problem {
  parts: ProblemPart[]
  stocks: StockItem[]
  rules?: Rules
  objective?: Objective
  budgetMs?: number
  seed?: number
}

// ---------------------------------------------------------------- catalog ---

export interface Material {
  id: string
  code: string
  name: string
  dimensionProfile: DimensionProfile
  isActive: boolean
}

export interface CreateMaterialInput {
  code: string
  name: string
  dimensionProfile?: DimensionProfile
  attributes?: Record<string, unknown>
}

export interface StockFormat {
  id: string
  code: string
  materialCode: string
  materialName: string
  specCode: string
  dimensionProfile: DimensionProfile
  thicknessMicron: number
  lengthMicron: number
  widthMicron: number
  heightMicron: number
  onHandQty: number
  costPerUnit: number
}

export interface Part {
  id: string
  code: string
  name: string
  materialSpecId?: string
  finishedLengthMicron: number
  finishedWidthMicron: number
  finishedHeightMicron: number
  grain: GrainMode
  allowRotate: boolean
  priority: number
}

export interface CreatePartInput {
  code: string
  name?: string
  materialSpecId?: string
  finishedLengthMicron?: number
  finishedWidthMicron?: number
  finishedHeightMicron?: number
  grain?: GrainMode
  allowRotate?: boolean
  priority?: number
}

// ------------------------------------------------- physical stock / remnants ---

export type StockItemStatus = 'available' | 'reserved' | 'consumed' | 'retired'

/** One physical piece: a full sheet, a bar or a labelled remnant. */
export interface StockPiece {
  id: string
  formatId?: string
  formatCode?: string
  code: string
  label: string
  materialCode?: string
  specCode?: string
  dimensionProfile?: DimensionProfile
  lengthMicron?: number
  widthMicron?: number
  heightMicron?: number
  isRemnant: boolean
  status: StockItemStatus
  location: string
  costPerUnit: number
  notes?: string
  parentPlanId?: string
  parentSheetIndex?: number
  consumedByPlanId?: string
  consumedAt?: string
  createdAt: string
}

export interface CreateStockPieceInput {
  formatId?: string
  code?: string
  label: string
  lengthMicron?: number
  widthMicron?: number
  heightMicron?: number
  isRemnant?: boolean
  location?: string
  costPerUnit?: number
  notes?: string
}

export interface UpdateStockPieceInput {
  label?: string
  location?: string
  status?: Exclude<StockItemStatus, 'consumed'>
  notes?: string
}

export interface FetchStockItemsArgs {
  status?: StockItemStatus
  isRemnant?: boolean
}

// ---------------------------------------------------------- archived plans ---

export type PlanStatus = 'draft' | 'approved' | 'accepted' | 'archived'

export interface PlanSummary {
  id: string
  jobId?: string
  status: PlanStatus
  version: number
  name?: string
  solver: string
  solverVersion: string
  seed: number
  metrics: Metrics
  createdAt: string
  acceptedAt?: string
}

export interface PlanDetail extends PlanSummary {
  result: OptimizeResult
}

export interface RemnantRef {
  id: string
  label: string
  lengthMicron?: number
  widthMicron?: number
  heightMicron?: number
}

export interface AcceptPlanResponse {
  planId: string
  status: 'accepted'
  sheets: number
  /** Go marshals nil slices as null. */
  remnantsCreated: RemnantRef[] | null
  /** Go marshals nil slices as null. */
  stockConsumed: string[] | null
  formatDecrements: number
  formatShortages: number
}
