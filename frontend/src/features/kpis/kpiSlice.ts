import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type { KpiReport } from '@/lib/types'

type RequestStatus = 'idle' | 'loading' | 'ready' | 'error'

interface KpiState {
  report?: KpiReport
  status: RequestStatus
  error?: string
  /** Reporting window in days. */
  days: number
}

const initialState: KpiState = {
  status: 'idle',
  days: 90,
}

/**
 * Loads the realized-yield report. `realized` covers accepted plans and
 * `created` every plan in the window, so the dashboard can compare the
 * pipeline with what the shop committed to.
 */
export const fetchKpis = createAsyncThunk('kpis/fetchKpis', async (days: number | undefined) => {
  const effective = days && days > 0 ? Math.min(days, 3650) : 90
  const report = await api.get<KpiReport>(`/api/v1/kpis?days=${effective}`)
  return { report, days: effective }
})

const kpiSlice = createSlice({
  name: 'kpis',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchKpis.pending, (state) => {
        state.status = 'loading'
        state.error = undefined
      })
      .addCase(fetchKpis.fulfilled, (state, action) => {
        state.status = 'ready'
        state.report = action.payload.report
        state.days = action.payload.days
      })
      .addCase(fetchKpis.rejected, (state, action) => {
        state.status = 'error'
        state.error = action.error.message ?? 'Could not load the KPI report'
      })
  },
})

export default kpiSlice.reducer
