import { configureStore } from '@reduxjs/toolkit'

import backendReducer from '@/features/backend/backendSlice'
import catalogReducer from '@/features/catalog/catalogSlice'
import optimizerReducer from '@/features/optimizer/optimizerSlice'
import viewerReducer from '@/features/viewer/viewerSlice'

export const store = configureStore({
  reducer: {
    backend: backendReducer,
    catalog: catalogReducer,
    optimizer: optimizerReducer,
    viewer: viewerReducer,
  },
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
