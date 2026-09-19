import { createAsyncThunk, createSlice } from '@reduxjs/toolkit'

import { api } from '@/lib/api'
import type {
  CreateMaterialInput,
  CreatePartInput,
  CreateStockPieceInput,
  FetchStockItemsArgs,
  Material,
  Part,
  StockFormat,
  StockPiece,
  UpdateStockPieceInput,
} from '@/lib/types'

type RequestStatus = 'idle' | 'loading' | 'ready' | 'error'

interface CatalogState {
  materials: Material[]
  materialsStatus: RequestStatus
  materialsError?: string
  createMaterialStatus: RequestStatus
  createMaterialError?: string
  parts: Part[]
  partsStatus: RequestStatus
  partsError?: string
  createPartStatus: RequestStatus
  createPartError?: string
  stockFormats: StockFormat[]
  stockStatus: RequestStatus
  stockError?: string
  stockItems: StockPiece[]
  stockItemsStatus: RequestStatus
  stockItemsError?: string
  createStockPieceStatus: RequestStatus
  createStockPieceError?: string
  updateStockPieceStatus: RequestStatus
  updateStockPieceError?: string
}

const initialState: CatalogState = {
  materials: [],
  materialsStatus: 'idle',
  createMaterialStatus: 'idle',
  parts: [],
  partsStatus: 'idle',
  createPartStatus: 'idle',
  stockFormats: [],
  stockStatus: 'idle',
  stockItems: [],
  stockItemsStatus: 'idle',
  createStockPieceStatus: 'idle',
  updateStockPieceStatus: 'idle',
}

// --------------------------------------------------------------- materials ---

export const fetchMaterials = createAsyncThunk('catalog/fetchMaterials', async () => {
  const response = await api.get<{ materials: Material[] }>('/api/v1/materials')
  return response.materials ?? []
})

export const createMaterial = createAsyncThunk(
  'catalog/createMaterial',
  async (input: CreateMaterialInput) => api.post<Material>('/api/v1/materials', input),
)

// ------------------------------------------------------------------- parts ---

export const fetchParts = createAsyncThunk('catalog/fetchParts', async () => {
  const response = await api.get<{ parts: Part[] }>('/api/v1/parts')
  return response.parts ?? []
})

export const createPart = createAsyncThunk('catalog/createPart', async (input: CreatePartInput) =>
  api.post<Part>('/api/v1/parts', input),
)

// ----------------------------------------------------------- stock formats ---

export const fetchStockFormats = createAsyncThunk('catalog/fetchStockFormats', async () => {
  const response = await api.get<{ stockFormats: StockFormat[] }>('/api/v1/stock-formats')
  return response.stockFormats ?? []
})

// ----------------------------------------------------- physical stock pool ---

export const fetchStockItems = createAsyncThunk(
  'catalog/fetchStockItems',
  async (args: FetchStockItemsArgs | undefined) => {
    const query = new URLSearchParams()
    if (args?.status) query.set('status', args.status)
    if (args?.isRemnant !== undefined) query.set('isRemnant', String(args.isRemnant))
    const suffix = query.size > 0 ? `?${query.toString()}` : ''
    const response = await api.get<{ stockItems: StockPiece[] }>(`/api/v1/stock-items${suffix}`)
    return response.stockItems ?? []
  },
)

export const createStockPiece = createAsyncThunk(
  'catalog/createStockPiece',
  async (input: CreateStockPieceInput) => api.post<StockPiece>('/api/v1/stock-items', input),
)

export const updateStockPiece = createAsyncThunk(
  'catalog/updateStockPiece',
  async (args: { id: string; input: UpdateStockPieceInput }) =>
    api.patch<StockPiece>(`/api/v1/stock-items/${args.id}`, args.input),
)

