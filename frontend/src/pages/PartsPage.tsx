import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Play, Plus, RefreshCw } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
  createPart,
  fetchMaterials,
  fetchMaterialSpecs,
  fetchParts,
  fetchStockFormats,
} from '@/features/catalog/catalogSlice'
import { buildCatalogOrders, toProblemPart, type OrderLine } from '@/features/optimizer/buildProblem'
import { optimizeProblem } from '@/features/optimizer/optimizerSlice'
import { micronToMm, mmToMicron } from '@/lib/format'
import type { CreatePartInput, DimensionProfile, GrainMode, MaterialSpec, Part } from '@/lib/types'

type PartKind = '1d' | '2d'

function profileOf(part: Part, specs: MaterialSpec[]): DimensionProfile {
  const spec = specs.find((item) => item.id === part.materialSpecId)
  if (spec) return spec.dimensionProfile
  return part.cutLengthMicron > 0 ? '1d' : '2d'
}

function sizeLabel(part: Part, cut: boolean): string {
  const l = cut ? part.cutLengthMicron : part.finishedLengthMicron
  const w = cut ? part.cutWidthMicron : part.finishedWidthMicron
  const h = cut ? part.cutHeightMicron : part.finishedHeightMicron
  if (w > 0 && h > 0) return `${micronToMm(w)} × ${micronToMm(h)} mm`
  return `${micronToMm(l)} mm`
}

