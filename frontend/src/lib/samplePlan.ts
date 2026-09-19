import type { Metrics, OptimizeResult, Placement, Rect, SheetPlan } from './types'

// Hand-built fallback plans so the viewer renders even when the Go API is not
// running. The 2D layout mirrors what shelf-2d produces for the demo problem:
// strip 1 holds two 1200x1400 panes, three strips below hold 500x250 shelves.
// The 1D layout mirrors what ffd-1d produces for the bar demo: four bars,
// the last one with a reusable offcut.

const MM = 1000
const KERF = 4 * MM
const TRIM = 10 * MM

function part(code: string, x: number, y: number, w: number, h: number, rotated = false): Placement {
  return { partId: code, partCode: code, x, y, w, h, rotated, priority: 1 }
}

function metricsFor(sheets: SheetPlan[], requested: number): Metrics {
  let stockAreaM2 = 0
  let partAreaM2 = 0
  let offcutAreaM2 = 0
  for (const sheet of sheets) {
    stockAreaM2 += (sheet.width / 1e6) * (sheet.height / 1e6)
    for (const pl of sheet.placements) {
      partAreaM2 += (pl.w / 1e6) * (pl.h / 1e6)
    }
    for (const off of sheet.offcuts ?? []) {
      offcutAreaM2 += (off.w / 1e6) * (off.h / 1e6)
    }
  }
  const yieldPct = stockAreaM2 > 0 ? (100 * partAreaM2) / stockAreaM2 : 0
  return {
    sheetCount: sheets.length,
    partsPlaced: sheets.reduce((sum, s) => sum + s.placements.length, 0),
    partsRequested: requested,
    stockAreaM2,
    partAreaM2,
    kerfAreaM2: 0.04,
    trimAreaM2: 0.18,
    scrapAreaM2: Math.max(0, stockAreaM2 - partAreaM2 - offcutAreaM2),
    offcutAreaM2,
    yieldPct,
    wastePct: 100 - yieldPct,
    patternCount: sheets.length,
    cost: 153,
    elapsedMs: 3,
  }
}

export function sampleResult(): OptimizeResult {
  const shelfStrips: Placement[] = []
  for (let strip = 0; strip < 3; strip++) {
    const y = 1414 + strip * (250 * MM + 4 * MM)
    for (let i = 0; i < 2; i++) {
      shelfStrips.push(part('SHELF-500x250', 10 * MM + i * (500 * MM + 4 * MM), y, 500 * MM, 250 * MM))
    }
  }

  const sheet1: SheetPlan = {
    index: 0,
    stockId: 'sample-large',
    stockCode: 'SHEET-3210x2250',
    label: 'Sheet 1',
    width: 3210 * MM,
    height: 2250 * MM,
    placements: [
      part('WINDOW-1200x1400', 10 * MM, 10 * MM, 1200 * MM, 1400 * MM),
      part('WINDOW-1200x1400', 1214 * MM, 10 * MM, 1200 * MM, 1400 * MM),
      ...shelfStrips,
    ],
    offcuts: [{ x: 2430 * MM, y: 10 * MM, w: 770 * MM, h: 1400 * MM } satisfies Rect],
    cutSteps: [
      'vertical (rip) cut at 2414 mm from the region edge, kerf 4 mm',
      'horizontal (crosscut) cut at 1400 mm from the region edge, kerf 4 mm',
    ],
  }

  const sheet2: SheetPlan = {
    index: 1,
    stockId: 'sample-small',
    stockCode: 'SHEET-2440x1220',
    label: 'Sheet 2',
    width: 2440 * MM,
    height: 1220 * MM,
    placements: [
      part('WINDOW-800x1000', 10 * MM, 10 * MM, 800 * MM, 1000 * MM),
      part('WINDOW-800x1000', 814 * MM, 10 * MM, 800 * MM, 1000 * MM),
    ],
    offcuts: [{ x: 1624 * MM, y: 10 * MM, w: 806 * MM, h: 1000 * MM }],
    cutSteps: ['horizontal (crosscut) cut at 1000 mm from the region edge, kerf 4 mm'],
  }

  const sheets = [sheet1, sheet2]
  return {
    solution: {
      solver: 'sample',
      solverVersion: 'fallback',
      seed: 0,
      sheets,
      unplaced: [],
      metrics: metricsFor(sheets, 10),
      notes: [
        'Sample layout: the Go API was not reachable, so the viewer is showing built-in demo data.',
        'Start the API with: cd backend; go run ./cmd/cutoptics',
      ],
    },
    violations: [],
    score: 0,
  }
}

// ------------------------------------------------------------------- 1D ---

const BAR_LENGTH = 6000 * MM
const BAR_HEIGHT = 60 * MM
const BAR_STOCK = 'BAR-6000'

