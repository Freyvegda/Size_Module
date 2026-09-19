import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ListPlus, Play, Scale } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import {
  cancelAsyncJob,
  compareSolvers,
  runDemoOptimization,
  startAsyncJob,
} from '@/features/optimizer/optimizerSlice'
import { SolverBadges } from '@/features/optimizer/SolverBadges'
import { fetchRulesProfiles } from '@/features/rules/rulesSlice'
import { percent } from '@/lib/format'
import { rulesOverrideFor } from '@/lib/rules'
import { solversFor } from '@/lib/solvers'
import type { CutMode, DimensionProfile } from '@/lib/types'

export function JobsPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const meta = useAppSelector((state) => state.backend.meta)
  const solvers = useMemo(() => meta?.solvers ?? [], [meta])
  const databaseUp = useAppSelector((state) => state.backend.health?.db === 'up')
  const {
    runStatus,
    runError,
    lastJobId,
    result,
    dimension,
    comparison,
    comparisonProfile,
    comparisonStatus,
    comparisonError,
    jobId,
    jobState,
    jobProgress,
    jobError,
    jobRequest,
  } = useAppSelector((state) => state.optimizer)
  const [profile, setProfile] = useState<DimensionProfile>('2d')
  const [cutMode, setCutMode] = useState<CutMode>('guillotine')
  const [solverChoice, setSolverChoice] = useState('auto')
  const [includeRemnants, setIncludeRemnants] = useState(true)
  const [rulesProfileChoice, setRulesProfileChoice] = useState('default')
  const rulesProfiles = useAppSelector((state) => state.rules.profiles)

  useEffect(() => {
    void dispatch(fetchRulesProfiles())
  }, [dispatch])

  // Bars are always guillotine-cut; free cutting only exists for 2D machines.
  const effectiveCutMode: CutMode = profile === '1d' ? 'guillotine' : cutMode
  const selectedRulesProfile = rulesProfiles.find((item) => item.id === rulesProfileChoice)
  // The server resolves a named profile; only an actual override (free cutting,
  // or a profile whose cut mode differs) needs the complete rules on the body.
  const rulesOverride = rulesOverrideFor(selectedRulesProfile?.rules, effectiveCutMode)
  const compatible = useMemo(
    () => solversFor(solvers, profile, effectiveCutMode),
    [solvers, profile, effectiveCutMode],
  )
  // A solver chosen under another profile/cut mode must not stay selected. The
  // fallback is derived, not written back to state, so switching back restores
  // the explicit choice.
  const solver =
    solverChoice === 'auto' || compatible.some((item) => item.name === solverChoice)
      ? solverChoice
      : 'auto'

  const selected = solver === 'auto' ? undefined : compatible.find((item) => item.name === solver)
  const autoPick = compatible[0]

  const run = () => {
    void dispatch(
      runDemoOptimization({
        profile,
        cutMode: effectiveCutMode,
        solver: selected?.name,
        includeRemnants,
        rulesProfileId: selectedRulesProfile?.id,
        rulesOverride,
      }),
    )
  }

  const compare = () => {
    void dispatch(
      compareSolvers({
        profile,
        cutMode: effectiveCutMode,
        solvers: compatible.map((item) => item.name),
        includeRemnants,
        rulesProfileId: selectedRulesProfile?.id,
        rulesOverride,
      }),
    )
  }

  const openResult = () => {
    navigate('/viewer')
  }

  const queueJob = () => {
    void dispatch(
      startAsyncJob({
        profile,
        cutMode: effectiveCutMode,
        solver: selected?.name,
        includeRemnants,
        rulesProfileId: selectedRulesProfile?.id,
        rulesOverride,
      }),
    )
  }

  const bestComparisonScore = comparison?.length
    ? Math.max(...comparison.map((entry) => entry.result.score))
    : undefined
  const comparisonIs1D = comparisonProfile === '1d'

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-semibold">Jobs</h1>
        <p className="text-sm text-muted-foreground">
          Every run is archived: the problem snapshot, the plan, its sheets and placements. Results
          stay reproducible even when master data changes.
        </p>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Run an optimization</CardTitle>
            <CardDescription>
              The demo problem is wired in — 2D glass sheets or 1D aluminium bars. Only solvers the
              API accepts for the selected profile and cutting mode are offered.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <Select value={profile} onValueChange={(value) => setProfile(value as DimensionProfile)}>
                <SelectTrigger className="w-[170px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="2d">2D sheets &amp; panels</SelectItem>
                  <SelectItem value="1d">1D bars &amp; profiles</SelectItem>
                </SelectContent>
              </Select>
              {profile === '2d' && (
                <Select value={cutMode} onValueChange={(value) => setCutMode(value as CutMode)}>
                  <SelectTrigger className="w-[200px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="guillotine">Guillotine (saw)</SelectItem>
                    <SelectItem value="free">Free cutting (CNC/laser)</SelectItem>
                  </SelectContent>
                </Select>
              )}
              <Select value={rulesProfileChoice} onValueChange={setRulesProfileChoice}>
                <SelectTrigger className="w-[220px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="default">Built-in rules</SelectItem>
                  {rulesProfiles.map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.name || item.code}
                      {item.isDefault ? ' (default)' : ''}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={solver} onValueChange={setSolverChoice}>
                <SelectTrigger className="w-[230px]">
                  <SelectValue placeholder="Auto" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">Auto (best available)</SelectItem>
                  {compatible.map((item) => (
                    <SelectItem key={item.name} value={item.name}>
                      {item.name} · {item.capabilities.CutMode}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button onClick={run} disabled={runStatus === 'loading' || compatible.length === 0}>
                <Play className="mr-2 h-4 w-4" />
                {runStatus === 'loading' ? 'Solving…' : 'Run'}
              </Button>
              <Button
                variant="outline"
                onClick={compare}
                disabled={comparisonStatus === 'loading' || compatible.length < 2}
              >
                <Scale className="mr-2 h-4 w-4" />
                {comparisonStatus === 'loading' ? 'Comparing…' : 'Compare solvers'}
              </Button>
            </div>

            {selected ? (
              <div className="space-y-1.5 rounded-md border border-border p-3">
                <SolverBadges capabilities={selected.capabilities} />
                <p className="text-xs text-muted-foreground">{selected.capabilities.Description}</p>
              </div>
            ) : (
              autoPick && (
                <p className="text-xs text-muted-foreground">
                  Auto picks <span className="font-medium text-foreground">{autoPick.name}</span> —
                  the top-ranked solver for {profile} {effectiveCutMode} problems:{' '}
                  {autoPick.capabilities.Description}
                </p>
              )
            )}

            <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
              <div>
                <div className="text-sm font-medium">Use available remnants</div>
                <p className="text-xs text-muted-foreground">
                  The server adds the plant's labelled leftovers to the stock list before solving,
                  so paid-for offcuts are used before new sheets
                  {!databaseUp && ' (needs the database)'}.
                </p>
              </div>
              <Switch
                checked={includeRemnants}
                onCheckedChange={setIncludeRemnants}
                disabled={!databaseUp}
              />
            </div>

            {runError && (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm">
                <div className="font-medium text-destructive">Run failed</div>
                <p className="mt-1 text-muted-foreground">{runError}</p>
                <p className="mt-2 text-xs text-muted-foreground">
                  The optimizer itself works without a database; archiving and listings need
                  PostgreSQL.
                </p>
              </div>
            )}
            {lastJobId && (
              <div className="rounded-md border border-border p-3 text-sm">
                <div className="flex items-center gap-2">
                  <Badge className="bg-emerald-600 text-white">archived</Badge>
                  <code className="text-xs">{lastJobId}</code>
                </div>
                <p className="mt-2 text-xs text-muted-foreground">
                  Check it with <code>db/scripts/psql.ps1</code>:
                  <br />
                  <code>select id, status, solver from cut_jobs order by created_at desc limit 3;</code>
                </p>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Latest result</CardTitle>
            <CardDescription>What the solver produced</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {result ? (
              <>
                <Line label="Solver" value={result.solution.solver} />
                <Line
                  label={dimension === '1d' ? 'Bars' : 'Sheets'}
                  value={String(result.solution.metrics.sheetCount)}
                />
                <Line
                  label="Pieces placed"
                  value={`${result.solution.metrics.partsPlaced}/${result.solution.metrics.partsRequested}`}
                />
                {dimension === '1d' ? (
                  <>
                    <Line
                      label="Stock length"
                      value={`${(result.solution.metrics.stockLengthM ?? 0).toFixed(2)} m`}
                    />
                    <Line
                      label="Used length"
                      value={`${(result.solution.metrics.usedLengthM ?? 0).toFixed(2)} m`}
                    />
                  </>
                ) : (
                  <>
                    <Line label="Yield" value={percent(result.solution.metrics.yieldPct)} />
                    <Line label="Waste" value={percent(result.solution.metrics.wastePct)} />
                    {(result.solution.metrics.remnantSheets ?? 0) > 0 && (
                      <Line
                        label="From remnants"
                        value={String(result.solution.metrics.remnantSheets)}
                      />
                    )}
                    <Line
                      label="Offcuts kept"
                      value={`${result.solution.metrics.offcutAreaM2.toFixed(2)} m²`}
                    />
                  </>
                )}
                <Line label="Violations" value={String(result.violations?.length ?? 0)} />
                <Button variant="link" className="h-auto p-0" asChild>
                  <Link to="/viewer">Inspect in the plan viewer →</Link>
                </Button>
              </>
            ) : (
              <p className="text-muted-foreground">No result yet.</p>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Queue a job (live progress)</CardTitle>
          <CardDescription>
            The problem snapshot goes into the Postgres queue and a worker claims it with{' '}
            <code>FOR UPDATE SKIP LOCKED</code>. Progress streams over Server-Sent Events, so long
            solves never block a request.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <Button
              onClick={queueJob}
              disabled={
                !databaseUp ||
                (jobRequest !== 'idle' && jobRequest !== 'error') ||
                compatible.length === 0
              }
            >
              <ListPlus className="mr-2 h-4 w-4" />
              {jobRequest === 'starting'
                ? 'Queueing…'
                : jobRequest === 'watching'
                  ? 'Running…'
                  : 'Queue job'}
            </Button>
            {(jobState === 'queued' || jobState === 'running') && jobId && (
              <Button variant="outline" onClick={() => void dispatch(cancelAsyncJob(jobId))}>
                Cancel
              </Button>
            )}
            {jobId && (
              <span className="flex items-center gap-2">
                <Badge
                  variant={
                    jobState === 'failed'
                      ? 'destructive'
                      : jobState === 'done'
                        ? 'default'
                        : 'secondary'
                  }
                  className={jobState === 'done' ? 'bg-emerald-600 text-white' : undefined}
                >
                  {jobState}
                </Badge>
                <code className="text-xs text-muted-foreground">{jobId.slice(0, 8)}…</code>
              </span>
            )}
          </div>

          {jobProgress && (
            <div className="space-y-1.5">
              <Progress
                value={
                  jobProgress.requested > 0
                    ? Math.round((100 * jobProgress.placed) / jobProgress.requested)
                    : 0
                }
              />
              <div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground">
                <span>
                  {jobProgress.sheets} {profile === '1d' ? 'bar(s)' : 'sheet(s)'} ·{' '}
                  {jobProgress.placed}/{jobProgress.requested} pieces
                </span>
                <span>
                  yield {percent(jobProgress.yieldPct)} · waste {percent(jobProgress.wastePct)} ·{' '}
                  {jobProgress.elapsedMs} ms
                </span>
              </div>
            </div>
          )}

          {jobState === 'done' && (
            <Button variant="outline" size="sm" onClick={openResult}>
              Open the archived plan in the viewer
            </Button>
          )}
          {jobError && (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <div className="font-medium text-destructive">Job problem</div>
              <p className="mt-1 text-muted-foreground">{jobError}</p>
            </div>
          )}
          {!databaseUp && (
            <p className="text-xs text-muted-foreground">
              The queue needs PostgreSQL. Start it with <code>db/scripts/up.ps1</code> and restart
              the API; the synchronous Run above works without it.
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Solver comparison</CardTitle>
          <CardDescription>
            Same problem, every compatible strategy. Higher score is better; the score follows the
            objective (fill priority demand first). These runs are not archived.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {comparisonError && (
            <p className="text-sm text-destructive">Comparison failed: {comparisonError}</p>
          )}
          {comparison && comparison.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Solver</TableHead>
                  <TableHead className="text-right">{comparisonIs1D ? 'Bars' : 'Sheets'}</TableHead>
                  <TableHead className="text-right">Pieces</TableHead>
                  <TableHead className="text-right">Fill</TableHead>
                  <TableHead className="text-right">Yield</TableHead>
                  <TableHead className="text-right">Waste</TableHead>
                  {comparisonIs1D && <TableHead className="text-right">Length used</TableHead>}
                  <TableHead className="text-right">Score</TableHead>
                  <TableHead className="text-right">Time</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {comparison.map((entry) => {
                  const m = entry.result.solution.metrics
                  const fill = m.partsRequested > 0 ? (100 * m.partsPlaced) / m.partsRequested : 0
                  const best =
                    bestComparisonScore !== undefined && entry.result.score >= bestComparisonScore
                  return (
                    <TableRow key={entry.solver}>
                      <TableCell className="font-medium">
                        <span className="flex items-center gap-2">
                          {entry.solver}
                          {best && <Badge className="bg-emerald-600 text-white">best</Badge>}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">{m.sheetCount}</TableCell>
                      <TableCell className="text-right">
                        {m.partsPlaced}/{m.partsRequested}
                      </TableCell>
                      <TableCell className="text-right">{percent(fill)}</TableCell>
                      <TableCell className="text-right">{percent(m.yieldPct)}</TableCell>
                      <TableCell className="text-right">{percent(m.wastePct)}</TableCell>
                      {comparisonIs1D && (
                        <TableCell className="text-right">
                          {(m.usedLengthM ?? 0).toFixed(2)}/{(m.stockLengthM ?? 0).toFixed(2)} m
                        </TableCell>
                      )}
                      <TableCell className="text-right">{entry.result.score.toFixed(1)}</TableCell>
                      <TableCell className="text-right">{m.elapsedMs} ms</TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          ) : (
            <p className="text-sm text-muted-foreground">
              Run the comparison to see how the shelf baseline, beam search, column generation and
              MaxRects free-cut packer stack up on the same problem.
            </p>
          )}
        </CardContent>
      </Card>
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
