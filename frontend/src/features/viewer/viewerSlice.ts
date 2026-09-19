import { createSlice, type PayloadAction } from '@reduxjs/toolkit'

import type { EditOperation, SheetPlan } from '@/lib/types'

export type ViewMode = '2d' | '3d'

interface ViewerState {
  mode: ViewMode
  selectedSheet: number
  /** Placement key (placement id when available, part id otherwise). */
  selectedPartId?: string
  showOffcuts: boolean
  explode: number
  // Plan editing: a draft copy of the sheets plus the operations to submit.
  editing: boolean
  draftSheets: SheetPlan[] | null
  pendingOps: EditOperation[]
}

const initialState: ViewerState = {
  mode: '3d',
  selectedSheet: 0,
  showOffcuts: true,
  explode: 0,
  editing: false,
  draftSheets: null,
  pendingOps: [],
}

/** The identity of a placement: the stored id when there is one. */
export function placementKey(placement: { id?: string; partId: string }): string {
  return placement.id ?? placement.partId
}

function cloneSheets(sheets: SheetPlan[]): SheetPlan[] {
  return JSON.parse(JSON.stringify(sheets)) as SheetPlan[]
}

function upsertOp(ops: EditOperation[], op: EditOperation) {
  const index = ops.findIndex((item) => item.placementId === op.placementId)
  if (index >= 0) {
    ops[index] = { ...ops[index], ...op }
  } else {
    ops.push(op)
  }
}

function findPlacement(sheets: SheetPlan[], placementId: string) {
  for (const sheet of sheets) {
    for (const placement of sheet.placements ?? []) {
      if (placementKey(placement) === placementId) {
        return placement
      }
    }
  }
  return undefined
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
    beginEdit(state, action: PayloadAction<SheetPlan[]>) {
      state.editing = true
      state.draftSheets = cloneSheets(action.payload)
      state.pendingOps = []
      state.mode = '2d'
    },
    endEdit(state) {
      state.editing = false
      state.draftSheets = null
      state.pendingOps = []
    },
    movePart(state, action: PayloadAction<{ placementId: string; x: number; y: number }>) {
      if (!state.draftSheets) return
      const placement = findPlacement(state.draftSheets, action.payload.placementId)
      if (!placement) return
      placement.x = action.payload.x
      placement.y = action.payload.y
      upsertOp(state.pendingOps, {
        placementId: action.payload.placementId,
        x: action.payload.x,
        y: action.payload.y,
      })
    },
    rotatePart(state, action: PayloadAction<{ placementId: string }>) {
      if (!state.draftSheets) return
      const placement = findPlacement(state.draftSheets, action.payload.placementId)
      if (!placement) return
      const cx = placement.x + placement.w / 2
      const cy = placement.y + placement.h / 2
      const w = placement.w
      placement.w = placement.h
      placement.h = w
      placement.rotated = !placement.rotated
      placement.x = Math.round(cx - placement.w / 2)
      placement.y = Math.round(cy - placement.h / 2)
      upsertOp(state.pendingOps, {
        placementId: action.payload.placementId,
        x: placement.x,
        y: placement.y,
        rotated: placement.rotated,
      })
    },
    toggleLockPart(state, action: PayloadAction<{ placementId: string }>) {
      if (!state.draftSheets) return
      const placement = findPlacement(state.draftSheets, action.payload.placementId)
      if (!placement) return
      placement.locked = !placement.locked
      upsertOp(state.pendingOps, {
        placementId: action.payload.placementId,
        locked: placement.locked,
      })
    },
    deletePart(state, action: PayloadAction<{ placementId: string }>) {
      if (!state.draftSheets) return
      for (const sheet of state.draftSheets) {
        const index = (sheet.placements ?? []).findIndex(
          (placement) => placementKey(placement) === action.payload.placementId,
        )
        if (index >= 0) {
          sheet.placements.splice(index, 1)
          upsertOp(state.pendingOps, { placementId: action.payload.placementId, delete: true })
          return
        }
      }
    },
  },
})

export const {
  setMode,
  selectSheet,
  selectPart,
  toggleOffcuts,
  setExplode,
  beginEdit,
  endEdit,
  movePart,
  rotatePart,
  toggleLockPart,
  deletePart,
} = viewerSlice.actions
export default viewerSlice.reducer