export function PartsPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { parts, partsStatus, partsError, createPartStatus, createPartError, materialSpecs, stockFormats } =
    useAppSelector((state) => state.catalog)

  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [kind, setKind] = useState<PartKind>('2d')
  const [length, setLength] = useState('')
  const [width, setWidth] = useState('')
  const [height, setHeight] = useState('')
  const [materialSpecId, setMaterialSpecId] = useState('')
  const [grain, setGrain] = useState<GrainMode>('none')
  const [allowRotate, setAllowRotate] = useState(true)
  const [priority, setPriority] = useState('100')
  // A single optional operation, enough to show finished size → cut size.
  const [edgeAllowance, setEdgeAllowance] = useState('')

  const [planQty, setPlanQty] = useState<Record<string, string>>({})
  const [includeRemnants, setIncludeRemnants] = useState(true)
  const [planning, setPlanning] = useState(false)
  const [planError, setPlanError] = useState<string | undefined>()
  const [planNotice, setPlanNotice] = useState<string | undefined>()

  useEffect(() => {
    void dispatch(fetchParts())
    void dispatch(fetchMaterialSpecs())
    void dispatch(fetchStockFormats())
    void dispatch(fetchMaterials())
  }, [dispatch])

  const loading = partsStatus === 'loading'

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!code.trim()) return

    const input: CreatePartInput = {
      code: code.trim(),
      name: name.trim() || undefined,
      materialSpecId: materialSpecId.trim() || undefined,
      grain,
      allowRotate,
      priority: Number(priority) || 100,
    }
    if (kind === '1d') {
      const mm = Number(length)
      if (!Number.isFinite(mm) || mm <= 0) return
      input.finishedLengthMicron = mmToMicron(mm)
    } else {
      const mmW = Number(width)
      const mmH = Number(height)
      if (!Number.isFinite(mmW) || !Number.isFinite(mmH) || mmW <= 0 || mmH <= 0) return
      input.finishedWidthMicron = mmToMicron(mmW)
      input.finishedHeightMicron = mmToMicron(mmH)
    }
    const allowanceMm = Number(edgeAllowance)
    if (Number.isFinite(allowanceMm) && allowanceMm > 0) {
      input.routings = [
        {
          seq: 1,
          operation: 'edge grinding',
          allowanceMicron: mmToMicron(allowanceMm),
          allowancePerEdge: true,
        },
      ]
    }

    const action = await dispatch(createPart(input))
    if (createPart.fulfilled.match(action)) {
      setCode('')
      setName('')
      setLength('')
      setWidth('')
      setHeight('')
      setMaterialSpecId('')
      setGrain('none')
      setAllowRotate(true)
      setPriority('100')
      setEdgeAllowance('')
    }
  }

  const selectedLines = useMemo<OrderLine[]>(() => {
    const lines: OrderLine[] = []
    for (const part of parts) {
      const quantity = Number(planQty[part.id] ?? '0') || 0
      if (quantity <= 0) continue
      lines.push({
        part: toProblemPart(part, quantity),
        materialSpecId: part.materialSpecId,
        dimension: profileOf(part, materialSpecs),
      })
    }
    return lines
  }, [parts, planQty, materialSpecs])

  const planParts = async () => {
    if (selectedLines.length === 0) return
    const orders = buildCatalogOrders({ lines: selectedLines, materialSpecs, stockFormats })
    if (orders.length === 0) {
      setPlanError('No stock format matches the selected parts.')
      return
    }
    setPlanning(true)
    setPlanError(undefined)
    setPlanNotice(undefined)
    try {
      for (const order of orders) {
        if (order.problem.stocks.length === 0) {
          throw new Error(`No stock for ${order.label}. Add a stock format for that material first.`)
        }
        const action = await dispatch(
          optimizeProblem({
            problem: order.problem,
            dimension: order.dimension,
            materialSpecId: order.materialSpecId,
            includeRemnants,
          }),
        )
        if (!optimizeProblem.fulfilled.match(action)) {
          throw new Error(action.error.message ?? 'Optimization failed')
        }
      }
      setPlanNotice(
        orders.length === 1
          ? 'Plan created.'
          : `${orders.length} plans created (one per material/dimension).`,
      )
      navigate('/viewer')
    } catch (error) {
      setPlanError(error instanceof Error ? error.message : 'Optimization failed')
    } finally {
      setPlanning(false)
    }
  }

  const totalSelected = selectedLines.reduce((sum, line) => sum + (line.part.quantity ?? 0), 0)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Parts</h1>
          <p className="text-sm text-muted-foreground">
            Finished sizes and the routings that turn them into cut sizes. Pick quantities and plan a
            real order against catalog stock.
          </p>
        </div>
        <Button variant="outline" onClick={() => dispatch(fetchParts())} disabled={loading}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {loading ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Catalog</CardTitle>
              <CardDescription>
                {parts.length} part(s) from /api/v1/parts — finished size is the drawing, cut size is
                what the solver cuts.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {partsError ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                  <div className="font-medium text-destructive">Database not reachable</div>
                  <p className="mt-1 text-muted-foreground">{partsError}</p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Start it with <code>db/scripts/up.ps1</code>, then{' '}
                    <code>db/scripts/migrate.ps1</code> and <code>db/scripts/seed.ps1</code>.
                  </p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Code</TableHead>
                      <TableHead>Finished</TableHead>
                      <TableHead>Cut</TableHead>
                      <TableHead>Material</TableHead>
                      <TableHead>Grain</TableHead>
                      <TableHead className="text-right">Priority</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {parts.map((part) => {
                      const spec = materialSpecs.find((item) => item.id === part.materialSpecId)
                      return (
                        <TableRow key={part.id}>
                          <TableCell className="font-medium">
                            {part.code}
                            {part.name && (
                              <div className="text-xs text-muted-foreground">{part.name}</div>
                            )}
                          </TableCell>
                          <TableCell>{sizeLabel(part, false)}</TableCell>
                          <TableCell>
                            <span className="font-medium">{sizeLabel(part, true)}</span>
                            {(part.routings ?? []).length > 0 && (
                              <div className="text-xs text-muted-foreground">
                                +{micronToMm(
                                  part.cutWidthMicron - part.finishedWidthMicron ||
                                    part.cutLengthMicron - part.finishedLengthMicron,
                                )}{' '}
                                mm
                              </div>
                            )}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {spec ? `${spec.materialName} · ${spec.name || spec.code}` : '—'}
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{part.grain}</Badge>
                          </TableCell>
                          <TableCell className="text-right">{part.priority}</TableCell>
                        </TableRow>
                      )
                    })}
                    {parts.length === 0 && !loading && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">
                          No parts yet. Run db/scripts/seed.ps1 or add one on the right.
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Plan an order</CardTitle>
              <CardDescription>
                Enter the quantities to make; parts are grouped by material and dimension, and each
                group is planned against its own stock.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {parts.length === 0 ? (
                <p className="text-sm text-muted-foreground">No parts to plan yet.</p>
              ) : (
                <div className="space-y-2">
                  {parts.map((part) => (
                    <div
                      key={part.id}
                      className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
                    >
                      <div className="min-w-0">
                        <div className="truncate text-sm font-medium">{part.code}</div>
                        <div className="text-xs text-muted-foreground">
                          cut {sizeLabel(part, true)}
                        </div>
                      </div>
                      <Input
                        className="w-20"
                        inputMode="numeric"
                        placeholder="0"
                        value={planQty[part.id] ?? ''}
                        onChange={(event) =>
                          setPlanQty((current) => ({ ...current, [part.id]: event.target.value }))
                        }
                      />
                    </div>
                  ))}
                </div>
              )}
              <div className="flex items-center justify-between rounded-md border border-border p-3">
                <div>
                  <div className="text-sm font-medium">Use available remnants</div>
                  <p className="text-xs text-muted-foreground">
                    Paid-for leftovers are offered before new sheets.
                  </p>
                </div>
                <Switch checked={includeRemnants} onCheckedChange={setIncludeRemnants} />
              </div>
              {planError && <p className="text-sm text-destructive">{planError}</p>}
              {planNotice && <p className="text-sm text-emerald-600">{planNotice}</p>}
              <Button
                className="w-full"
                disabled={totalSelected === 0 || planning}
                onClick={planParts}
              >
                <Play className="mr-2 h-4 w-4" />
                {planning ? 'Planning…' : `Plan ${totalSelected || ''} piece(s)`}
              </Button>
            </CardContent>
          </Card>
        </div>

        <Card className="self-start">
          <CardHeader>
            <CardTitle>Add part</CardTitle>
            <CardDescription>POST /api/v1/parts — finished sizes in mm</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-3" onSubmit={submit}>
              <div className="space-y-1.5">
                <Label htmlFor="part-code">Code</Label>
                <Input
                  id="part-code"
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                  placeholder="WINDOW-800x1000"
                  required
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="part-name">Name</Label>
                <Input
                  id="part-name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Window pane"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="part-kind">Geometry</Label>
                <Select value={kind} onValueChange={(value) => setKind(value as PartKind)}>
                  <SelectTrigger id="part-kind" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="2d">2D — width × height</SelectItem>
                    <SelectItem value="1d">1D — length only</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {kind === '1d' ? (
                <div className="space-y-1.5">
                  <Label htmlFor="part-length">Finished length (mm)</Label>
                  <Input
                    id="part-length"
                    type="number"
                    min="1"
                    step="1"
                    value={length}
                    onChange={(event) => setLength(event.target.value)}
                    placeholder="2400"
                    required
                  />
                </div>
              ) : (
                <div className="grid grid-cols-2 gap-2">
                  <div className="space-y-1.5">
                    <Label htmlFor="part-width">Width (mm)</Label>
                    <Input
                      id="part-width"
                      type="number"
                      min="1"
                      step="1"
                      value={width}
                      onChange={(event) => setWidth(event.target.value)}
                      placeholder="800"
                      required
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="part-height">Height (mm)</Label>
                    <Input
                      id="part-height"
                      type="number"
                      min="1"
                      step="1"
                      value={height}
                      onChange={(event) => setHeight(event.target.value)}
                      placeholder="1000"
                      required
                    />
                  </div>
                </div>
              )}
              <div className="space-y-1.5">
                <Label>Material type</Label>
                <Select
                  value={materialSpecId || 'none'}
                  onValueChange={(value) => setMaterialSpecId(value === 'none' ? '' : value)}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Choose…" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">None</SelectItem>
                    {materialSpecs.map((spec) => (
                      <SelectItem key={spec.id} value={spec.id}>
                        {spec.materialName} · {spec.name || spec.code}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1.5">
                  <Label htmlFor="part-grain">Grain</Label>
                  <Select value={grain} onValueChange={(value) => setGrain(value as GrainMode)}>
                    <SelectTrigger id="part-grain" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">none</SelectItem>
                      <SelectItem value="along_x">along_x</SelectItem>
                      <SelectItem value="along_y">along_y</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="part-priority">Priority</Label>
                  <Input
                    id="part-priority"
                    type="number"
                    min="1"
                    step="1"
                    value={priority}
                    onChange={(event) => setPriority(event.target.value)}
                  />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="part-allowance">Edge allowance (mm, optional)</Label>
                <Input
                  id="part-allowance"
                  type="number"
                  min="0"
                  step="1"
                  value={edgeAllowance}
                  onChange={(event) => setEdgeAllowance(event.target.value)}
                  placeholder="2"
                />
                <p className="text-xs text-muted-foreground">
                  Added on every edge; the cut size grows by twice this amount.
                </p>
              </div>
              <label className="flex items-center justify-between gap-2 text-sm">
                <span className="text-muted-foreground">Allow 90° rotation</span>
                <Switch
                  checked={allowRotate}
                  onCheckedChange={(checked) => setAllowRotate(checked === true)}
                />
              </label>
              <Button type="submit" disabled={createPartStatus === 'loading'} className="w-full">
                <Plus className="mr-2 h-4 w-4" />
                {createPartStatus === 'loading' ? 'Creating…' : 'Create part'}
              </Button>
              {createPartError && <p className="text-xs text-destructive">{createPartError}</p>}
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