const catalogSlice = createSlice({
  name: 'catalog',
  initialState,
  reducers: {},
  extraReducers: (builder) => {
    builder
      .addCase(fetchMaterials.pending, (state) => {
        state.materialsStatus = 'loading'
        state.materialsError = undefined
      })
      .addCase(fetchMaterials.fulfilled, (state, action) => {
        state.materialsStatus = 'ready'
        state.materials = action.payload
      })
      .addCase(fetchMaterials.rejected, (state, action) => {
        state.materialsStatus = 'error'
        state.materialsError = action.error.message ?? 'Could not load materials'
      })
      .addCase(createMaterial.pending, (state) => {
        state.createMaterialStatus = 'loading'
        state.createMaterialError = undefined
      })
      .addCase(createMaterial.fulfilled, (state, action) => {
        state.createMaterialStatus = 'ready'
        state.materials.unshift(action.payload)
      })
      .addCase(createMaterial.rejected, (state, action) => {
        state.createMaterialStatus = 'error'
        state.createMaterialError = action.error.message ?? 'Could not create material'
      })
      .addCase(fetchParts.pending, (state) => {
        state.partsStatus = 'loading'
        state.partsError = undefined
      })
      .addCase(fetchParts.fulfilled, (state, action) => {
        state.partsStatus = 'ready'
        state.parts = action.payload
      })
      .addCase(fetchParts.rejected, (state, action) => {
        state.partsStatus = 'error'
        state.partsError = action.error.message ?? 'Could not load parts'
      })
      .addCase(createPart.pending, (state) => {
        state.createPartStatus = 'loading'
        state.createPartError = undefined
      })
      .addCase(createPart.fulfilled, (state, action) => {
        state.createPartStatus = 'ready'
        state.parts.unshift(action.payload)
      })
      .addCase(createPart.rejected, (state, action) => {
        state.createPartStatus = 'error'
        state.createPartError = action.error.message ?? 'Could not create part'
      })
      .addCase(fetchStockFormats.pending, (state) => {
        state.stockStatus = 'loading'
        state.stockError = undefined
      })
      .addCase(fetchStockFormats.fulfilled, (state, action) => {
        state.stockStatus = 'ready'
        state.stockFormats = action.payload
      })
      .addCase(fetchStockFormats.rejected, (state, action) => {
        state.stockStatus = 'error'
        state.stockError = action.error.message ?? 'Could not load stock formats'
      })
      .addCase(fetchStockItems.pending, (state) => {
        state.stockItemsStatus = 'loading'
        state.stockItemsError = undefined
      })
      .addCase(fetchStockItems.fulfilled, (state, action) => {
        state.stockItemsStatus = 'ready'
        state.stockItems = action.payload
      })
      .addCase(fetchStockItems.rejected, (state, action) => {
        state.stockItemsStatus = 'error'
        state.stockItemsError = action.error.message ?? 'Could not load physical stock'
      })
      .addCase(createStockPiece.pending, (state) => {
        state.createStockPieceStatus = 'loading'
        state.createStockPieceError = undefined
      })
      .addCase(createStockPiece.fulfilled, (state, action) => {
        state.createStockPieceStatus = 'ready'
        state.stockItems.unshift(action.payload)
      })
      .addCase(createStockPiece.rejected, (state, action) => {
        state.createStockPieceStatus = 'error'
        state.createStockPieceError = action.error.message ?? 'Could not register the piece'
      })
      .addCase(updateStockPiece.pending, (state) => {
        state.updateStockPieceStatus = 'loading'
        state.updateStockPieceError = undefined
      })
      .addCase(updateStockPiece.fulfilled, (state, action) => {
        state.updateStockPieceStatus = 'ready'
        const index = state.stockItems.findIndex((item) => item.id === action.payload.id)
        if (index >= 0) state.stockItems[index] = action.payload
      })
      .addCase(updateStockPiece.rejected, (state, action) => {
        state.updateStockPieceStatus = 'error'
        state.updateStockPieceError = action.error.message ?? 'Could not update the piece'
      })
  },
})

export default catalogSlice.reducer
