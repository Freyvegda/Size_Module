import { Navigate, Route, Routes } from 'react-router-dom'

import { AppShell } from '@/components/layout/AppShell'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { MaterialsPage } from '@/pages/MaterialsPage'
import { PartsPage } from '@/pages/PartsPage'
import { PlanViewerPage } from '@/pages/PlanViewerPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { StockPage } from '@/pages/StockPage'

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
        <Route path="viewer" element={<PlanViewerPage />} />
        <Route path="materials" element={<MaterialsPage />} />
        <Route path="parts" element={<PartsPage />} />
        <Route path="stock" element={<StockPage />} />
        <Route path="jobs" element={<JobsPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
