import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Plus, RefreshCw, Scissors, Trash2, Wand2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { AssemblyViewer } from '@/features/assemblies/AssemblyViewer'
import { explodeComponents, type ExplodeComponent } from '@/features/assemblies/explode'
import {
  COMPONENT_COLORS,
  componentLabel,
  type AssemblyGeometry,
  type GeometryComponent,
} from '@/features/assemblies/geometry'
import {
  tableTemplate,
  windowTemplate,
  type TableTemplateInput,
  type TemplateComponent,
} from '@/features/assemblies/templates'
import { createAssembly, fetchAssemblies, selectAssembly } from '@/features/assemblies/assemblySlice'
import {
  fetchMaterials,
  fetchMaterialSpecs,
  fetchStockFormats,
} from '@/features/catalog/catalogSlice'
import { optimizeProblem } from '@/features/optimizer/optimizerSlice'
import { micronToMm, mmToMicron } from '@/lib/format'
import type {
  Assembly,
  AssemblyKind,
  ComponentKind,
  CreateAssemblyInput,
  DimensionProfile,
  Problem,
  StockItem,
} from '@/lib/types'

// ------------------------------------------------------------------ drafts ---

interface DraftComponent {
  key: string
  role: string
  kind: ComponentKind
  name: string
  materialSpecId: string
  quantity: string
  width: string
  height: string
  depth: string
  offsetX: string
  offsetY: string
  offsetZ: string
}

interface Draft {
  code: string
  name: string
  kind: AssemblyKind
  materialSpecId: string
  width: string
  height: string
  depth: string
  components: DraftComponent[]
}

let keySeq = 0
const nextKey = () => `c${(keySeq += 1)}`

const blankComponent = (): DraftComponent => ({
  key: nextKey(),
  role: 'part',
  kind: 'custom',
  name: '',
  materialSpecId: '',
  quantity: '1',
  width: '',
  height: '',
  depth: '',
  offsetX: '0',
  offsetY: '0',
  offsetZ: '0',
})

const blankDraft = (): Draft => ({
  code: '',
  name: '',
  kind: 'generic',
  materialSpecId: '',
  width: '',
  height: '',
  depth: '',
  components: [],
})

const toNumber = (value: string) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

const mmStr = (micron: number) => String(Math.round(micron / 1000))

const componentFromDraft = (component: DraftComponent): GeometryComponent => ({
  id: component.key,
  role: component.role.trim() || 'part',
  kind: component.kind,
  name: component.name,
  widthMicron: mmToMicron(toNumber(component.width)),
  heightMicron: mmToMicron(toNumber(component.height)),
  depthMicron: mmToMicron(toNumber(component.depth)),
  offsetXMicron: mmToMicron(toNumber(component.offsetX)),
  offsetYMicron: mmToMicron(toNumber(component.offsetY)),
  offsetZMicron: mmToMicron(toNumber(component.offsetZ)),
})

const geometryFromAssembly = (assembly: Assembly): AssemblyGeometry => ({
  widthMicron: assembly.widthMicron,
  heightMicron: assembly.heightMicron,
  depthMicron: assembly.depthMicron,
  components: assembly.components.map((component) => ({ ...component })),
})

const toDraftComponent = (template: TemplateComponent): DraftComponent => ({
  key: nextKey(),
  role: template.role,
  kind: template.kind,
  name: template.name,
  materialSpecId: '',
  quantity: String(template.quantity),
  width: String(Math.round(template.widthMm)),
  height: String(Math.round(template.heightMm)),
  depth: String(Math.round(template.depthMm)),
  offsetX: String(Math.round(template.offsetXMm)),
  offsetY: String(Math.round(template.offsetYMm)),
  offsetZ: String(Math.round(template.offsetZMm)),
})

// ------------------------------------------------------------------- page ---

