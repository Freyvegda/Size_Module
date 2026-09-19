import { Suspense, lazy } from 'react'
import { Download, Lock, Pencil, RefreshCw, RotateCw, Save, Trash2, Unlock, X } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import {
  editPlan,
  loadPlanDetail,
  reoptimizePlan,
} from '@/features/optimizer/optimizerSlice'
import { micronToMm, percent } from '@/lib/format'
import type { ExportFormat, SheetPlan } from '@/lib/types'
import { cn } from '@/lib/utils'
import { BarPlanView } from './BarPlanView'
import {
  beginEdit,
  deletePart,
  endEdit,
  movePart,
  placementKey,
  rotatePart,
  selectPart,
  selectSheet,
  setExplode,
  setMode,
  toggleLockPart,
  toggleOffcuts,
  type ViewMode,
} from './viewerSlice'

// three.js is heavy: keep it out of the initial bundle and load it on demand.
const PlanScene = lazy(() =>
  import('./PlanScene').then((module) => ({ default: module.PlanScene })),
)

function offcutLengthM(sheets: SheetPlan[]): number {
  const microns = sheets.reduce(
    (sum, sheet) => sum + (sheet.offcuts ?? []).reduce((acc, off) => acc + off.w, 0),
    0,
  )
  return microns / 1e6
}

