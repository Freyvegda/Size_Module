import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type { RulesProfile, SaveRulesProfileInput } from '@/lib/types'

type RequestStatus = 'idle' | 'loading' | 'ready' | 'error'

interface RulesState {
  profiles: RulesProfile[]
  status: RequestStatus
  error?: string
  saveStatus: RequestStatus
  saveError?: string
}

const initialState: RulesState = {
  profiles: [],
  status: 'idle',
  saveStatus: 'idle',
}

export const fetchRulesProfiles = createAsyncThunk('rules/fetchProfiles', async () => {
  const response = await api.get<{ rulesProfiles: RulesProfile[] }>('/api/v1/rules-profiles')
  return response.rulesProfiles ?? []
})

export const createRulesProfile = createAsyncThunk(
  'rules/createProfile',
  async (input: SaveRulesProfileInput) =>
    api.post<RulesProfile>('/api/v1/rules-profiles', input),
)

export const updateRulesProfile = createAsyncThunk(
  'rules/updateProfile',
  async (args: { id: string; input: SaveRulesProfileInput }) =>
    api.put<RulesProfile>(`/api/v1/rules-profiles/${args.id}`, args.input),
)

const rulesSlice = createSlice({
  name: 'rules',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchRulesProfiles.pending, (state) => {
        state.status = 'loading'
        state.error = undefined
      })
      .addCase(fetchRulesProfiles.fulfilled, (state, action) => {
        state.status = 'ready'
        state.profiles = action.payload
      })
      .addCase(fetchRulesProfiles.rejected, (state, action) => {
        state.status = 'error'
        state.error = action.error.message ?? 'Could not load rules profiles'
      })
      .addCase(createRulesProfile.pending, (state) => {
        state.saveStatus = 'loading'
        state.saveError = undefined
      })
      .addCase(createRulesProfile.fulfilled, (state, action) => {
        state.saveStatus = 'ready'
        state.profiles.unshift(action.payload)
      })
      .addCase(createRulesProfile.rejected, (state, action) => {
        state.saveStatus = 'error'
        state.saveError = action.error.message ?? 'Could not create the rules profile'
      })
      .addCase(updateRulesProfile.pending, (state) => {
        state.saveStatus = 'loading'
        state.saveError = undefined
      })
      .addCase(updateRulesProfile.fulfilled, (state, action) => {
        state.saveStatus = 'ready'
        const index = state.profiles.findIndex((profile) => profile.id === action.payload.id)
        if (index >= 0) state.profiles[index] = action.payload
      })
      .addCase(updateRulesProfile.rejected, (state, action) => {
        state.saveStatus = 'error'
        state.saveError = action.error.message ?? 'Could not update the rules profile'
      })
  },
})

export default rulesSlice.reducer
