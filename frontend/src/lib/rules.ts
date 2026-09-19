// Shared rules/objective defaults and helpers.
//
// The engine fills defaults only when the whole rules object is empty, so a
// caller that overrides one field (e.g. cut mode) must send the complete set.
// These mirror backend/internal/optimizer/core: keep them in sync with
// core.DefaultRules() and core.DefaultWeights().

import type { CutMode, Rules, Weights } from '@/lib/types'

/** Mirror of the backend's core.DefaultRules(). */
export const defaultRules: Rules = {
  kerf: 4000,
  trim: 10000,
  allowRotate: true,
  grainMode: 'none',
  cutMode: 'guillotine',
  maxCutStages: 0,
  offcutMinW: 300000,
  offcutMinH: 300000,
  offcutMinLength: 300000,
  minPartDim: 0,
  maxPartsPerSheet: 0,
  oversAllowedPct: 0,
  preferRemnants: true,
}

/** Mirror of the backend's core.DefaultWeights(). */
export const defaultWeights: Weights = {
  fillPriority: 1000,
  minSheets: 1,
  minScrap: 1,
  minPatterns: 1,
  minOffcutArea: 0.25,
  cost: 1,
}

/**
 * The explicit rules to attach to an optimization request, or undefined to let
 * the server apply a stored profile (via `?rulesProfileId=`) or its own
 * defaults. When the UI overrides a single field on top of a stored profile
 * (for example forcing free cutting), the complete profile rules are merged so
 * kerf/trim are never dropped.
 */
export function rulesOverrideFor(base: Rules | undefined, cutMode: CutMode): Rules | undefined {
  if (!base) {
    // No stored profile: guillotine uses the server defaults, free cutting
    // needs the complete set so kerf/trim are not lost.
    return cutMode === 'free' ? { ...defaultRules, cutMode: 'free' } : undefined
  }
  // The profile's cut mode already matches, so let the server resolve it by id.
  if (base.cutMode === cutMode) return undefined
  return { ...base, cutMode }
}
