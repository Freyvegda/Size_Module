import { Navigate, Route, Routes } from 'react-router-dom'

import { AppShell } from '@/components/layout/AppShell'
import { CampaignDetailPage } from '@/pages/CampaignDetailPage'
import { CampaignsPage } from '@/pages/CampaignsPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { JobsPage } from '@/pages/JobsPage'
import { MaterialsPage } from '@/pages/MaterialsPage'
import { PartsPage } from '@/pages/PartsPage'
import { PlansPage } from '@/pages/PlansPage'
import { PlanViewerPage } from '@/pages/PlanViewerPage'
import { ProductsPage } from '@/pages/ProductsPage'
import { RulesPage } from '@/pages/RulesPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { StockPage } from '@/pages/StockPage'

export default function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
        <Route path="viewer" element={<PlanViewerPage />} />
        <Route path="plans" element={<PlansPage />} />
        <Route path="materials" element={<MaterialsPage />} />
        <Route path="parts" element={<PartsPage />} />
        <Route path="products" element={<ProductsPage />} />
        <Route path="stock" element={<StockPage />} />
        <Route path="jobs" element={<JobsPage />} />
        <Route path="campaigns" element={<CampaignsPage />} />
        <Route path="campaigns/:id" element={<CampaignDetailPage />} />
        <Route path="rules" element={<RulesPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