export function PlanViewer() {
  const dispatch = useAppDispatch()
  const result = useAppSelector((state) => state.optimizer.result)
  const source = useAppSelector((state) => state.optimizer.source)
  const dimension = useAppSelector((state) => state.optimizer.dimension)
  const lastPlanId = useAppSelector((state) => state.optimizer.lastPlanId)
  const planEditStatus = useAppSelector((state) => state.optimizer.planEditStatus)
  const planEditError = useAppSelector((state) => state.optimizer.planEditError)
  const { mode, selectedSheet, selectedPartId, showOffcuts, explode } = useAppSelector(
    (state) => state.viewer,
  )
  const { editing, draftSheets, pendingOps } = useAppSelector((state) => state.viewer)

  if (!result) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>No plan loaded</CardTitle>
          <CardDescription>
            Run a demo optimization or load a demo plan first (2D sheets or 1D bars).
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  const isLengthPlan = dimension === '1d'
  const solution = result.solution
  const savedSheets = solution.sheets ?? []
  const sheets = editing && draftSheets ? draftSheets : savedSheets
  const unplaced = solution.unplaced ?? []
  const activeSheet = sheets.find((sheet) => sheet.index === selectedSheet) ?? sheets[0]
  const activePart = activeSheet?.placements.find(
    (placement) => placementKey(placement) === selectedPartId,
  )
  const metrics = solution.metrics
  const cutSteps = activeSheet?.cutSteps ?? []
  const notes = solution.notes ?? []

  const exportUrl = (format: ExportFormat) =>
    `/api/v1/plans/${lastPlanId}/exports?format=${format}`

  const startEditing = () => {
    if (!lastPlanId) return
    void dispatch(loadPlanDetail(lastPlanId))
      .unwrap()
      .then((detail) => dispatch(beginEdit(detail.result.solution.sheets ?? [])))
      .catch(() => {
        // loadPlanDetail already recorded the error in the optimizer slice.
      })
  }

  const saveEdits = () => {
    if (!lastPlanId) return
    void dispatch(
      editPlan({ planId: lastPlanId, operations: pendingOps, name: 'Edited plan' }),
    ).then((action) => {
      if (editPlan.fulfilled.match(action)) dispatch(endEdit())
    })
  }

  const resolvePlan = () => {
    if (!lastPlanId) return
    void dispatch(
      reoptimizePlan({
        planId: lastPlanId,
        operations: pendingOps.length > 0 ? pendingOps : undefined,
        name: 'Re-solved plan',
      }),
    ).then((action) => {
      if (reoptimizePlan.fulfilled.match(action)) dispatch(endEdit())
    })
  }

  return (
    <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_330px]">
      <div className="min-w-0 space-y-4">
        <Card className="overflow-hidden">
          <CardHeader className="gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <CardTitle>{isLengthPlan ? 'Bar plan' : 'Cut plan'}</CardTitle>
              <CardDescription>
                {solution.solver} v{solution.solverVersion} ·{' '}
                {source === 'api' ? 'from API' : 'sample data'}
              </CardDescription>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {!isLengthPlan && !editing && (
                <ToggleGroup
                  type="single"
                  value={mode}
                  onValueChange={(value) => value && dispatch(setMode(value as ViewMode))}
                  variant="outline"
                  size="sm"
                >
                  <ToggleGroupItem value="2d">2D</ToggleGroupItem>
                  <ToggleGroupItem value="3d">3D</ToggleGroupItem>
                </ToggleGroup>
              )}
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                Offcuts
                <Switch checked={showOffcuts} onCheckedChange={() => dispatch(toggleOffcuts())} />
              </label>
              {!isLengthPlan && !editing && mode === '3d' && (
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  Explode
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.05}
                    value={explode}
                    onChange={(event) => dispatch(setExplode(Number(event.target.value)))}
                    className="w-24 accent-sky-500"
                  />
                </label>
              )}
            </div>
          </CardHeader>

          {lastPlanId && (
            <div className="flex flex-wrap items-center gap-2 border-t border-border px-6 py-3">
              {!editing ? (
                <>
                  {!isLengthPlan && (
                    <Button variant="outline" size="sm" onClick={startEditing}>
                      <Pencil className="mr-2 h-3.5 w-3.5" />
                      Edit layout
                    </Button>
                  )}
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Download className="h-3.5 w-3.5" />
                    Export
                  </span>
                  {(['csv', 'svg', 'dxf', 'pdf'] as ExportFormat[]).map((format) => (
                    <Button key={format} variant="outline" size="sm" asChild>
                      <a href={exportUrl(format)} download>
                        {format.toUpperCase()}
                      </a>
                    </Button>
                  ))}
                </>
              ) : (
                <>
                  <Badge variant="secondary">
                    editing · {pendingOps.length} change{pendingOps.length === 1 ? '' : 's'}
                  </Badge>
                  <Button size="sm" onClick={saveEdits} disabled={planEditStatus === 'loading' || pendingOps.length === 0}>
                    <Save className="mr-2 h-3.5 w-3.5" />
                    {planEditStatus === 'loading' ? 'Working…' : 'Save edits'}
                  </Button>
                  <Button variant="outline" size="sm" onClick={resolvePlan} disabled={planEditStatus === 'loading'}>
                    <RefreshCw className="mr-2 h-3.5 w-3.5" />
                    Re-solve with locks
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => dispatch(endEdit())} disabled={planEditStatus === 'loading'}>
                    <X className="mr-2 h-3.5 w-3.5" />
                    Cancel
                  </Button>
                  <span className="text-xs text-muted-foreground">
                    Drag a piece to move it; lock pieces to keep them on re-solve.
                  </span>
                </>
              )}
            </div>
          )}
          {planEditError && (
            <div className="border-t border-destructive/40 bg-destructive/5 px-6 py-2 text-xs text-destructive">
              {planEditError}
            </div>
          )}

          <CardContent className="px-0 pb-0">
            {isLengthPlan ? (
              <div className="border-t border-border">
                <BarPlanView
                  sheets={sheets}
                  selectedSheet={activeSheet?.index ?? 0}
                  selectedPartId={selectedPartId}
                  showOffcuts={showOffcuts}
                  onSelectSheet={(index) => {
                    dispatch(selectSheet(index))
                    dispatch(selectPart(undefined))
                  }}
                  onSelectPart={(partId) => dispatch(selectPart(partId))}
                />
              </div>
            ) : (
              <div className="h-[520px] w-full border-t border-border">
                <Suspense
                  fallback={
                    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                      Loading the 3D engine…
                    </div>
                  }
                >
                  <PlanScene
                    sheets={sheets}
                    mode={editing ? '2d' : mode}
                    selectedSheet={activeSheet?.index ?? 0}
                    selectedPartId={selectedPartId}
                    showOffcuts={showOffcuts}
                    explode={explode}
                    onSelectPart={(partId) => dispatch(selectPart(partId))}
                    editMode={editing}
                    onMovePart={(placementId, x, y) =>
                      dispatch(movePart({ placementId, x, y }))
                    }
                  />
                </Suspense>
              </div>
            )}
          </CardContent>
        </Card>

        {(cutSteps.length > 0 || notes.length > 0) && (
          <div className="grid gap-4 md:grid-cols-2">
            {cutSteps.length > 0 && (
              <Card
                key={`cut-sequence-${activeSheet?.index ?? 0}`}
                className="animate-in fade-in slide-in-from-bottom-2 duration-500"
              >
                <CardHeader>
                  <CardTitle>Cut sequence</CardTitle>
                  <CardDescription>
                    {activeSheet?.label || activeSheet?.stockCode} · {cutSteps.length} steps
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ScrollArea className="h-40 pr-2">
                    <ol className="list-decimal space-y-1 pl-4 text-xs text-muted-foreground">
                      {cutSteps.map((step, index) => (
                        <li
                          key={index}
                          className="animate-in fade-in slide-in-from-bottom-1 fill-mode-backwards"
                          style={{ animationDelay: `${Math.min(index, 12) * 35}ms` }}
                        >
                          {step}
                        </li>
                      ))}
                    </ol>
                  </ScrollArea>
                </CardContent>
              </Card>
            )}

            {notes.length > 0 && (
              <Card
                key={`why-this-plan-${activeSheet?.index ?? 0}`}
                className="animate-in fade-in slide-in-from-bottom-2 duration-500"
              >
                <CardHeader>
                  <CardTitle>Why this plan</CardTitle>
                  <CardDescription>How the solver reasoned about this layout.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-2 text-xs text-muted-foreground">
                  {notes.map((note, index) => (
                    <p
                      key={index}
                      className="animate-in fade-in slide-in-from-bottom-1 fill-mode-backwards"
                      style={{ animationDelay: `${Math.min(index, 12) * 35}ms` }}
                    >
                      {note}
                    </p>
                  ))}
                </CardContent>
              </Card>
            )}
          </div>
        )}
      </div>

      <div className="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle>Scorecard</CardTitle>
            <CardDescription>
              {metrics.sheetCount} {isLengthPlan ? 'bar(s)' : 'sheet(s)'} ·{' '}
              {metrics.partsPlaced}/{metrics.partsRequested} pieces
            </CardDescription>
          </CardHeader>
          <CardContent className="grid grid-cols-2 gap-3 text-sm">
            <Metric label="Yield" value={percent(metrics.yieldPct)} tone="good" />
            <Metric label="Waste" value={percent(metrics.wastePct)} tone="bad" />
            {isLengthPlan ? (
              <>
                <Metric label="Stock length" value={`${(metrics.stockLengthM ?? 0).toFixed(2)} m`} />
                <Metric label="Used length" value={`${(metrics.usedLengthM ?? 0).toFixed(2)} m`} />
                <Metric label="Offcut length" value={`${offcutLengthM(sheets).toFixed(2)} m`} />
              </>
            ) : (
              <>
                <Metric label="Offcut area" value={`${metrics.offcutAreaM2.toFixed(2)} m²`} />
                <Metric label="Scrap area" value={`${metrics.scrapAreaM2.toFixed(2)} m²`} />
                <Metric label="Kerf ≈" value={`${metrics.kerfAreaM2.toFixed(2)} m²`} />
                <Metric label="Trim" value={`${metrics.trimAreaM2.toFixed(2)} m²`} />
              </>
            )}
            <Metric label="Patterns" value={String(metrics.patternCount)} />
            {(metrics.remnantSheets ?? 0) > 0 && (
              <Metric label="From remnants" value={String(metrics.remnantSheets)} tone="good" />
            )}
            <Metric label="Cost" value={metrics.cost.toFixed(2)} />
            <Metric label="Solve time" value={`${metrics.elapsedMs} ms`} />
            <Metric label="Score" value={result.score.toFixed(2)} />
            {result.cost && (
              <div className="col-span-2 space-y-1 border-t border-border pt-3">
                <div className="text-xs font-medium text-muted-foreground">Cost breakdown</div>
                <Row label="New material" value={result.cost.newMaterialCost.toFixed(2)} />
                <Row label="Remnants taken" value={result.cost.remnantCost.toFixed(2)} />
                <Row label="Offcut credit" value={`−${result.cost.offcutCredit.toFixed(2)}`} />
                <Row label="Net cost" value={result.cost.netCost.toFixed(2)} />
                <Row label="Cost / part" value={result.cost.costPerPart.toFixed(2)} />
                <Row label="Cost / m²" value={result.cost.costPerM2.toFixed(2)} />
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{isLengthPlan ? 'Bars' : 'Sheets'}</CardTitle>
            <CardDescription>
              {isLengthPlan ? 'Click a bar to highlight it.' : 'Click a sheet to inspect it in 2D.'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {sheets.map((sheet) => {
              const active = sheet.index === activeSheet?.index
              return (
                <button
                  key={sheet.index}
                  type="button"
                  onClick={() => {
                    dispatch(selectSheet(sheet.index))
                    dispatch(selectPart(undefined))
                  }}
                  className={cn(
                    'flex w-full items-center justify-between rounded-md border px-3 py-2 text-left text-sm transition-all duration-300 ease-out',
                    active
                      ? 'border-sky-500/60 bg-sky-500/10 shadow-[0_0_0_1px_rgba(56,189,248,0.35)]'
                      : 'border-border hover:-translate-y-px hover:border-sky-500/30 hover:bg-muted',
                  )}
                >
                  <span className="flex items-center gap-2 font-medium">
                    <span
                      className={cn(
                        'h-1.5 w-1.5 rounded-full transition-colors duration-300',
                        active ? 'bg-sky-400' : 'bg-muted-foreground/30',
                      )}
                    />
                    {sheet.label || sheet.stockCode}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {(sheet.placements ?? []).length} pieces ·{' '}
                    {isLengthPlan
                      ? `${micronToMm(sheet.width)} mm`
                      : `${micronToMm(sheet.width)}×${micronToMm(sheet.height)} mm`}
                  </span>
                </button>
              )
            })}
          </CardContent>
        </Card>

        {activePart && (
          <Card className="animate-in fade-in slide-in-from-bottom-1 duration-300">
            <CardHeader>
              <CardTitle>Selected piece</CardTitle>
              <CardDescription>
                {activePart.partCode}
                {activePart.locked ? ' · locked' : ''}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-1 text-sm">
              {isLengthPlan ? (
                <Row label="Length" value={`${micronToMm(activePart.w)} mm`} />
              ) : (
                <Row
                  label="Size"
                  value={`${micronToMm(activePart.w)} × ${micronToMm(activePart.h)} mm`}
                />
              )}
              <Row
                label="Position"
                value={`x ${micronToMm(activePart.x)}${isLengthPlan ? '' : ` / y ${micronToMm(activePart.y)}`} mm`}
              />
              {!isLengthPlan && <Row label="Rotated" value={activePart.rotated ? 'yes' : 'no'} />}
              {editing && !isLengthPlan && activePart.id && (
                <div className="flex flex-wrap gap-2 pt-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => dispatch(rotatePart({ placementId: activePart.id! }))}
                  >
                    <RotateCw className="mr-1.5 h-3.5 w-3.5" />
                    Rotate
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => dispatch(toggleLockPart({ placementId: activePart.id! }))}
                  >
                    {activePart.locked ? (
                      <Unlock className="mr-1.5 h-3.5 w-3.5" />
                    ) : (
                      <Lock className="mr-1.5 h-3.5 w-3.5" />
                    )}
                    {activePart.locked ? 'Unlock' : 'Lock'}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => dispatch(deletePart({ placementId: activePart.id! }))}
                  >
                    <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                    Delete
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {unplaced.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Unplaced demand</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              {unplaced.map((item) => (
                <div key={item.partId} className="flex items-start justify-between gap-2">
                  <div>
                    <div className="font-medium">
                      {item.quantity} × {item.partCode}
                    </div>
                    <div className="text-xs text-muted-foreground">{item.reason}</div>
                  </div>
                  <Badge variant="destructive">unplaced</Badge>
                </div>
              ))}
            </CardContent>
          </Card>
        )}

        {(result.violations?.length ?? 0) > 0 && (
          <Card>
            <CardHeader>
              <CardTitle>Validation</CardTitle>
              <CardDescription>Invariants checked before a plan is shown.</CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 text-xs">
              <Separator />
              {result.violations?.map((violation, index) => (
                <p key={index} className="text-muted-foreground">
                  <Badge variant={violation.severity === 'error' ? 'destructive' : 'secondary'}>
                    {violation.severity}
                  </Badge>{' '}
                  {violation.message}
                </p>
              ))}
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  )
}

function Metric({ label, value, tone }: { label: string; value: string; tone?: 'good' | 'bad' }) {
  const toneClass =
    tone === 'good' ? 'text-emerald-500' : tone === 'bad' ? 'text-amber-500' : 'text-foreground'
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={`text-lg font-semibold ${toneClass}`}>{value}</div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span>{value}</span>
    </div>
  )
}
