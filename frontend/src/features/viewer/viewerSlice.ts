import { createSlice, type PayloadAction } from '@reduxjs/toolkit'

export type ViewMode = '2d' | '3d'

interface ViewerState {
  mode: ViewMode
  selectedSheet: number
  selectedPartId?: string
  showOffcuts: boolean
  explode: number
}

const initialState: ViewerState = {
  mode: '3d',
  selectedSheet: 0,
  showOffcuts: true,
  explode: 0,
}

const viewerSlice = createSlice({
  name: 'viewer',
  initialState,
  reducers: {
    setMode(state, action: PayloadAction<ViewMode>) {
      state.mode = action.payload
    },
    selectSheet(state, action: PayloadAction<number>) {
      state.selectedSheet = action.payload
    },
    selectPart(state, action: PayloadAction<string | undefined>) {
      state.selectedPartId = action.payload
    },
    toggleOffcuts(state) {
      state.showOffcuts = !state.showOffcuts
    },
    setExplode(state, action: PayloadAction<number>) {
      state.explode = action.payload
    },
  },
})

export const { setMode, selectSheet, selectPart, toggleOffcuts, setExplode } = viewerSlice.actions
export default viewerSlice.reducer
