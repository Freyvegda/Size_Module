import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type { Health, Meta } from '@/lib/types'

interface BackendState {
  health?: Health
  meta?: Meta
  status: 'idle' | 'loading' | 'ready' | 'error'
  error?: string
}

const initialState: BackendState = {
  status: 'idle',
}

export const fetchBackend = createAsyncThunk('backend/fetch', async () => {
  const [health, meta] = await Promise.all([
    api.get<Health>('/healthz'),
    api.get<Meta>('/api/v1/meta'),
  ])
  return { health, meta }
})

const backendSlice = createSlice({
  name: 'backend',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchBackend.pending, (state) => {
        state.status = 'loading'
        state.error = undefined
      })
      .addCase(fetchBackend.fulfilled, (state, action) => {
        state.status = 'ready'
        state.health = action.payload.health
        state.meta = action.payload.meta
      })
      .addCase(fetchBackend.rejected, (state, action) => {
        state.status = 'error'
        state.error = action.error.message ?? 'API unreachable'
      })
  },
})

export default backendSlice.reducer
