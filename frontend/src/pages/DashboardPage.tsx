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
import { Separator } from '@/components/ui/separator'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { fetchBackend } from '@/features/backend/backendSlice'
import {
  loadBarDemoPlan,
  loadDemoPlan,
  runDemoOptimization,
} from '@/features/optimizer/optimizerSlice'
import { SolverBadges } from '@/features/optimizer/SolverBadges'
import { percent } from '@/lib/format'

export function DashboardPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const backend = useAppSelector((state) => state.backend)
  const optimizer = useAppSelector((state) => state.optimizer)

  useEffect(() => {
    void dispatch(fetchBackend())
  }, [dispatch])

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
          <Button variant="outline" onClick={() => dispatch(fetchBackend())}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh status
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
