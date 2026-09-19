import { createAction, createAsyncThunk, createSlice, type PayloadAction } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import { normalizeResult } from '@/lib/results'
import { sampleBarResult, sampleResult } from '@/lib/samplePlan'
import type {
  AcceptPlanResponse,
  CutMode,
  DimensionProfile,
  JobEvent,
  JobProgressEvent,
  JobStatus,
  JobSubmitResponse,
  JobView,
  Metrics,
  OptimizeResponse,
  OptimizeResult,
  Problem,
  Rules,
} from '@/lib/types'
import { isTerminalJobStatus } from '@/lib/types'

/**
 * Mirror of the backend's core.DefaultRules(). The server fills these in when a
 * request has no rules, but a caller that overrides one rule (cut mode) must
 * send the complete set, otherwise the zero values would drop kerf and trim.
 */
const defaultRules: Rules = {
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

/** The 2D demo problem, identical to the backend's jobs.DemoProblem(). */
function demoProblem(): Problem {
  return {
    parts: [
      { id: 'demo-window-a', code: 'WINDOW-1200x1400', width: 1200000, height: 1400000, quantity: 6, allowRotate: true, priority: 1 },
      { id: 'demo-window-b', code: 'WINDOW-800x1000', width: 800000, height: 1000000, quantity: 6, allowRotate: true, priority: 1 },
      { id: 'demo-shelf', code: 'SHELF-500x250', width: 500000, height: 250000, quantity: 10, allowRotate: true, priority: 2 },
    ],
    stocks: [
      { id: 'demo-sheet-large', code: 'SHEET-3210x2250', width: 3210000, height: 2250000, quantity: 4, costPerUnit: 62.5 },
      { id: 'demo-sheet-small', code: 'SHEET-2440x1220', width: 2440000, height: 1220000, quantity: 3, costPerUnit: 28 },
    ],
    budgetMs: 5000,
  }
}

/** The 1D demo problem, identical to the backend's jobs.DemoBarProblem(). */
function demoBarProblem(): Problem {
  return {
    parts: [
      { id: 'demo-rail-long', code: 'RAIL-2400', length: 2400000, quantity: 4 },
      { id: 'demo-rail-mid', code: 'RAIL-1100', length: 1100000, quantity: 6 },
      { id: 'demo-spacer', code: 'SPACER-450', length: 450000, quantity: 8 },
    ],
    stocks: [
      { id: 'demo-bar', code: 'BAR-6000', length: 6000000, width: 60000, quantity: 5, costPerUnit: 14.75 },
    ],
    budgetMs: 5000,
  }
}

/**
 * Builds the demo problem for a dimension profile and cut mode. A guillotine
 * request leaves the rules out so the server applies its documented defaults; a
 * free-cutting request sends the complete default set with `cutMode: free`.
 */
export function demoProblemFor(profile: DimensionProfile, cutMode: CutMode): Problem {
  const problem = profile === '1d' ? demoBarProblem() : demoProblem()
  if (cutMode !== 'free') return problem
  return { ...problem, rules: { ...defaultRules, cutMode: 'free' } }
}

export interface ComparisonEntry {
  solver: string
  result: OptimizeResult
}

export interface RunDemoArgs {
  profile?: DimensionProfile
  cutMode?: CutMode
  /** Omit or pass "auto" to let the server pick. */
  solver?: string
  /** Ask the server to append the plant's available remnants before solving. */
  includeRemnants?: boolean
}

export interface CompareSolversArgs {
  profile: DimensionProfile
  cutMode: CutMode
  /** Only the solvers the caller verified are compatible; the API rejects the rest. */
  solvers: string[]
  /** Ask the server to append the plant's available remnants before solving. */
  includeRemnants?: boolean
}

export interface RunJobArgs {
  profile?: DimensionProfile
  cutMode?: CutMode
  /** Omit or pass "auto" to let the server pick. */
  solver?: string
  /** Ask the server to append the plant's available remnants before queueing. */
  includeRemnants?: boolean
}

/** Metrics of a finished plan, shaped like a progress event. */
function metricsToProgress(metrics: Metrics): JobProgressEvent {
  return {
    sheets: metrics.sheetCount,
    placed: metrics.partsPlaced,
    requested: metrics.partsRequested,
    yieldPct: metrics.yieldPct,
    wastePct: metrics.wastePct,
    elapsedMs: metrics.elapsedMs,
  }
}

// The job-stream actions are created here rather than inside the slice so the
// thunks below can dispatch them; the slice handles them in extraReducers.
export const jobStarted = createAction<{ jobId: string }>('optimizer/jobStarted')
export const jobSnapshot = createAction<JobView>('optimizer/jobSnapshot')
export const jobEvent = createAction<JobEvent>('optimizer/jobEvent')
export const clearJob = createAction('optimizer/clearJob')

interface OptimizerState {
  result?: OptimizeResult
  /** Dimension profile of the loaded result, so the viewer picks the right renderer. */
  dimension: DimensionProfile
  source: 'api' | 'sample' | 'none'
  runStatus: 'idle' | 'loading' | 'ready' | 'error'
  runError?: string
  lastJobId?: string
  comparison?: ComparisonEntry[]
  comparisonProfile: DimensionProfile
  comparisonStatus: 'idle' | 'loading' | 'ready' | 'error'
  comparisonError?: string
  // Asynchronous job pipeline (queue + SSE progress).
  jobId?: string
  jobState?: JobStatus
  jobProgress?: JobProgressEvent
  jobError?: string
  jobRequest: 'idle' | 'starting' | 'watching' | 'error'
  // Acceptance of the archived plan behind the current result.
  lastPlanId?: string
  acceptStatus: 'idle' | 'loading' | 'ready' | 'error'
  acceptResult?: AcceptPlanResponse
  acceptError?: string
}

const initialState: OptimizerState = {
  dimension: '2d',
  source: 'none',
  runStatus: 'idle',
  comparisonProfile: '2d',
  comparisonStatus: 'idle',
  jobRequest: 'idle',
  acceptStatus: 'idle',
}

/**
 * Loads the server-side 2D demo plan. When the API is down the built-in sample
 * is used instead, so the viewer always has something to render.
 */
export const loadDemoPlan = createAsyncThunk('optimizer/loadDemoPlan', async () => {
  try {
    const response = await api.get<OptimizeResponse>('/api/v1/demo/plan')
    return {
      result: response.result,
      source: 'api' as const,
      dimension: '2d' as DimensionProfile,
    }
  } catch {
    return {
      result: sampleResult(),
      source: 'sample' as const,
      dimension: '2d' as DimensionProfile,
    }
  }
})

/** Same fallback story for the 1D bar demo (aluminium profiles). */
export const loadBarDemoPlan = createAsyncThunk('optimizer/loadBarDemoPlan', async () => {
  try {
    const response = await api.get<OptimizeResponse>('/api/v1/demo/bar-plan')
    return {
      result: response.result,
      source: 'api' as const,
      dimension: '1d' as DimensionProfile,
    }
  } catch {
    return {
      result: sampleBarResult(),
      source: 'sample' as const,
      dimension: '1d' as DimensionProfile,
    }
  }
})

/**
 * Runs a demo problem through the API. `profile` picks sheets or bars, `cutMode`
 * picks guillotine or free cutting, and `solver` may be omitted to let the
 * server pick its preferred solver for the problem.
 */
export const runDemoOptimization = createAsyncThunk(
  'optimizer/runDemo',
  async (args: RunDemoArgs | undefined) => {
    const profile = args?.profile ?? '2d'
    const cutMode = args?.cutMode ?? 'guillotine'
    const query = new URLSearchParams()
    if (args?.solver && args.solver !== 'auto') query.set('solver', args.solver)
    if (args?.includeRemnants) query.set('includeRemnants', 'true')
    const suffix = query.size > 0 ? `?${query.toString()}` : ''
    const response = await api.post<OptimizeResponse>(
      `/api/v1/optimize${suffix}`,
      demoProblemFor(profile, cutMode),
    )
    return { response, dimension: profile }
  },
)

/**
 * Runs the same problem through several solvers without archiving the runs, so
 * the results can be compared side by side. The caller filters the solver list
 * to the ones compatible with the profile and cut mode.
 */
export const compareSolvers = createAsyncThunk(
  'optimizer/compareSolvers',
  async (args: CompareSolversArgs) => {
    const problem = demoProblemFor(args.profile, args.cutMode)
    const entries = await Promise.all(
      args.solvers.map(async (solver) => {
        const query = new URLSearchParams({ solver, dryRun: 'true' })
        if (args.includeRemnants) query.set('includeRemnants', 'true')
        const response = await api.post<OptimizeResponse>(
          `/api/v1/optimize?${query.toString()}`,
          problem,
        )
        return { solver, result: response.result } satisfies ComparisonEntry
      }),
    )
    return { entries, profile: args.profile }
  },
)

/**
 * Queues a job and follows it to completion over Server-Sent Events. Progress
 * events update the store live; the final view (with the archived result) is
 * returned so the viewer can show the plan. Only transport problems reject —
 * a failed or cancelled job resolves with its status.
 */
export const startAsyncJob = createAsyncThunk(
  'optimizer/startAsyncJob',
  async (args: RunJobArgs | undefined, { dispatch }) => {
    const profile = args?.profile ?? '2d'
    const cutMode = args?.cutMode ?? 'guillotine'
    const query = new URLSearchParams()
    if (args?.solver && args.solver !== 'auto') query.set('solver', args.solver)
    if (args?.includeRemnants) query.set('includeRemnants', 'true')
    const suffix = query.size > 0 ? `?${query.toString()}` : ''

    const submit = await api.post<JobSubmitResponse>(
      `/api/v1/jobs${suffix}`,
      demoProblemFor(profile, cutMode),
    )
    dispatch(jobStarted({ jobId: submit.id }))

    const view = await new Promise<JobView>((resolve, reject) => {
      const source = new EventSource(`/api/v1/jobs/${submit.id}/events`)
      let settled = false

      const finish = async () => {
        if (settled) return
        settled = true
        source.close()
        try {
          resolve(await api.get<JobView>(`/api/v1/jobs/${submit.id}`))
        } catch (error) {
          reject(error instanceof Error ? error : new Error('could not read the job result'))
        }
      }

      source.onmessage = (message) => {
        try {
          const event = JSON.parse(message.data) as JobEvent
          if (event.type === 'snapshot' && event.data) {
            const snapshot = event.data as JobView
            dispatch(jobSnapshot(snapshot))
            // A worker in another process reports completion through the poll
            // as a terminal snapshot; do not wait for a second event.
            if (isTerminalJobStatus(snapshot.status)) {
              void finish()
              return
            }
          } else if (event.type !== 'snapshot') {
            dispatch(jobEvent(event))
          }
          if (event.type === 'done' || event.type === 'failed' || event.type === 'cancelled') {
            void finish()
          }
        } catch {
          // A malformed frame should not break the stream.
        }
      }

      source.onerror = () => {
        // The server closes the stream after the terminal event; check the job
        // state before calling this a transport failure.
        void (async () => {
          if (settled) return
          try {
            const current = await api.get<JobView>(`/api/v1/jobs/${submit.id}`)
            if (isTerminalJobStatus(current.status)) {
              settled = true
              source.close()
              resolve(current)
              return
            }
          } catch {
            // fall through to the transport error below
          }
          if (!settled) {
            settled = true
            source.close()
            reject(new Error('the progress stream closed unexpectedly'))
          }
        })()
      }
    })

    return { view, dimension: profile }
  },
)

/** Cancels a queued or running job. */
export const cancelAsyncJob = createAsyncThunk(
  'optimizer/cancelAsyncJob',
  async (jobId: string) => {
    return api.post<{ cancelled: boolean }>(`/api/v1/jobs/${jobId}/cancel`, {})
  },
)

/**
 * Accepts an archived plan: consumes the physical pieces it used, decrements
 * catalog stock and registers labelled remnants for its offcuts. The plan must
 * not have been accepted before.
 */
export const acceptPlan = createAsyncThunk('optimizer/acceptPlan', async (planId: string) =>
  api.post<AcceptPlanResponse>(`/api/v1/plans/${planId}/accept`, {}),
)

const optimizerSlice = createSlice({
  name: 'optimizer',
  initialState,
  reducers: {
    setResult(
      state,
      action: PayloadAction<{ result: OptimizeResult; source: OptimizerState['source']; dimension?: DimensionProfile }>,
    ) {
      state.result = normalizeResult(action.payload.result)
      state.source = action.payload.source
      state.dimension = action.payload.dimension ?? state.dimension
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(jobStarted, (state, action) => {
        state.jobId = action.payload.jobId
        state.jobState = 'queued'
        state.jobProgress = undefined
        state.jobError = undefined
        state.jobRequest = 'watching'
        state.lastPlanId = undefined
        state.acceptStatus = 'idle'
        state.acceptResult = undefined
        state.acceptError = undefined
      })
      .addCase(jobSnapshot, (state, action) => {
        state.jobId = action.payload.id
        state.jobState = action.payload.status
        if (action.payload.planId) state.lastPlanId = action.payload.planId
        if (action.payload.error) state.jobError = action.payload.error
        if (action.payload.metrics) state.jobProgress = metricsToProgress(action.payload.metrics)
      })
      .addCase(jobEvent, (state, action) => {
        const event = action.payload
        switch (event.type) {
          case 'running':
            state.jobState = 'running'
            break
          case 'progress':
            state.jobState = 'running'
            if (event.data) state.jobProgress = event.data as JobProgressEvent
            break
          case 'done':
            state.jobState = 'done'
            break
          case 'failed': {
            state.jobState = 'failed'
            const data = event.data as { error?: string } | undefined
            state.jobError = data?.error ?? 'the job failed'
            break
          }
          case 'cancelled':
            state.jobState = 'cancelled'
            break
        }
      })
      .addCase(clearJob, (state) => {
        state.jobId = undefined
        state.jobState = undefined
        state.jobProgress = undefined
        state.jobError = undefined
        state.jobRequest = 'idle'
      })
      .addCase(loadDemoPlan.fulfilled, (state, action) => {
        state.result = normalizeResult(action.payload.result)
        state.source = action.payload.source
        state.dimension = action.payload.dimension
      })
      .addCase(loadBarDemoPlan.fulfilled, (state, action) => {
        state.result = normalizeResult(action.payload.result)
        state.source = action.payload.source
        state.dimension = action.payload.dimension
      })
      .addCase(runDemoOptimization.pending, (state) => {
        state.runStatus = 'loading'
        state.runError = undefined
        state.acceptStatus = 'idle'
        state.acceptResult = undefined
        state.acceptError = undefined
      })
      .addCase(runDemoOptimization.fulfilled, (state, action) => {
        state.runStatus = 'ready'
        state.result = normalizeResult(action.payload.response.result)
        state.dimension = action.payload.dimension
        state.source = 'api'
        state.lastJobId = action.payload.response.id
        state.lastPlanId = action.payload.response.planId
      })
      .addCase(runDemoOptimization.rejected, (state, action) => {
        state.runStatus = 'error'
        state.runError = action.error.message ?? 'Optimization failed'
      })
      .addCase(compareSolvers.pending, (state) => {
        state.comparisonStatus = 'loading'
        state.comparisonError = undefined
      })
      .addCase(compareSolvers.fulfilled, (state, action) => {
        state.comparisonStatus = 'ready'
        state.comparison = action.payload.entries.map((entry) => ({
          ...entry,
          result: normalizeResult(entry.result),
        }))
        state.comparisonProfile = action.payload.profile
      })
      .addCase(compareSolvers.rejected, (state, action) => {
        state.comparisonStatus = 'error'
        state.comparisonError = action.error.message ?? 'Comparison failed'
      })
      .addCase(startAsyncJob.pending, (state) => {
        state.jobRequest = 'starting'
        state.jobError = undefined
        state.acceptStatus = 'idle'
        state.acceptResult = undefined
        state.acceptError = undefined
      })
      .addCase(startAsyncJob.fulfilled, (state, action) => {
        const { view, dimension } = action.payload
        state.jobRequest = 'idle'
        state.jobId = view.id
        state.jobState = view.status
        if (view.planId) state.lastPlanId = view.planId
        if (view.metrics) state.jobProgress = metricsToProgress(view.metrics)
        if (view.error) state.jobError = view.error
        if (view.result) {
          state.result = normalizeResult(view.result)
          state.source = 'api'
          state.dimension = dimension
        }
      })
      .addCase(startAsyncJob.rejected, (state, action) => {
        state.jobRequest = 'error'
        state.jobError = action.error.message ?? 'the job could not be queued'
      })
      .addCase(cancelAsyncJob.fulfilled, (state) => {
        if (state.jobState === 'queued' || state.jobState === 'running') {
          state.jobState = 'cancelled'
        }
      })
      .addCase(acceptPlan.pending, (state) => {
        state.acceptStatus = 'loading'
        state.acceptError = undefined
      })
      .addCase(acceptPlan.fulfilled, (state, action) => {
        state.acceptStatus = 'ready'
        state.acceptResult = action.payload
      })
      .addCase(acceptPlan.rejected, (state, action) => {
        state.acceptStatus = 'error'
        state.acceptError = action.error.message ?? 'the plan could not be accepted'
      })
  },
})

export const { setResult } = optimizerSlice.actions
export default optimizerSlice.reducer
