import { useEffect, useState, type FormEvent } from 'react'
import { Plus, RefreshCw } from 'lucide-react'

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
import { createPart, fetchParts } from '@/features/catalog/catalogSlice'
import { micronToMm, mmToMicron } from '@/lib/format'
import type { CreatePartInput, GrainMode } from '@/lib/types'

type PartKind = '1d' | '2d'

export function PartsPage() {
  const dispatch = useAppDispatch()
  const { parts, partsStatus, partsError, createPartStatus, createPartError } = useAppSelector(
    (state) => state.catalog,
  )

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

  useEffect(() => {
    void dispatch(fetchParts())
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
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Parts</h1>
          <p className="text-sm text-muted-foreground">
            Finished sizes from the database. Cut sizes are derived from routings (allowances), never
            typed twice.
          </p>
        </div>
        <Button variant="outline" onClick={() => dispatch(fetchParts())} disabled={loading}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {loading ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_340px]">
        <Card>
          <CardHeader>
            <CardTitle>Catalog</CardTitle>
            <CardDescription>{parts.length} part(s) from /api/v1/parts</CardDescription>
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
                    <TableHead>Name</TableHead>
                    <TableHead>Finished size</TableHead>
                    <TableHead>Grain</TableHead>
                    <TableHead>Rotate</TableHead>
                    <TableHead className="text-right">Priority</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {parts.map((part) => (
                    <TableRow key={part.id}>
                      <TableCell className="font-medium">{part.code}</TableCell>
                      <TableCell>{part.name || '—'}</TableCell>
                      <TableCell>
                        {part.finishedWidthMicron > 0
                          ? `${micronToMm(part.finishedWidthMicron)} × ${micronToMm(part.finishedHeightMicron)} mm`
                          : `${micronToMm(part.finishedLengthMicron)} mm`}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{part.grain}</Badge>
                      </TableCell>
                      <TableCell>{part.allowRotate ? 'yes' : 'no'}</TableCell>
                      <TableCell className="text-right">{part.priority}</TableCell>
                    </TableRow>
                  ))}
                  {parts.length === 0 && !loading && (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">
                        No parts yet. Run db/scripts/seed.ps1 to load the demo catalog.
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
                <Label htmlFor="part-spec">Material spec id (optional)</Label>
                <Input
                  id="part-spec"
                  value={materialSpecId}
                  onChange={(event) => setMaterialSpecId(event.target.value)}
                  placeholder="uuid from db/seeds"
                />
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