export function ProductsPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const {
    assemblies,
    assembliesStatus,
    assembliesError,
    selectedId,
    createAssemblyStatus,
    createAssemblyError,
  } = useAppSelector((state) => state.assemblies)
  const { materials, materialSpecs, stockFormats } = useAppSelector((state) => state.catalog)
  const runStatus = useAppSelector((state) => state.optimizer.runStatus)
  const runError = useAppSelector((state) => state.optimizer.runError)

  const [draft, setDraft] = useState<Draft | null>(null)
  const [template, setTemplate] = useState<'window' | 'table'>('window')
  // Window template parameters.
  const [frameSection, setFrameSection] = useState('70')
  const [frameDepth, setFrameDepth] = useState('70')
  const [glassThickness, setGlassThickness] = useState('24')
  const [glassInset, setGlassInset] = useState('10')
  // Table template parameters.
  const [topThickness, setTopThickness] = useState('30')
  const [legSection, setLegSection] = useState('70')
  const [apronHeight, setApronHeight] = useState('90')
  const [apronThickness, setApronThickness] = useState('20')
  const [legInset, setLegInset] = useState('40')
  const [includeShelf, setIncludeShelf] = useState(true)
  // How many products the cut plan should cover.
  const [productQuantity, setProductQuantity] = useState('1')

  useEffect(() => {
    void dispatch(fetchAssemblies())
    void dispatch(fetchMaterials())
    void dispatch(fetchMaterialSpecs())
    void dispatch(fetchStockFormats())
  }, [dispatch])

  useEffect(() => {
    if (!selectedId && assemblies.length > 0) {
      dispatch(selectAssembly(assemblies[0].id))
    }
  }, [assemblies, selectedId, dispatch])

  const selected = assemblies.find((assembly) => assembly.id === selectedId)
  const loading = assembliesStatus === 'loading'

  const previewGeometry = useMemo<AssemblyGeometry>(() => {
    if (draft) {
      return {
        widthMicron: mmToMicron(toNumber(draft.width)),
        heightMicron: mmToMicron(toNumber(draft.height)),
        depthMicron: mmToMicron(toNumber(draft.depth)),
        components: draft.components.map(componentFromDraft),
      }
    }
    if (selected) return geometryFromAssembly(selected)
    return { widthMicron: 0, heightMicron: 0, depthMicron: 0, components: [] }
  }, [draft, selected])

  // Material resolution for the active product (draft wins over the selection).
  const activeMaterialSpecId = draft ? draft.materialSpecId : (selected?.materialSpecId ?? '')
  const activeSpec = materialSpecs.find((spec) => spec.id === activeMaterialSpecId)
  const profile: DimensionProfile = activeSpec?.dimensionProfile ?? '2d'
  const familySpecs = materialSpecs.filter((spec) => spec.materialId === activeSpec?.materialId)

  const matchingFormats = useMemo(() => {
    if (!activeSpec) return []
    return stockFormats.filter((format) => format.specCode === activeSpec.code)
  }, [stockFormats, activeSpec])

  // Components in the shape the solver understands, with material resolved.
  const explodeSource = useMemo<ExplodeComponent[]>(() => {
    if (draft) {
      return draft.components.map((component) => ({
        role: component.role,
        name: component.name,
        quantity: Math.round(toNumber(component.quantity)) || 1,
        materialSpecId: component.materialSpecId || draft.materialSpecId || undefined,
        widthMicron: mmToMicron(toNumber(component.width)),
        heightMicron: mmToMicron(toNumber(component.height)),
        depthMicron: mmToMicron(toNumber(component.depth)),
      }))
    }
    if (selected) {
      return selected.components.map((component) => ({
        role: component.role,
        name: component.name,
        quantity: component.quantity || 1,
        materialSpecId: component.materialSpecId || selected.materialSpecId,
        widthMicron: component.widthMicron,
        heightMicron: component.heightMicron,
        depthMicron: component.depthMicron,
      }))
    }
    return []
  }, [draft, selected])

  const updateComponent = (key: string, patch: Partial<DraftComponent>) =>
    setDraft((current) =>
      current
        ? {
            ...current,
            components: current.components.map((component) =>
              component.key === key ? { ...component, ...patch } : component,
            ),
          }
        : current,
    )

  const addComponent = () =>
    setDraft((current) =>
      current ? { ...current, components: [...current.components, blankComponent()] } : current,
    )

  const removeComponent = (key: string) =>
    setDraft((current) =>
      current
        ? { ...current, components: current.components.filter((component) => component.key !== key) }
        : current,
    )

  const generateTemplate = () => {
    if (!draft) return
    if (template === 'table') {
      const input: TableTemplateInput = {
        widthMm: toNumber(draft.width),
        heightMm: toNumber(draft.height),
        depthMm: toNumber(draft.depth),
        topThicknessMm: toNumber(topThickness),
        legSectionMm: toNumber(legSection),
        apronHeightMm: toNumber(apronHeight),
        apronThicknessMm: toNumber(apronThickness),
        legInsetMm: toNumber(legInset),
        includeShelf,
      }
      if (input.widthMm <= 0 || input.heightMm <= 0 || input.depthMm <= 0) return
      setDraft({ ...draft, components: tableTemplate(input).map(toDraftComponent) })
      return
    }
    const input = {
      widthMm: toNumber(draft.width),
      heightMm: toNumber(draft.height),
      frameSectionMm: toNumber(frameSection),
      frameDepthMm: toNumber(frameDepth),
      glassThicknessMm: toNumber(glassThickness),
      glassInsetMm: toNumber(glassInset),
    }
    if (input.widthMm <= 0 || input.heightMm <= 0 || input.frameSectionMm <= 0) return
    setDraft({ ...draft, components: windowTemplate(input).map(toDraftComponent) })
  }

  const loadSelected = () => {
    if (!selected) return
    setDraft({
      code: `${selected.code}-COPY`,
      name: selected.name,
      kind: selected.kind,
      materialSpecId: selected.materialSpecId ?? '',
      width: mmStr(selected.widthMicron),
      height: mmStr(selected.heightMicron),
      depth: mmStr(selected.depthMicron),
      components: selected.components.map((component) => ({
        key: nextKey(),
        role: component.role,
        kind: component.kind,
        name: component.name,
        materialSpecId: component.materialSpecId ?? '',
        quantity: String(component.quantity || 1),
        width: mmStr(component.widthMicron),
        height: mmStr(component.heightMicron),
        depth: mmStr(component.depthMicron),
        offsetX: mmStr(component.offsetXMicron),
        offsetY: mmStr(component.offsetYMicron),
        offsetZ: mmStr(component.offsetZMicron),
      })),
    })
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!draft || !draft.code.trim()) return

    const widthMicron = mmToMicron(toNumber(draft.width))
    const heightMicron = mmToMicron(toNumber(draft.height))
    if (widthMicron <= 0 && heightMicron <= 0) return

    const input: CreateAssemblyInput = {
      code: draft.code.trim(),
      name: draft.name.trim() || undefined,
      kind: draft.kind,
      materialSpecId: draft.materialSpecId || undefined,
      widthMicron,
      heightMicron,
      depthMicron: mmToMicron(toNumber(draft.depth)),
      components: draft.components.map((component, index) => ({
        seq: index + 1,
        role: component.role.trim() || undefined,
        kind: component.kind,
        name: component.name.trim() || undefined,
        materialSpecId: component.materialSpecId.trim() || undefined,
        quantity: Math.round(toNumber(component.quantity)) || 1,
        widthMicron: mmToMicron(toNumber(component.width)),
        heightMicron: mmToMicron(toNumber(component.height)),
        depthMicron: mmToMicron(toNumber(component.depth)),
        offsetXMicron: mmToMicron(toNumber(component.offsetX)),
        offsetYMicron: mmToMicron(toNumber(component.offsetY)),
        offsetZMicron: mmToMicron(toNumber(component.offsetZ)),
      })),
    }

    const action = await dispatch(createAssembly(input))
    if (createAssembly.fulfilled.match(action)) {
      setDraft(blankDraft())
    }
  }

  // Explode the product into cut parts, plan them against matching stock, and
  // open the result in the plan viewer.
  const runCutPlan = async () => {
    if (!activeSpec || explodeSource.length === 0 || matchingFormats.length === 0) return

    const parts = explodeComponents({
      components: explodeSource,
      profile,
      productQuantity: Math.round(toNumber(productQuantity)) || 1,
      fallbackMaterialSpecId: activeSpec.id,
    })
    const stocks: StockItem[] = matchingFormats.map((format) =>
      profile === '1d'
        ? {
            id: format.id,
            code: format.code,
            length: format.lengthMicron,
            ...(format.widthMicron > 0 ? { width: format.widthMicron } : {}),
            quantity: Math.max(format.onHandQty, 1),
            costPerUnit: format.costPerUnit,
          }
        : {
            id: format.id,
            code: format.code,
            width: format.widthMicron,
            height: format.heightMicron,
            quantity: Math.max(format.onHandQty, 1),
            costPerUnit: format.costPerUnit,
          },
    )
    const problem: Problem = { parts, stocks, budgetMs: 5000 }

    const action = await dispatch(optimizeProblem({ problem, dimension: profile }))
    if (optimizeProblem.fulfilled.match(action)) {
      navigate('/viewer')
    }
  }

  const previewComponents = previewGeometry.components
  const canRun =
    !!activeSpec && explodeSource.length > 0 && matchingFormats.length > 0 && runStatus !== 'loading'

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Products</h1>
          <p className="text-sm text-muted-foreground">
            Design a product (a window, a table) from a material and its type, then plan its cut
            parts against that material's stock. Components are boxes in the product's local frame:
            x right, y up, z out of the wall.
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => {
            dispatch(fetchAssemblies())
            dispatch(fetchMaterials())
            dispatch(fetchMaterialSpecs())
            dispatch(fetchStockFormats())
          }}
          disabled={loading}
        >
          <RefreshCw className="mr-2 h-4 w-4" />
          {loading ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_420px]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Catalog</CardTitle>
              <CardDescription>{assemblies.length} product(s) from /api/v1/assemblies</CardDescription>
            </CardHeader>
            <CardContent>
              {assembliesError ? (
                <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                  <div className="font-medium text-destructive">Database not reachable</div>
                  <p className="mt-1 text-muted-foreground">{assembliesError}</p>
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
                      <TableHead>Kind</TableHead>
                      <TableHead>Material</TableHead>
                      <TableHead>Overall (W × H × D)</TableHead>
                      <TableHead className="text-right">Subparts</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {assemblies.map((assembly) => {
                      const spec = materialSpecs.find((item) => item.id === assembly.materialSpecId)
                      return (
                        <TableRow
                          key={assembly.id}
                          onClick={() => {
                            dispatch(selectAssembly(assembly.id))
                            setDraft(null)
                          }}
                          className={`cursor-pointer ${
                            !draft && assembly.id === selectedId ? 'bg-muted/60' : ''
                          }`}
                        >
                          <TableCell className="font-medium">
                            {assembly.code}
                            {assembly.name && (
                              <div className="text-xs text-muted-foreground">{assembly.name}</div>
                            )}
                          </TableCell>
                          <TableCell>
                            <Badge variant="secondary">{assembly.kind}</Badge>
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">
                            {spec ? `${spec.materialName} · ${spec.name || spec.code}` : '—'}
                          </TableCell>
                          <TableCell>
                            {micronToMm(assembly.widthMicron)} × {micronToMm(assembly.heightMicron)} ×{' '}
                            {micronToMm(assembly.depthMicron)} mm
                          </TableCell>
                          <TableCell className="text-right">{assembly.components.length}</TableCell>
                        </TableRow>
                      )
                    })}
                    {assemblies.length === 0 && !loading && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center text-sm text-muted-foreground">
                          No products yet. Run db/scripts/seed.ps1 or build one below.
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              )}
              {selected && !draft && (
                <div className="mt-3 flex justify-end">
                  <Button variant="outline" size="sm" onClick={loadSelected}>
                    Duplicate into builder
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {draft ? (
            <Card>
              <CardHeader>
                <CardTitle>Build product</CardTitle>
                <CardDescription>
                  POST /api/v1/assemblies — sizes in mm, material from the catalog
                </CardDescription>
              </CardHeader>
              <CardContent>
                <form className="space-y-4" onSubmit={submit}>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field label="Code">
                      <Input
                        value={draft.code}
                        onChange={(event) => setDraft({ ...draft, code: event.target.value })}
                        placeholder="TABLE-1600x800"
                        required
                      />
                    </Field>
                    <Field label="Name">
                      <Input
                        value={draft.name}
                        onChange={(event) => setDraft({ ...draft, name: event.target.value })}
                        placeholder="Dining table"
                      />
                    </Field>
                    <Field label="Kind">
                      <Select
                        value={draft.kind}
                        onValueChange={(value) => setDraft({ ...draft, kind: value as AssemblyKind })}
                      >
                        <SelectTrigger className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="window">window</SelectItem>
                          <SelectItem value="door">door</SelectItem>
                          <SelectItem value="generic">generic</SelectItem>
                        </SelectContent>
                      </Select>
                    </Field>
                    <div className="grid grid-cols-2 gap-2">
                      <Field label="Material">
                        <Select
                          value={activeSpec?.materialId ?? 'none'}
                          onValueChange={(value) => {
                            const first = materialSpecs.find((spec) => spec.materialId === value)
                            setDraft({ ...draft, materialSpecId: first?.id ?? '' })
                          }}
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue placeholder="Choose…" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="none">None</SelectItem>
                            {materials.map((material) => (
                              <SelectItem key={material.id} value={material.id}>
                                {material.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </Field>
                      <Field label="Type">
                        <Select
                          value={draft.materialSpecId || 'none'}
                          onValueChange={(value) =>
                            setDraft({ ...draft, materialSpecId: value === 'none' ? '' : value })
                          }
                          disabled={!activeSpec}
                        >
                          <SelectTrigger className="w-full">
                            <SelectValue placeholder="Choose…" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="none">None</SelectItem>
                            {familySpecs.map((spec) => (
                              <SelectItem key={spec.id} value={spec.id}>
                                {spec.name || spec.code}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </Field>
                    </div>
                  </div>

                  <div className="grid grid-cols-3 gap-2">
                    <Field label="Width (mm)">
                      <Input
                        type="number"
                        min="1"
                        step="1"
                        value={draft.width}
                        onChange={(event) => setDraft({ ...draft, width: event.target.value })}
                        placeholder="1600"
                        required
                      />
                    </Field>
                    <Field label="Height (mm)">
                      <Input
                        type="number"
                        min="1"
                        step="1"
                        value={draft.height}
                        onChange={(event) => setDraft({ ...draft, height: event.target.value })}
                        placeholder="750"
                        required
                      />
                    </Field>
                    <Field label="Depth (mm)">
                      <Input
                        type="number"
                        min="0"
                        step="1"
                        value={draft.depth}
                        onChange={(event) => setDraft({ ...draft, depth: event.target.value })}
                        placeholder="800"
                      />
                    </Field>
                  </div>

                  <div className="rounded-md border border-dashed border-border p-3">
                    <div className="mb-2 flex items-center gap-2 text-sm font-medium">
                      <Wand2 className="h-4 w-4" />
                      Template
                    </div>
                    <Field label="Shape">
                      <Select
                        value={template}
                        onValueChange={(value) => setTemplate(value as 'window' | 'table')}
                      >
                        <SelectTrigger className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="window">Window — frame + glass</SelectItem>
                          <SelectItem value="table">Table — top + legs + aprons</SelectItem>
                        </SelectContent>
                      </Select>
                    </Field>

                    {template === 'table' ? (
                      <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
                        <Field label="Top thickness">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={topThickness}
                            onChange={(event) => setTopThickness(event.target.value)}
                          />
                        </Field>
                        <Field label="Leg section">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={legSection}
                            onChange={(event) => setLegSection(event.target.value)}
                          />
                        </Field>
                        <Field label="Apron height">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={apronHeight}
                            onChange={(event) => setApronHeight(event.target.value)}
                          />
                        </Field>
                        <Field label="Apron thickness">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={apronThickness}
                            onChange={(event) => setApronThickness(event.target.value)}
                          />
                        </Field>
                        <Field label="Leg inset">
                          <Input
                            type="number"
                            min="0"
                            step="1"
                            value={legInset}
                            onChange={(event) => setLegInset(event.target.value)}
                          />
                        </Field>
                        <Field label="Shelf">
                          <Select
                            value={includeShelf ? 'yes' : 'no'}
                            onValueChange={(value) => setIncludeShelf(value === 'yes')}
                          >
                            <SelectTrigger className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="yes">With shelf</SelectItem>
                              <SelectItem value="no">No shelf</SelectItem>
                            </SelectContent>
                          </Select>
                        </Field>
                      </div>
                    ) : (
                      <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                        <Field label="Frame section">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={frameSection}
                            onChange={(event) => setFrameSection(event.target.value)}
                          />
                        </Field>
                        <Field label="Frame depth">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={frameDepth}
                            onChange={(event) => setFrameDepth(event.target.value)}
                          />
                        </Field>
                        <Field label="Glass thickness">
                          <Input
                            type="number"
                            min="1"
                            step="1"
                            value={glassThickness}
                            onChange={(event) => setGlassThickness(event.target.value)}
                          />
                        </Field>
                        <Field label="Glass inset">
                          <Input
                            type="number"
                            min="0"
                            step="1"
                            value={glassInset}
                            onChange={(event) => setGlassInset(event.target.value)}
                          />
                        </Field>
                      </div>
                    )}
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      className="mt-2"
                      onClick={generateTemplate}
                    >
                      Generate from overall size
                    </Button>
                  </div>

                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium">Subparts</span>
                      <Button type="button" variant="outline" size="sm" onClick={addComponent}>
                        <Plus className="mr-1 h-4 w-4" />
                        Add subpart
                      </Button>
                    </div>

                    {draft.components.length === 0 && (
                      <p className="rounded-md border border-border p-3 text-xs text-muted-foreground">
                        No subparts yet. Generate a template or add subparts by hand.
                      </p>
                    )}

                    {draft.components.map((component, index) => (
                      <div
                        key={component.key}
                        className="space-y-2 rounded-md border border-border p-2"
                      >
                        <div className="grid grid-cols-[1fr_1fr_110px_80px_auto] items-end gap-2">
                          <Field label={`Role · ${index + 1}`}>
                            <Input
                              value={component.role}
                              onChange={(event) =>
                                updateComponent(component.key, { role: event.target.value })
                              }
                              placeholder="leg"
                            />
                          </Field>
                          <Field label="Name">
                            <Input
                              value={component.name}
                              onChange={(event) =>
                                updateComponent(component.key, { name: event.target.value })
                              }
                              placeholder="Leg"
                            />
                          </Field>
                          <Field label="Kind">
                            <Select
                              value={component.kind}
                              onValueChange={(value) =>
                                updateComponent(component.key, { kind: value as ComponentKind })
                              }
                            >
                              <SelectTrigger className="w-full">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="beam">beam</SelectItem>
                                <SelectItem value="panel">panel</SelectItem>
                                <SelectItem value="custom">custom</SelectItem>
                              </SelectContent>
                            </Select>
                          </Field>
                          <Field label="Qty">
                            <Input
                              type="number"
                              min="1"
                              step="1"
                              value={component.quantity}
                              onChange={(event) =>
                                updateComponent(component.key, { quantity: event.target.value })
                              }
                            />
                          </Field>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            onClick={() => removeComponent(component.key)}
                            aria-label="Remove subpart"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                        <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                          <Field label="W mm">
                            <Input
                              type="number"
                              min="0"
                              step="1"
                              value={component.width}
                              onChange={(event) =>
                                updateComponent(component.key, { width: event.target.value })
                              }
                            />
                          </Field>
                          <Field label="H mm">
                            <Input
                              type="number"
                              min="0"
                              step="1"
                              value={component.height}
                              onChange={(event) =>
                                updateComponent(component.key, { height: event.target.value })
                              }
                            />
                          </Field>
                          <Field label="D mm">
                            <Input
                              type="number"
                              min="0"
                              step="1"
                              value={component.depth}
                              onChange={(event) =>
                                updateComponent(component.key, { depth: event.target.value })
                              }
                            />
                          </Field>
                          <Field label="X mm">
                            <Input
                              type="number"
                              step="1"
                              value={component.offsetX}
                              onChange={(event) =>
                                updateComponent(component.key, { offsetX: event.target.value })
                              }
                            />
                          </Field>
                          <Field label="Y mm">
                            <Input
                              type="number"
                              step="1"
                              value={component.offsetY}
                              onChange={(event) =>
                                updateComponent(component.key, { offsetY: event.target.value })
                              }
                            />
                          </Field>
                          <Field label="Z mm">
                            <Input
                              type="number"
                              step="1"
                              value={component.offsetZ}
                              onChange={(event) =>
                                updateComponent(component.key, { offsetZ: event.target.value })
                              }
                            />
                          </Field>
                        </div>
                        <Field label="Material (optional — inherits the product)">
                          <Select
                            value={component.materialSpecId || 'inherit'}
                            onValueChange={(value) =>
                              updateComponent(component.key, {
                                materialSpecId: value === 'inherit' ? '' : value,
                              })
                            }
                          >
                            <SelectTrigger className="w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="inherit">
                                Inherit product material
                              </SelectItem>
                              {materialSpecs.map((spec) => (
                                <SelectItem key={spec.id} value={spec.id}>
                                  {spec.materialName} · {spec.name || spec.code}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </Field>
                      </div>
                    ))}
                  </div>

                  <div className="flex gap-2">
                    <Button type="submit" disabled={createAssemblyStatus === 'loading'}>
                      <Plus className="mr-2 h-4 w-4" />
                      {createAssemblyStatus === 'loading' ? 'Creating…' : 'Create product'}
                    </Button>
                    <Button type="button" variant="ghost" onClick={() => setDraft(null)}>
                      Cancel
                    </Button>
                  </div>
                  {createAssemblyError && (
                    <p className="text-xs text-destructive">{createAssemblyError}</p>
                  )}
                </form>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardHeader>
                <CardTitle>New product</CardTitle>
                <CardDescription>Build a table, window or any product from subparts</CardDescription>
              </CardHeader>
              <CardContent>
                <Button onClick={() => setDraft(blankDraft())}>
                  <Plus className="mr-2 h-4 w-4" />
                  New product
                </Button>
              </CardContent>
            </Card>
          )}
        </div>

        <div className="space-y-4 self-start lg:sticky lg:top-4">
          <Card>
            <CardHeader>
              <CardTitle>{draft ? 'Draft preview' : (selected?.code ?? 'Preview')}</CardTitle>
              <CardDescription>
                {draft
                  ? `${draft.components.length} subpart(s) in the builder`
                  : 'Select a product from the catalog'}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <AssemblyViewer geometry={previewGeometry} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Cut plan</CardTitle>
              <CardDescription>
                {activeSpec
                  ? `${activeSpec.materialName} · ${activeSpec.name || activeSpec.code} (${profile})`
                  : 'Choose a material and type for the product'}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-end gap-2">
                <Field label="Products to make">
                  <Input
                    type="number"
                    min="1"
                    step="1"
                    value={productQuantity}
                    onChange={(event) => setProductQuantity(event.target.value)}
                  />
                </Field>
                <Button onClick={runCutPlan} disabled={!canRun}>
                  <Scissors className="mr-2 h-4 w-4" />
                  {runStatus === 'loading' ? 'Planning…' : 'Run cut plan'}
                </Button>
              </div>
              <ul className="space-y-1 text-xs text-muted-foreground">
                <li>{explodeSource.length} subpart(s) to cut</li>
                <li>
                  {matchingFormats.length} stock format(s) matching{' '}
                  {activeSpec ? activeSpec.name || activeSpec.code : '—'}
                </li>
              </ul>
              {!activeSpec && (
                <p className="text-xs text-muted-foreground">
                  Pick a material on the Stock or Materials page first, then choose it here.
                </p>
              )}
              {activeSpec && matchingFormats.length === 0 && (
                <p className="text-xs text-destructive">
                  No stock for this type yet. Add a stock format on the Stock page.
                </p>
              )}
              {runError && <p className="text-xs text-destructive">{runError}</p>}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Subparts</CardTitle>
              <CardDescription>
                {previewComponents.length} component(s) · origin bottom-left-front
              </CardDescription>
            </CardHeader>
            <CardContent>
              {previewComponents.length === 0 ? (
                <p className="text-sm text-muted-foreground">No components to list.</p>
              ) : (
                <ul className="space-y-2 text-sm">
                  {previewComponents.map((component) => (
                    <li key={component.id} className="flex items-start gap-2">
                      <span
                        className="mt-0.5 h-3 w-3 shrink-0 rounded-sm border border-border"
                        style={{ background: COMPONENT_COLORS[component.kind] }}
                      />
                      <div className="min-w-0">
                        <div className="truncate font-medium">{componentLabel(component)}</div>
                        <div className="text-xs text-muted-foreground">
                          {micronToMm(component.widthMicron)} × {micronToMm(component.heightMicron)} ×{' '}
                          {micronToMm(component.depthMicron)} mm · {component.role}
                        </div>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</span>
      {children}
    </label>
  )
}
