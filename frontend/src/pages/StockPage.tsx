import { useEffect, useState } from 'react'
import { Archive, RefreshCw } from 'lucide-react'

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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import {
  createStockFormat,
  createStockPiece,
  fetchMaterials,
  fetchMaterialSpecs,
  fetchStockFormats,
  fetchStockItems,
  updateStockPiece,
} from '@/features/catalog/catalogSlice'
import { micronToMm, mmToMicron } from '@/lib/format'
import type { Material, MaterialSpec, Rect, StockItemStatus, StockPiece } from '@/lib/types'

const statusVariants: Record<StockItemStatus, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  available: 'default',
  reserved: 'secondary',
  consumed: 'outline',
  retired: 'destructive',
}

export function StockPage() {
  const dispatch = useAppDispatch()
  const {
    materials,
    materialSpecs,
    stockFormats: formats,
    stockStatus,
    stockError,
    stockItems: pieces,
    stockItemsStatus,
    stockItemsError,
    createStockPieceStatus,
    createStockPieceError,
    createStockFormatStatus,
    createStockFormatError,
    updateStockPieceStatus,
    updateStockPieceError,
  } = useAppSelector((state) => state.catalog)
  const [statusFilter, setStatusFilter] = useState<StockItemStatus | 'all'>('available')
  const [defectTarget, setDefectTarget] = useState<StockPiece>()

  useEffect(() => {
    void dispatch(fetchStockFormats())
    void dispatch(fetchMaterials())
    void dispatch(fetchMaterialSpecs())
  }, [dispatch])

  useEffect(() => {
    void dispatch(fetchStockItems(statusFilter === 'all' ? undefined : { status: statusFilter }))
  }, [dispatch, statusFilter])

  const loading = stockStatus === 'loading' || stockItemsStatus === 'loading'
  const refresh = () => {
    void dispatch(fetchStockFormats())
    void dispatch(fetchMaterials())
    void dispatch(fetchMaterialSpecs())
    void dispatch(fetchStockItems(statusFilter === 'all' ? undefined : { status: statusFilter }))
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Stock</h1>
          <p className="text-sm text-muted-foreground">
            Catalog formats the solver can buy from, and the physical pieces in the shop — including
            the labelled remnants that later plans should use first.
          </p>
        </div>
        <Button variant="outline" onClick={refresh} disabled={loading}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {loading ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      <Tabs defaultValue="formats">
        <TabsList>
          <TabsTrigger value="formats">Formats</TabsTrigger>
          <TabsTrigger value="pieces">Pieces &amp; remnants</TabsTrigger>
        </TabsList>

        <TabsContent value="formats">
          <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
          <Card>
            <CardHeader>
              <CardTitle>Formats</CardTitle>
              <CardDescription>
                {formats.length} format(s) from /api/v1/stock-formats — on-hand quantities are
                decremented when a plan is accepted.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {stockError ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                  <div className="font-medium text-destructive">Database not reachable</div>
                  <p className="mt-1 text-muted-foreground">{stockError}</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Code</TableHead>
                      <TableHead>Material</TableHead>
                      <TableHead>Profile</TableHead>
                      <TableHead>Size</TableHead>
                      <TableHead className="text-right">On hand</TableHead>
                      <TableHead className="text-right">Cost / unit</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {formats.map((format) => (
                      <TableRow key={format.id}>
                        <TableCell className="font-medium">{format.code}</TableCell>
                        <TableCell>
                          {format.materialCode} · {format.specCode}
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary">{format.dimensionProfile}</Badge>
                        </TableCell>
                        <TableCell>
                          {format.dimensionProfile === '1d'
                            ? `${micronToMm(format.lengthMicron)} mm`
                            : `${micronToMm(format.widthMicron)} × ${micronToMm(format.heightMicron)} mm`}
                        </TableCell>
                        <TableCell className="text-right">{format.onHandQty}</TableCell>
                        <TableCell className="text-right">{format.costPerUnit.toFixed(2)}</TableCell>
                      </TableRow>
                    ))}
                    {formats.length === 0 && !loading && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">
                          No stock formats yet. Run db/scripts/seed.ps1 to load the demo catalog.
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
            <AddStockFormatCard
              materials={materials}
              specs={materialSpecs}
              status={createStockFormatStatus}
              error={createStockFormatError}
            />
          </div>
        </TabsContent>

        <TabsContent value="pieces">
          <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
            <Card>
              <CardHeader>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <CardTitle>Pieces &amp; remnants</CardTitle>
                    <CardDescription>
                      {pieces.length} physical piece(s) from /api/v1/stock-items. Labels are what the
                      shop floor scans.
                    </CardDescription>
                  </div>
                  <Select
                    value={statusFilter}
                    onValueChange={(value) => setStatusFilter(value as StockItemStatus | 'all')}
                  >
                    <SelectTrigger className="w-[160px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="available">Available</SelectItem>
                      <SelectItem value="reserved">Reserved</SelectItem>
                      <SelectItem value="consumed">Consumed</SelectItem>
                      <SelectItem value="retired">Retired</SelectItem>
                      <SelectItem value="all">All statuses</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </CardHeader>
              <CardContent className="space-y-3">
                {stockItemsError ? (
                  <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                    <div className="font-medium text-destructive">Database not reachable</div>
                    <p className="mt-1 text-muted-foreground">{stockItemsError}</p>
                  </div>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Label</TableHead>
                        <TableHead>Size</TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Location</TableHead>
                        <TableHead>Source</TableHead>
                        <TableHead className="text-right">Cost basis</TableHead>
                        <TableHead>Defects</TableHead>
                        <TableHead />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {pieces.map((piece) => (
                        <TableRow key={piece.id}>
                          <TableCell>
                            <div className="font-medium">{piece.label}</div>
                            <div className="text-xs text-muted-foreground">
                              {piece.isRemnant ? 'remnant' : 'full piece'} · {piece.code}
                            </div>
                          </TableCell>
                          <TableCell>{pieceSize(piece)}</TableCell>
                          <TableCell>
                            <Badge variant={statusVariants[piece.status]}>{piece.status}</Badge>
                          </TableCell>
                          <TableCell>{piece.location || '—'}</TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {piece.parentPlanId ? `plan ${piece.parentPlanId.slice(0, 8)}…` : '—'}
                          </TableCell>
                          <TableCell className="text-right">
                            {piece.costPerUnit > 0 ? piece.costPerUnit.toFixed(2) : '—'}
                          </TableCell>
                          <TableCell>
                            {(piece.defects?.length ?? 0) > 0 ? (
                              <Badge variant="destructive">{piece.defects?.length}</Badge>
                            ) : (
                              <span className="text-xs text-muted-foreground">—</span>
                            )}
                          </TableCell>
                          <TableCell className="text-right">
                            {piece.widthMicron && piece.heightMicron ? (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => setDefectTarget(piece)}
                              >
                                Defects
                              </Button>
                            ) : null}
                            {(piece.status === 'available' || piece.status === 'reserved') && (
                              <Button
                                variant="ghost"
                                size="sm"
                                disabled={updateStockPieceStatus === 'loading'}
                                onClick={() =>
                                  void dispatch(
                                    updateStockPiece({ id: piece.id, input: { status: 'retired' } }),
                                  )
                                }
                              >
                                <Archive className="mr-1 h-3.5 w-3.5" />
                                Retire
                              </Button>
                            )}
                          </TableCell>
                        </TableRow>
                      ))}
                      {pieces.length === 0 && !loading && (
                        <TableRow>
                          <TableCell colSpan={8} className="text-center text-sm text-muted-foreground">
                            No pieces with this status. Accept a plan to create labelled remnants, or
                            register one on the right.
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                )}
                {updateStockPieceError && (
                  <p className="text-sm text-destructive">{updateStockPieceError}</p>
                )}
              </CardContent>
            </Card>

            {defectTarget ? (
              <DefectEditorCard piece={defectTarget} onClose={() => setDefectTarget(undefined)} />
            ) : (
              <RegisterPieceCard
                formats={formats}
                status={createStockPieceStatus}
                error={createStockPieceError}
              />
            )}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function pieceSize(piece: StockPiece): string {
  if (piece.lengthMicron) return `${micronToMm(piece.lengthMicron)} mm`
  if (piece.widthMicron && piece.heightMicron) {
    return `${micronToMm(piece.widthMicron)} × ${micronToMm(piece.heightMicron)} mm`
  }
  return '—'
}

interface RegisterPieceCardProps {
  formats: { id: string; code: string }[]
  status: 'idle' | 'loading' | 'ready' | 'error'
  error?: string
}

function RegisterPieceCard({ formats, status, error }: RegisterPieceCardProps) {
  const dispatch = useAppDispatch()
  const [label, setLabel] = useState('')
  const [profile, setProfile] = useState<'1d' | '2d'>('2d')
  const [length, setLength] = useState('')
  const [width, setWidth] = useState('')
  const [height, setHeight] = useState('')
  const [location, setLocation] = useState('')
  const [formatId, setFormatId] = useState('none')
  const [isRemnant, setIsRemnant] = useState(true)

  const formatCode = formats.find((format) => format.id === formatId)?.code

  const submit = () => {
    const input = {
      label: label.trim(),
      isRemnant,
      location: location.trim(),
      formatId: formatId === 'none' ? undefined : formatId,
      ...(profile === '1d'
        ? { lengthMicron: mmToMicron(Number(length) || 0) }
        : {
            widthMicron: mmToMicron(Number(width) || 0),
            heightMicron: mmToMicron(Number(height) || 0),
          }),
    }
    void dispatch(createStockPiece(input)).then((action) => {
      if (createStockPiece.fulfilled.match(action)) {
        setLabel('')
        setLength('')
        setWidth('')
        setHeight('')
        setLocation('')
      }
    })
  }

  const valid =
    label.trim().length > 0 &&
    (profile === '1d'
      ? Number(length) > 0
      : Number(width) > 0 && Number(height) > 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Register a piece</CardTitle>
        <CardDescription>
          Manually measured remnants get a label and join the pool; later runs can be asked to use
          them first.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor="piece-label">Label</Label>
          <Input
            id="piece-label"
            placeholder="OFF-2026-0042"
            value={label}
            onChange={(event) => setLabel(event.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Format (optional)</Label>
          <Select value={formatId} onValueChange={setFormatId}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">No catalog format</SelectItem>
              {formats.map((format) => (
                <SelectItem key={format.id} value={format.id}>
                  {format.code}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {formatCode && (
            <p className="text-xs text-muted-foreground">
              Inherits code, size and cost from {formatCode}.
            </p>
          )}
        </div>
        <div className="space-y-1.5">
          <Label>Shape</Label>
          <Select value={profile} onValueChange={(value) => setProfile(value as '1d' | '2d')}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="2d">Sheet / panel (width × height)</SelectItem>
              <SelectItem value="1d">Bar / profile (length)</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {profile === '1d' ? (
          <div className="space-y-1.5">
            <Label htmlFor="piece-length">Length (mm)</Label>
            <Input
              id="piece-length"
              inputMode="decimal"
              value={length}
              onChange={(event) => setLength(event.target.value)}
            />
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="piece-width">Width (mm)</Label>
              <Input
                id="piece-width"
                inputMode="decimal"
                value={width}
                onChange={(event) => setWidth(event.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="piece-height">Height (mm)</Label>
              <Input
                id="piece-height"
                inputMode="decimal"
                value={height}
                onChange={(event) => setHeight(event.target.value)}
              />
            </div>
          </div>
        )}
        <div className="space-y-1.5">
          <Label htmlFor="piece-location">Location</Label>
          <Input
            id="piece-location"
            placeholder="Rack A / bay 3"
            value={location}
            onChange={(event) => setLocation(event.target.value)}
          />
        </div>
        <div className="flex items-center justify-between rounded-md border border-border p-3">
          <div>
            <div className="text-sm font-medium">Reusable remnant</div>
            <p className="text-xs text-muted-foreground">
              Remnants are offered to solvers before fresh stock.
            </p>
          </div>
          <Switch checked={isRemnant} onCheckedChange={setIsRemnant} />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button className="w-full" disabled={!valid || status === 'loading'} onClick={submit}>
          {status === 'loading' ? 'Registering…' : 'Register piece'}
        </Button>
      </CardContent>
    </Card>
  )
}

interface AddStockFormatCardProps {
  materials: Material[]
  specs: MaterialSpec[]
  status: 'idle' | 'loading' | 'ready' | 'error'
  error?: string
}

function AddStockFormatCard({ materials, specs, status, error }: AddStockFormatCardProps) {
  const dispatch = useAppDispatch()
  const [materialId, setMaterialId] = useState('')
  const [specId, setSpecId] = useState('')
  const [code, setCode] = useState('')
  const [length, setLength] = useState('')
  const [width, setWidth] = useState('')
  const [height, setHeight] = useState('')
  const [qty, setQty] = useState('1')
  const [cost, setCost] = useState('')

  const familySpecs = specs.filter((spec) => spec.materialId === materialId)
  const spec = specs.find((item) => item.id === specId)
  const profile = spec?.dimensionProfile ?? '2d'

  const valid =
    !!specId &&
    code.trim().length > 0 &&
    (profile === '1d' ? Number(length) > 0 : Number(width) > 0 && Number(height) > 0)

  const submit = () => {
    void dispatch(
      createStockFormat({
        materialSpecId: specId,
        code: code.trim(),
        ...(profile === '1d'
          ? {
              lengthMicron: mmToMicron(Number(length) || 0),
              widthMicron: mmToMicron(Number(width) || 0),
            }
          : {
              widthMicron: mmToMicron(Number(width) || 0),
              heightMicron: mmToMicron(Number(height) || 0),
            }),
        onHandQty: Math.max(0, Math.round(Number(qty) || 0)),
        costPerUnit: Math.max(0, Number(cost) || 0),
      }),
    ).then((action) => {
      if (createStockFormat.fulfilled.match(action)) {
        setCode('')
        setLength('')
        setWidth('')
        setHeight('')
        setQty('1')
        setCost('')
      }
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add stock</CardTitle>
        <CardDescription>POST /api/v1/stock-formats — a size the solver can buy</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1.5">
          <Label>Material</Label>
          <Select
            value={materialId || 'none'}
            onValueChange={(value) => {
              const id = value === 'none' ? '' : value
              setMaterialId(id)
              const first = specs.find((item) => item.materialId === id)
              setSpecId(first?.id ?? '')
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder="Choose…" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">Choose a material…</SelectItem>
              {materials.map((material) => (
                <SelectItem key={material.id} value={material.id}>
                  {material.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label>Type</Label>
          <Select
            value={specId || 'none'}
            onValueChange={(value) => setSpecId(value === 'none' ? '' : value)}
            disabled={!materialId}
          >
            <SelectTrigger>
              <SelectValue placeholder="Choose…" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">Choose a type…</SelectItem>
              {familySpecs.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {item.name || item.code}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="format-code">Code</Label>
          <Input
            id="format-code"
            value={code}
            onChange={(event) => setCode(event.target.value)}
            placeholder="SHEET-OAK-2440x1220"
          />
        </div>
        {profile === '1d' ? (
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1.5">
              <Label htmlFor="format-length">Length (mm)</Label>
              <Input
                id="format-length"
                inputMode="decimal"
                value={length}
                onChange={(event) => setLength(event.target.value)}
                placeholder="6000"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="format-section">Section (mm)</Label>
              <Input
                id="format-section"
                inputMode="decimal"
                value={width}
                onChange={(event) => setWidth(event.target.value)}
                placeholder="60"
              />
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1.5">
              <Label htmlFor="format-width">Width (mm)</Label>
              <Input
                id="format-width"
                inputMode="decimal"
                value={width}
                onChange={(event) => setWidth(event.target.value)}
                placeholder="2440"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="format-height">Height (mm)</Label>
              <Input
                id="format-height"
                inputMode="decimal"
                value={height}
                onChange={(event) => setHeight(event.target.value)}
                placeholder="1220"
              />
            </div>
          </div>
        )}
        <div className="grid grid-cols-2 gap-2">
          <div className="space-y-1.5">
            <Label htmlFor="format-qty">On hand</Label>
            <Input
              id="format-qty"
              type="number"
              min="0"
              step="1"
              value={qty}
              onChange={(event) => setQty(event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="format-cost">Cost / unit</Label>
            <Input
              id="format-cost"
              type="number"
              min="0"
              step="0.01"
              value={cost}
              onChange={(event) => setCost(event.target.value)}
              placeholder="48.00"
            />
          </div>
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button className="w-full" disabled={!valid || status === 'loading'} onClick={submit}>
          {status === 'loading' ? 'Adding…' : 'Add stock format'}
        </Button>
      </CardContent>
    </Card>
  )
}

interface DefectEditorCardProps {
  piece: StockPiece
  onClose: () => void
}

/**
 * Defects are unusable regions of a physical piece, in piece-local millimetres.
 * They are typed as "x, y, width, height" groups separated by semicolons and
 * stored (and validated) in micrometers by the API.
 */
function DefectEditorCard({ piece, onClose }: DefectEditorCardProps) {
  const dispatch = useAppDispatch()
  const updateStatus = useAppSelector((state) => state.catalog.updateStockPieceStatus)
  const error = useAppSelector((state) => state.catalog.updateStockPieceError)
  const [text, setText] = useState(() =>
    (piece.defects ?? [])
      .map((d) => [d.x, d.y, d.w, d.h].map((value) => micronToMm(value)).join(', '))
      .join('; '),
  )
  const [parseError, setParseError] = useState<string>()

  const parse = (): Rect[] | undefined => {
    const value = text.trim()
    if (!value) return []
    const rects: Rect[] = []
    for (const chunk of value.split(';').map((item) => item.trim()).filter(Boolean)) {
      const nums = chunk.split(',').map((item) => Number(item.trim()))
      if (nums.length !== 4 || nums.some((n) => !Number.isFinite(n))) {
        setParseError('Each defect needs four numbers: x, y, width, height (mm).')
        return undefined
      }
      const [x, y, w, h] = nums
      rects.push({ x: mmToMicron(x), y: mmToMicron(y), w: mmToMicron(w), h: mmToMicron(h) })
    }
    return rects
  }

  const save = () => {
    setParseError(undefined)
    const defects = parse()
    if (!defects) return
    void dispatch(updateStockPiece({ id: piece.id, input: { defects } })).then((action) => {
      if (updateStockPiece.fulfilled.match(action)) onClose()
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Defects on {piece.label}</CardTitle>
        <CardDescription>
          Unusable regions in piece-local millimetres: x, y, width, height. Separate several with a
          semicolon. Leave the field empty to clear them.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor="defect-map">Defects (mm)</Label>
          <Input
            id="defect-map"
            placeholder="100, 50, 300, 300; 1200, 400, 200, 200"
            value={text}
            onChange={(event) => setText(event.target.value)}
          />
        </div>
        <p className="text-xs text-muted-foreground">
          The solvers already avoid these regions; the validator rejects any plan that covers one.
        </p>
        {parseError && <p className="text-sm text-destructive">{parseError}</p>}
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="flex gap-2">
          <Button onClick={save} disabled={updateStatus === 'loading'}>
            {updateStatus === 'loading' ? 'Saving...' : 'Save defects'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
