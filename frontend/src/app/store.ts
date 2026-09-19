import { configureStore } from '@reduxjs/toolkit'

import backendReducer from '@/features/backend/backendSlice'
import assemblyReducer from '@/features/assemblies/assemblySlice'
import campaignReducer from '@/features/campaigns/campaignSlice'
import catalogReducer from '@/features/catalog/catalogSlice'
import kpiReducer from '@/features/kpis/kpiSlice'
import optimizerReducer from '@/features/optimizer/optimizerSlice'
import plansReducer from '@/features/plans/plansSlice'
import viewerReducer from '@/features/viewer/viewerSlice'

export const store = configureStore({
  reducer: {
    backend: backendReducer,
    assemblies: assemblyReducer,
    campaigns: campaignReducer,
    catalog: catalogReducer,
    kpis: kpiReducer,
    optimizer: optimizerReducer,
    plans: plansReducer,
    viewer: viewerReducer,
  },
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
