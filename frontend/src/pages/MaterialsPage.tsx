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
  createMaterial,
  createMaterialSpec,
  fetchMaterials,
  fetchMaterialSpecs,
} from '@/features/catalog/catalogSlice'
import { micronToMm, mmToMicron } from '@/lib/format'
import type { DimensionProfile } from '@/lib/types'

export function MaterialsPage() {
  const dispatch = useAppDispatch()
  const {
    materials,
    materialsStatus,
    materialsError,
    createMaterialStatus,
    createMaterialError,
    materialSpecs,
    materialSpecsError,
    createMaterialSpecStatus,
    createMaterialSpecError,
  } = useAppSelector((state) => state.catalog)
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [profile, setProfile] = useState<DimensionProfile>('2d')
  const [specMaterialId, setSpecMaterialId] = useState('')
  const [specCode, setSpecCode] = useState('')
  const [specName, setSpecName] = useState('')
  const [specThickness, setSpecThickness] = useState('')
  const [specFinish, setSpecFinish] = useState('')
  const [specColor, setSpecColor] = useState('')

  useEffect(() => {
    void dispatch(fetchMaterials())
    void dispatch(fetchMaterialSpecs())
  }, [dispatch])

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!code.trim() || !name.trim()) return
    const action = await dispatch(
      createMaterial({ code: code.trim(), name: name.trim(), dimensionProfile: profile }),
    )
    if (createMaterial.fulfilled.match(action)) {
      setCode('')
      setName('')
      setProfile('2d')
    }
  }

  const submitSpec = async (event: FormEvent) => {
    event.preventDefault()
    if (!specMaterialId || !specCode.trim()) return
    const action = await dispatch(
      createMaterialSpec({
        materialId: specMaterialId,
        code: specCode.trim(),
        name: specName.trim() || undefined,
        thicknessMicron: mmToMicron(Number(specThickness) || 0),
        finish: specFinish.trim() || undefined,
        color: specColor.trim() || undefined,
      }),
    )
    if (createMaterialSpec.fulfilled.match(action)) {
      setSpecCode('')
      setSpecName('')
      setSpecThickness('')
      setSpecFinish('')
      setSpecColor('')
    }
  }

  const loading = materialsStatus === 'loading'
  const familyName = (materialId: string) =>
    materials.find((material) => material.id === materialId)?.name ?? '—'

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Materials</h1>
          <p className="text-sm text-muted-foreground">
            Material families the plant works with. The dimension profile decides which solvers can
            use them: 1D bars, 2D sheets or 3D boxes.
          </p>
        </div>
        <Button variant="outline" onClick={() => dispatch(fetchMaterials())} disabled={loading}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {loading ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <Card>
          <CardHeader>
            <CardTitle>Catalog</CardTitle>
            <CardDescription>{materials.length} material(s) from /api/v1/materials</CardDescription>
          </CardHeader>
          <CardContent>
            {materialsError ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                <div className="font-medium text-destructive">Database not reachable</div>
                <p className="mt-1 text-muted-foreground">{materialsError}</p>
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
                    <TableHead>Profile</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {materials.map((material) => (
                    <TableRow key={material.id}>
                      <TableCell className="font-medium">{material.code}</TableCell>
                      <TableCell>{material.name || '—'}</TableCell>
                      <TableCell>
                        <Badge variant="secondary">{material.dimensionProfile}</Badge>
                      </TableCell>
                      <TableCell>
                        {material.isActive ? (
                          <Badge className="bg-emerald-600 text-white">active</Badge>
                        ) : (
                          <Badge variant="outline">inactive</Badge>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                  {materials.length === 0 && !loading && (
                    <TableRow>
                      <TableCell colSpan={4} className="text-center text-sm text-muted-foreground">
                        No materials yet. Run db/scripts/seed.ps1 to load the demo catalog.
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
            <CardTitle>Add material</CardTitle>
            <CardDescription>POST /api/v1/materials</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-3" onSubmit={submit}>
              <div className="space-y-1.5">
                <Label htmlFor="material-code">Code</Label>
                <Input
                  id="material-code"
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                  placeholder="GLASS"
                  required
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="material-name">Name</Label>
                <Input
                  id="material-name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Float glass"
                  required
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="material-profile">Dimension profile</Label>
                <Select
                  value={profile}
                  onValueChange={(value) => setProfile(value as DimensionProfile)}
                >
                  <SelectTrigger id="material-profile" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="1d">1D — bars, profiles</SelectItem>
                    <SelectItem value="2d">2D — sheets, panels</SelectItem>
                    <SelectItem value="3d">3D — boxes (future)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <Button type="submit" disabled={createMaterialStatus === 'loading'} className="w-full">
                <Plus className="mr-2 h-4 w-4" />
                {createMaterialStatus === 'loading' ? 'Creating…' : 'Create material'}
              </Button>
              {createMaterialError && (
                <p className="text-xs text-destructive">{createMaterialError}</p>
              )}
            </form>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Types</CardTitle>
          <CardDescription>
            Material types (specs) — the concrete boards and profiles you can stock and build from.{' '}
            {materialSpecs.length} type(s) from /api/v1/material-specs.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
            <div>
              {materialSpecsError ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                  <div className="font-medium text-destructive">Database not reachable</div>
                  <p className="mt-1 text-muted-foreground">{materialSpecsError}</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Family</TableHead>
                      <TableHead>Code</TableHead>
                      <TableHead>Name</TableHead>
                      <TableHead>Thickness</TableHead>
                      <TableHead>Finish</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {materialSpecs.map((spec) => (
                      <TableRow key={spec.id}>
                        <TableCell>{spec.materialName || familyName(spec.materialId)}</TableCell>
                        <TableCell className="font-medium">{spec.code}</TableCell>
                        <TableCell>{spec.name || '—'}</TableCell>
                        <TableCell>
                          {spec.thicknessMicron > 0 ? `${micronToMm(spec.thicknessMicron)} mm` : '—'}
                        </TableCell>
                        <TableCell>{spec.finish || '—'}</TableCell>
                      </TableRow>
                    ))}
                    {materialSpecs.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center text-sm text-muted-foreground">
                          No types yet. Add one on the right.
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              )}
            </div>

            <form className="space-y-3" onSubmit={submitSpec}>
              <div className="space-y-1.5">
                <Label>Family</Label>
                <Select
                  value={specMaterialId || 'none'}
                  onValueChange={(value) => setSpecMaterialId(value === 'none' ? '' : value)}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Choose…" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">Choose a family…</SelectItem>
                    {materials.map((material) => (
                      <SelectItem key={material.id} value={material.id}>
                        {material.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1.5">
                  <Label htmlFor="spec-code">Code</Label>
                  <Input
                    id="spec-code"
                    value={specCode}
                    onChange={(event) => setSpecCode(event.target.value)}
                    placeholder="OAK-18"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="spec-name">Name</Label>
                  <Input
                    id="spec-name"
                    value={specName}
                    onChange={(event) => setSpecName(event.target.value)}
                    placeholder="Oak 18 mm"
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1.5">
                  <Label htmlFor="spec-thickness">Thickness (mm)</Label>
                  <Input
                    id="spec-thickness"
                    type="number"
                    min="0"
                    step="1"
                    value={specThickness}
                    onChange={(event) => setSpecThickness(event.target.value)}
                    placeholder="18"
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="spec-finish">Finish</Label>
                  <Input
                    id="spec-finish"
                    value={specFinish}
                    onChange={(event) => setSpecFinish(event.target.value)}
                    placeholder="sanded"
                  />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="spec-color">Colour</Label>
                <Input
                  id="spec-color"
                  value={specColor}
                  onChange={(event) => setSpecColor(event.target.value)}
                  placeholder="natural"
                />
              </div>
              <Button
                type="submit"
                disabled={!specMaterialId || !specCode.trim() || createMaterialSpecStatus === 'loading'}
                className="w-full"
              >
                <Plus className="mr-2 h-4 w-4" />
                {createMaterialSpecStatus === 'loading' ? 'Creating…' : 'Add type'}
              </Button>
              {createMaterialSpecError && (
                <p className="text-xs text-destructive">{createMaterialSpecError}</p>
              )}
            </form>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
