import { createAsyncThunk, createSlice, type PayloadAction } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type { Assembly, CreateAssemblyInput } from '@/lib/types'

type RequestStatus = 'idle' | 'loading' | 'ready' | 'error'

interface AssemblyState {
  assemblies: Assembly[]
  assembliesStatus: RequestStatus
  assembliesError?: string
  selectedId?: string
  createAssemblyStatus: RequestStatus
  createAssemblyError?: string
}

const initialState: AssemblyState = {
  assemblies: [],
  assembliesStatus: 'idle',
  createAssemblyStatus: 'idle',
}

export const fetchAssemblies = createAsyncThunk('assemblies/fetchAssemblies', async () => {
  const response = await api.get<{ assemblies: Assembly[] }>('/api/v1/assemblies')
  return response.assemblies ?? []
})

export const fetchAssembly = createAsyncThunk('assemblies/fetchAssembly', async (id: string) =>
  api.get<Assembly>(`/api/v1/assemblies/${id}`),
)

export const createAssembly = createAsyncThunk(
  'assemblies/createAssembly',
  async (input: CreateAssemblyInput) => api.post<Assembly>('/api/v1/assemblies', input),
)

const assemblySlice = createSlice({
  name: 'assemblies',
  initialState,
  reducers: {
    selectAssembly(state, action: PayloadAction<string | undefined>) {
      state.selectedId = action.payload
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(fetchAssemblies.pending, (state) => {
        state.assembliesStatus = 'loading'
        state.assembliesError = undefined
      })
      .addCase(fetchAssemblies.fulfilled, (state, action) => {
        state.assembliesStatus = 'ready'
        state.assemblies = action.payload
      })
      .addCase(fetchAssemblies.rejected, (state, action) => {
        state.assembliesStatus = 'error'
        state.assembliesError = action.error.message ?? 'Could not load assemblies'
      })
      .addCase(fetchAssembly.fulfilled, (state, action) => {
        const index = state.assemblies.findIndex((item) => item.id === action.payload.id)
        if (index >= 0) state.assemblies[index] = action.payload
        else state.assemblies.push(action.payload)
        state.selectedId = action.payload.id
      })
      .addCase(createAssembly.pending, (state) => {
        state.createAssemblyStatus = 'loading'
        state.createAssemblyError = undefined
      })
      .addCase(createAssembly.fulfilled, (state, action) => {
        state.createAssemblyStatus = 'ready'
        state.assemblies.unshift(action.payload)
        state.selectedId = action.payload.id
      })
      .addCase(createAssembly.rejected, (state, action) => {
        state.createAssemblyStatus = 'error'
        state.createAssemblyError = action.error.message ?? 'Could not create the assembly'
      })
  },
})

export const { selectAssembly } = assemblySlice.actions
export default assemblySlice.reducer