function barPiece(code: string, x: number, length: number): Placement {
  return { partId: code, partCode: code, x, y: 0, w: length, h: BAR_HEIGHT, rotated: false, priority: 100 }
}

/** Lays pieces left to right with the backend's default trim (10 mm) and kerf (4 mm). */
function barPieces(pieces: Array<[string, number]>): Placement[] {
  let cursor = TRIM
  return pieces.map(([code, length]) => {
    const placement = barPiece(code, cursor, length)
    cursor += length + KERF
    return placement
  })
}

function barCutSteps(placements: Placement[], trailing?: number): string[] {
  const steps = placements.slice(1).map(
    (placement) =>
      `vertical (rip) cut at ${(placement.x - KERF) / MM} mm from the region edge, kerf ${KERF / MM} mm`,
  )
  if (trailing) {
    steps.push(`vertical (rip) cut at ${trailing / MM} mm from the region edge, kerf ${KERF / MM} mm`)
  }
  return steps
}

function barMetrics(sheets: SheetPlan[], requested: number): Metrics {
  let stockAreaM2 = 0
  let partAreaM2 = 0
  let offcutAreaM2 = 0
  for (const sheet of sheets) {
    stockAreaM2 += (sheet.width / 1e6) * (sheet.height / 1e6)
    for (const placement of sheet.placements) {
      partAreaM2 += (placement.w / 1e6) * (placement.h / 1e6)
    }
    for (const off of sheet.offcuts ?? []) {
      offcutAreaM2 += (off.w / 1e6) * (off.h / 1e6)
    }
  }
  const yieldPct = stockAreaM2 > 0 ? (100 * partAreaM2) / stockAreaM2 : 0
  return {
    sheetCount: sheets.length,
    partsPlaced: sheets.reduce((sum, sheet) => sum + sheet.placements.length, 0),
    partsRequested: requested,
    stockAreaM2,
    partAreaM2,
    kerfAreaM2: 0.0032,
    trimAreaM2: 0.0048,
    scrapAreaM2: Math.max(0, stockAreaM2 - partAreaM2 - offcutAreaM2),
    offcutAreaM2,
    yieldPct,
    wastePct: 100 - yieldPct,
    patternCount: 3,
    cost: 59,
    stockLengthM: 24,
    usedLengthM: 19.8,
    elapsedMs: 2,
  }
}

/**
 * Fallback for /api/v1/demo/bar-plan: the FFD layout of the bar demo problem
 * (RAIL-2400 x4, RAIL-1100 x6, SPACER-450 x8 on five 6 m bars). Four bars are
 * used and the last one keeps a 3710 mm offcut.
 */
export function sampleBarResult(): OptimizeResult {
  const placements: Array<[string, number]> = [
    ['RAIL-2400', 2400 * MM],
    ['RAIL-2400', 2400 * MM],
    ['RAIL-1100', 1100 * MM],
  ]
  const bars: SheetPlan[] = [
    { index: 0, placements: barPieces(placements) },
    { index: 1, placements: barPieces(placements) },
    {
      index: 2,
      placements: barPieces([
        ['RAIL-1100', 1100 * MM],
        ['RAIL-1100', 1100 * MM],
        ['RAIL-1100', 1100 * MM],
        ['RAIL-1100', 1100 * MM],
        ['SPACER-450', 450 * MM],
        ['SPACER-450', 450 * MM],
        ['SPACER-450', 450 * MM],
      ]),
    },
    {
      index: 3,
      placements: barPieces([
        ['SPACER-450', 450 * MM],
        ['SPACER-450', 450 * MM],
        ['SPACER-450', 450 * MM],
        ['SPACER-450', 450 * MM],
        ['SPACER-450', 450 * MM],
      ]),
      offcuts: [{ x: 2280 * MM, y: 0, w: 3710 * MM, h: BAR_HEIGHT }],
    },
  ].map((bar) => ({
    ...bar,
    stockId: 'sample-bar',
    stockCode: BAR_STOCK,
    label: `Bar ${bar.index + 1}`,
    width: BAR_LENGTH,
    height: BAR_HEIGHT,
    cutSteps: barCutSteps(bar.placements, bar.index === 3 ? 2276 * MM : undefined),
  }))

  return {
    solution: {
      solver: 'sample',
      solverVersion: 'fallback',
      seed: 0,
      sheets: bars,
      unplaced: [],
      metrics: barMetrics(bars, 18),
      notes: [
        'Sample bar layout: the Go API was not reachable, so the viewer is showing built-in demo data.',
        'Start the API with: cd backend; go run ./cmd/cutoptics',
      ],
    },
    violations: [],
    score: 0,
  }
}
