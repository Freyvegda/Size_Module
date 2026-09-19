import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type {
  CampaignDetail,
  CampaignSummary,
  CreateCampaignInput,
  CreateCampaignItemInput,
  RunCampaignNextInput,
  UpdateCampaignInput,
} from '@/lib/types'

type RequestStatus = 'idle' | 'loading' | 'ready' | 'error'

interface CampaignState {
  campaigns: CampaignSummary[]
  listStatus: RequestStatus
  listError?: string
  detail?: CampaignDetail
  detailStatus: RequestStatus
  detailError?: string
  /** Status of create/add/remove/run mutations. */
  mutateStatus: RequestStatus
  mutateError?: string
}

const initialState: CampaignState = {
  campaigns: [],
  listStatus: 'idle',
  detailStatus: 'idle',
  mutateStatus: 'idle',
}

export const fetchCampaigns = createAsyncThunk('campaigns/fetchCampaigns', async () => {
  const response = await api.get<{ campaigns: CampaignSummary[] }>('/api/v1/campaigns')
  return response.campaigns ?? []
})

export const fetchCampaign = createAsyncThunk('campaigns/fetchCampaign', async (id: string) =>
  api.get<CampaignDetail>(`/api/v1/campaigns/${id}`),
)

export const createCampaign = createAsyncThunk(
  'campaigns/createCampaign',
  async (input: CreateCampaignInput) => api.post<CampaignDetail>('/api/v1/campaigns', input),
)

export const updateCampaign = createAsyncThunk(
  'campaigns/updateCampaign',
  async (args: { id: string; input: UpdateCampaignInput }) =>
    api.patch<CampaignDetail>(`/api/v1/campaigns/${args.id}`, args.input),
)

export const addCampaignItem = createAsyncThunk(
  'campaigns/addCampaignItem',
  async (args: { id: string; input: CreateCampaignItemInput }) =>
    api.post<CampaignDetail>(`/api/v1/campaigns/${args.id}/items`, args.input),
)

export const removeCampaignItem = createAsyncThunk(
  'campaigns/removeCampaignItem',
  async (args: { id: string; itemId: string }) =>
    api.delete<CampaignDetail>(`/api/v1/campaigns/${args.id}/items/${args.itemId}`),
)

export const runCampaignNext = createAsyncThunk(
  'campaigns/runCampaignNext',
  async (args: RunCampaignNextInput) =>
    api.post<CampaignDetail>(`/api/v1/campaigns/${args.campaignId}/run-next`, {
      solver: args.solver,
      budgetMs: args.budgetMs,
    }),
)

function toSummary(detail: CampaignDetail): CampaignSummary {
  return {
    id: detail.id,
    code: detail.code,
    name: detail.name,
    status: detail.status,
    budgetMs: detail.budgetMs,
    seed: detail.seed,
    rules: detail.rules,
    objective: detail.objective,
    stock: detail.stock,
    initialStock: detail.initialStock,
    createdAt: detail.createdAt,
    updatedAt: detail.updatedAt,
    completedAt: detail.completedAt,
  }
}

function storeDetail(state: CampaignState, detail: CampaignDetail) {
  state.detail = detail
  const summary = toSummary(detail)
  const index = state.campaigns.findIndex((campaign) => campaign.id === detail.id)
  if (index >= 0) {
    state.campaigns[index] = summary
  } else {
    state.campaigns.unshift(summary)
  }
}

const campaignSlice = createSlice({
  name: 'campaigns',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchCampaigns.pending, (state) => {
        state.listStatus = 'loading'
        state.listError = undefined
      })
      .addCase(fetchCampaigns.fulfilled, (state, action) => {
        state.listStatus = 'ready'
        state.campaigns = action.payload
      })
      .addCase(fetchCampaigns.rejected, (state, action) => {
        state.listStatus = 'error'
        state.listError = action.error.message ?? 'Could not load campaigns'
      })
      .addCase(fetchCampaign.pending, (state) => {
        state.detailStatus = 'loading'
        state.detailError = undefined
      })
      .addCase(fetchCampaign.fulfilled, (state, action) => {
        state.detailStatus = 'ready'
        storeDetail(state, action.payload)
      })
      .addCase(fetchCampaign.rejected, (state, action) => {
        state.detailStatus = 'error'
        state.detailError = action.error.message ?? 'Could not load the campaign'
      })
      .addCase(createCampaign.pending, (state) => {
        state.mutateStatus = 'loading'
        state.mutateError = undefined
      })
      .addCase(createCampaign.fulfilled, (state, action) => {
        state.mutateStatus = 'ready'
        storeDetail(state, action.payload)
      })
      .addCase(createCampaign.rejected, (state, action) => {
        state.mutateStatus = 'error'
        state.mutateError = action.error.message ?? 'Could not create the campaign'
      })
      .addCase(updateCampaign.fulfilled, (state, action) => {
        state.mutateStatus = 'ready'
        storeDetail(state, action.payload)
      })
      .addCase(updateCampaign.rejected, (state, action) => {
        state.mutateStatus = 'error'
        state.mutateError = action.error.message ?? 'Could not update the campaign'
      })
      .addCase(addCampaignItem.pending, (state) => {
        state.mutateStatus = 'loading'
        state.mutateError = undefined
      })
      .addCase(addCampaignItem.fulfilled, (state, action) => {
        state.mutateStatus = 'ready'
        storeDetail(state, action.payload)
      })
      .addCase(addCampaignItem.rejected, (state, action) => {
        state.mutateStatus = 'error'
        state.mutateError = action.error.message ?? 'Could not add the item'
      })
      .addCase(removeCampaignItem.fulfilled, (state, action) => {
        state.mutateStatus = 'ready'
        storeDetail(state, action.payload)
      })
      .addCase(removeCampaignItem.rejected, (state, action) => {
        state.mutateStatus = 'error'
        state.mutateError = action.error.message ?? 'Could not remove the item'
      })
      .addCase(runCampaignNext.pending, (state) => {
        state.mutateStatus = 'loading'
        state.mutateError = undefined
      })
      .addCase(runCampaignNext.fulfilled, (state, action) => {
        state.mutateStatus = 'ready'
        storeDetail(state, action.payload)
      })
      .addCase(runCampaignNext.rejected, (state, action) => {
        state.mutateStatus = 'error'
        state.mutateError = action.error.message ?? 'Could not run the next item'
      })
  },
})

export default campaignSlice.reducer
