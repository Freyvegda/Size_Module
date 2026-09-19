import { useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Play, RefreshCw, Ruler, Upload } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { fetchBackend } from '@/features/backend/backendSlice'
import { fetchKpis } from '@/features/kpis/kpiSlice'
import {
  loadBarDemoPlan,
  loadDemoPlan,
  runDemoOptimization,
} from '@/features/optimizer/optimizerSlice'
import { SolverBadges } from '@/features/optimizer/SolverBadges'
import { percent } from '@/lib/format'
import type { KpiSeriesPoint } from '@/lib/types'

export function DashboardPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const backend = useAppSelector((state) => state.backend)
  const optimizer = useAppSelector((state) => state.optimizer)
  const kpis = useAppSelector((state) => state.kpis)
  const databaseUp = backend.health?.db === 'up'

  useEffect(() => {
    void dispatch(fetchBackend())
  }, [dispatch])

  useEffect(() => {
    if (databaseUp && kpis.status === 'idle') {
      void dispatch(fetchKpis(kpis.days))
    }
  }, [dispatch, databaseUp, kpis.status, kpis.days])

  const refresh = () => {
    void dispatch(fetchBackend())
    if (databaseUp) {
      void dispatch(fetchKpis(kpis.days))
    }
  }

  const runDemo = async () => {
    const action = await dispatch(runDemoOptimization({ profile: '2d' }))
    if (runDemoOptimization.fulfilled.match(action)) {
      navigate('/viewer')
    }
  }

  const loadPlan = async () => {
    await dispatch(loadDemoPlan())
    navigate('/viewer')
  }

  const runBarDemo = async () => {
    const action = await dispatch(runDemoOptimization({ profile: '1d' }))
    if (runDemoOptimization.fulfilled.match(action)) {
      navigate('/viewer')
    }
  }

  const loadBarPlan = async () => {
    await dispatch(loadBarDemoPlan())
    navigate('/viewer')
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Dashboard</h1>
          <p className="text-sm text-muted-foreground">
            Cut planning and material yield for any sheet, panel or bar stock.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={refresh}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
          <Button variant="outline" onClick={loadPlan}>
            <Upload className="mr-2 h-4 w-4" />
            Load demo plan
          </Button>
          <Button onClick={runDemo} disabled={optimizer.runStatus === 'loading'}>
            <Play className="mr-2 h-4 w-4" />
            {optimizer.runStatus === 'loading' ? 'Optimizing…' : 'Run demo optimization'}
          </Button>
        </div>
      </div>

      {optimizer.runError && (
        <Card className="border-destructive/40">
          <CardHeader>
            <CardTitle className="text-destructive">Optimization failed</CardTitle>
            <CardDescription>{optimizer.runError}</CardDescription>
          </CardHeader>
        </Card>
      )}

      <Card>
        <CardHeader className="gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <CardTitle>Realized yield</CardTitle>
            <CardDescription>
              Accepted plans — what the shop committed to — and everything planned in the window.
            </CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Select
              value={String(kpis.days)}
              onValueChange={(value) => dispatch(fetchKpis(Number(value)))}
            >
              <SelectTrigger className="w-[130px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="30">Last 30 days</SelectItem>
                <SelectItem value="90">Last 90 days</SelectItem>
                <SelectItem value="365">Last 365 days</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {!databaseUp ? (
            <p className="text-sm text-muted-foreground">
              KPIs are aggregated from accepted plans and need PostgreSQL. Start it with{' '}
              <code>db/scripts/up.ps1</code>.
            </p>
          ) : kpis.error ? (
            <p className="text-sm text-destructive">{kpis.error}</p>
          ) : !kpis.report ? (
            <p className="text-sm text-muted-foreground">
              {kpis.status === 'loading' ? 'Loading KPIs…' : 'No KPI data yet.'}
            </p>
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                <Kpi
                  label="Realized yield"
                  value={percent(kpis.report.realized.yieldPct)}
                  hint={`${kpis.report.realized.plans} accepted plan(s)`}
                  tone="good"
                />
                <Kpi
                  label="Realized waste"
                  value={percent(kpis.report.realized.wastePct)}
                  hint={`${kpis.report.realized.scrapAreaM2.toFixed(1)} m² scrap`}
                  tone={kpis.report.realized.wastePct > 30 ? 'bad' : undefined}
                />
                <Kpi
                  label="Stock value"
                  value={kpis.report.realized.cost.toFixed(2)}
                  hint={
                    kpis.report.realized.partsPlaced > 0
                      ? `${kpis.report.realized.costPerPart.toFixed(2)} / part`
                      : 'no parts placed'
                  }
                />
                <Kpi
                  label="Remnants used"
                  value={String(kpis.report.realized.remnantSheets)}
                  hint={`of ${kpis.report.realized.sheets} sheets`}
                />
                <Kpi
                  label="Parts produced"
                  value={String(kpis.report.realized.partsPlaced)}
                  hint={`${kpis.report.realized.partAreaM2.toFixed(1)} m² of parts`}
                />
                <Kpi
                  label="Offcuts kept"
                  value={`${kpis.report.realized.offcutAreaM2.toFixed(1)} m²`}
                  hint="back in circulation"
                />
                <Kpi
                  label="Plans created"
                  value={String(kpis.report.created.plans)}
                  hint={`pipeline yield ${percent(kpis.report.created.yieldPct)}`}
                />
                <Kpi
                  label="Pipeline value"
                  value={kpis.report.created.cost.toFixed(2)}
                  hint={`${kpis.report.created.sheets} sheet(s) planned`}
                />
              </div>

              {(kpis.report.series ?? []).length > 0 && (
                <div>
                  <div className="mb-1 text-xs text-muted-foreground">
                    Accepted plans, oldest to newest — bar height is yield %, darker means more
                    waste
                  </div>
                  <div className="flex h-24 items-end gap-1">
                    {(kpis.report.series ?? []).map((point) => (
                      <YieldBar key={point.planId} point={point} />
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>System</CardTitle>
            <CardDescription>API and database status</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Line label="API" value={backend.status === 'error' ? 'offline' : (backend.health?.status ?? '—')} />
            <Line label="Database" value={backend.health?.db ?? '—'} />
            <Line label="Env" value={backend.health?.env ?? '—'} />
            <Line label="Version" value={backend.health?.version ?? '—'} />
            {!backend.health && (
              <p className="pt-2 text-xs text-muted-foreground">
                Start the API with <code>cd backend; go run ./cmd/cutoptics</code>
              </p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Solvers</CardTitle>
            <CardDescription>Plug-and-play algorithms behind one interface</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {(backend.meta?.solvers ?? []).map((solver) => (
              <div key={solver.name} className="rounded-md border border-border p-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{solver.name}</span>
                  <span className="text-xs text-muted-foreground">v{solver.version}</span>
                </div>
                <SolverBadges capabilities={solver.capabilities} className="mt-2" />
                <p className="mt-2 text-xs text-muted-foreground">{solver.capabilities.Description}</p>
              </div>
            ))}
            {(backend.meta?.solvers ?? []).length === 0 && (
              <p className="text-sm text-muted-foreground">No solver information yet.</p>
            )}
            <Separator />
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                <Ruler className="h-3.5 w-3.5" />
                1D bar demo (aluminium profiles)
              </span>
              <div className="flex gap-2">
                <Button size="xs" variant="outline" onClick={loadBarPlan}>
                  Load plan
                </Button>
                <Button size="xs" onClick={runBarDemo} disabled={optimizer.runStatus === 'loading'}>
                  Run
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Latest result</CardTitle>
            <CardDescription>
              {optimizer.lastJobId ? `Archived as job ${optimizer.lastJobId.slice(0, 8)}…` : 'No run yet'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {optimizer.result ? (
              <>
                <Line label="Source" value={optimizer.source === 'api' ? 'Go API' : 'sample data'} />
                <div className="flex items-center justify-between gap-4">
                  <span className="text-muted-foreground">Dimension</span>
                  <Badge variant="secondary">{optimizer.dimension}</Badge>
                </div>
                <Line
                  label={optimizer.dimension === '1d' ? 'Bars' : 'Sheets'}
                  value={String(optimizer.result.solution.metrics.sheetCount)}
                />
                <Line
                  label="Pieces"
                  value={`${optimizer.result.solution.metrics.partsPlaced}/${optimizer.result.solution.metrics.partsRequested}`}
                />
                {optimizer.dimension === '1d' ? (
                  <Line
                    label="Used length"
                    value={`${(optimizer.result.solution.metrics.usedLengthM ?? 0).toFixed(2)} m`}
                  />
                ) : (
                  <Line
                    label="Yield"
                    value={percent(optimizer.result.solution.metrics.yieldPct)}
                  />
                )}
                <Button variant="link" className="h-auto p-0" asChild>
                  <Link to="/viewer">Open in viewer →</Link>
                </Button>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">
                Run the demo optimization to produce a plan, validate it and archive it.
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function Line({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  )
}

function Kpi({
  label,
  value,
  hint,
  tone,
}: {
  label: string
  value: string
  hint?: string
  tone?: 'good' | 'bad'
}) {
  const toneClass =
    tone === 'good' ? 'text-emerald-500' : tone === 'bad' ? 'text-amber-500' : 'text-foreground'
  return (
    <div className="rounded-md border border-border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={`mt-1 text-2xl font-semibold ${toneClass}`}>{value}</div>
      {hint && <div className="mt-1 text-xs text-muted-foreground">{hint}</div>}
    </div>
  )
}

/** One accepted plan on the yield trend line. */
function YieldBar({ point }: { point: KpiSeriesPoint }) {
  const height = Math.max(4, Math.min(100, point.yieldPct))
  return (
    <div
      className="min-w-[6px] flex-1 rounded-t-sm bg-sky-500"
      style={{
        height: `${height}%`,
        opacity: 0.45 + Math.min(0.55, point.wastePct / 100),
      }}
      title={`${new Date(point.createdAt).toLocaleDateString()} · yield ${point.yieldPct.toFixed(1)}% · waste ${point.wastePct.toFixed(1)}% · ${point.sheets} sheet(s) · ${point.partsPlaced} part(s)`}
    />
  )
}
