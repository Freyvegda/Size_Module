import type { OptimizeResult } from './types'

/**
 * The Go API marshals nil slices as JSON `null` — a demo plan with no unplaced
 * parts comes back as `"unplaced": null`, and a sheet with no reusable offcut
 * as `"offcuts": null`. The UI wants arrays, so normalize every result once at
 * the store boundary instead of scattering null checks through components.
 */
export function normalizeResult(result: OptimizeResult): OptimizeResult {
  return {
    ...result,
    solution: {
      ...result.solution,
      sheets: (result.solution.sheets ?? []).map((sheet) => ({
        ...sheet,
        placements: sheet.placements ?? [],
        offcuts: sheet.offcuts ?? [],
      })),
      unplaced: result.solution.unplaced ?? [],
      notes: result.solution.notes ?? [],
    },
    violations: result.violations ?? [],
  }
}
