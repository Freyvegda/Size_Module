import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type { PlanSummary } from '@/lib/types'

type RequestStatus = 'idle' | 'loading' | 'ready' | 'error'

interface PlansState {
  plans: PlanSummary[]
  status: RequestStatus
  error?: string
}

const initialState: PlansState = {
  plans: [],
  status: 'idle',
}

export const fetchPlans = createAsyncThunk('plans/fetchPlans', async () => {
  const response = await api.get<{ plans: PlanSummary[] }>('/api/v1/plans')
  return response.plans ?? []
})

const plansSlice = createSlice({
  name: 'plans',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchPlans.pending, (state) => {
        state.status = 'loading'
        state.error = undefined
      })
      .addCase(fetchPlans.fulfilled, (state, action) => {
        state.status = 'ready'
        state.plans = action.payload
      })
      .addCase(fetchPlans.rejected, (state, action) => {
        state.status = 'error'
        state.error = action.error.message ?? 'Could not load plans'
      })
  },
})

export default plansSlice.reducer
