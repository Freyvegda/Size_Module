// Solver selection helpers. These mirror the backend's registry rules
// (internal/optimizer/core/solver.go + profile.go) so the UI never offers a
// solver the API would reject with a 422.

import type { CutMode, DimensionProfile, SolverCapabilities, SolverInfo } from './types'

/**
 * Mirrors core.CutModeCompatible: a guillotine layout is always valid on a
 * machine that accepts free cutting; the reverse is not true.
 */
export function cutModeCompatible(have: CutMode, want: CutMode): boolean {
  if (have === want) return true
  return want === 'free' && have === 'guillotine'
}

/** A solver serves a problem when its dimension matches and its cut mode is compatible. */
export function solverServes(
  solver: SolverInfo,
  profile: DimensionProfile,
  cutMode: CutMode,
): boolean {
  const caps = solver.capabilities
  return caps.Dimension === profile && cutModeCompatible(caps.CutMode, cutMode)
}

/** Backend rank rule: lower wins; 0 means unranked and sorts last. */
export function solverRank(solver: SolverInfo): number {
  const rank = solver.capabilities.Rank
  return rank > 0 ? rank : 1000
}

/** Solvers serving a profile + cut mode, ordered exactly like the registry. */
export function solversFor(
  solvers: SolverInfo[],
  profile: DimensionProfile,
  cutMode: CutMode,
): SolverInfo[] {
  return solvers
    .filter((solver) => solverServes(solver, profile, cutMode))
    .sort((a, b) => solverRank(a) - solverRank(b) || a.name.localeCompare(b.name))
}

/** Short human label used in pickers and tables. */
export function solverSummary(capabilities: SolverCapabilities): string {
  const rank = capabilities.Rank > 0 ? `rank ${capabilities.Rank}` : 'unranked'
  return `${capabilities.Dimension} · ${capabilities.CutMode} · ${rank}`
}
