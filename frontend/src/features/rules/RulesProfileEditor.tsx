import { useState, type ReactNode } from 'react'
import { Save, X } from 'lucide-react'

import { Button } from '@/components/ui/button'
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
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { createRulesProfile, updateRulesProfile } from '@/features/rules/rulesSlice'
import { mmToMicron, micronToMm } from '@/lib/format'
import { defaultRules, defaultWeights } from '@/lib/rules'
import type { CutMode, GrainMode, RulesProfile, SaveRulesProfileInput } from '@/lib/types'

interface RulesProfileEditorProps {
  /** Omit to create a new profile; `key` on the element resets the draft. */
  profile?: RulesProfile
  onDone: () => void
}

/** Editor state keeps raw strings so typing decimals does not fight the cursor. */
interface Draft {
  code: string
  name: string
  isDefault: boolean
  cutMode: CutMode
  grainMode: GrainMode
  allowRotate: boolean
  preferRemnants: boolean
  kerfMm: string
  trimMm: string
  offcutMinWMm: string
  offcutMinHMm: string
  offcutMinLengthMm: string
  minPartDimMm: string
  maxCutStages: string
  maxPartsPerSheet: string
  oversPct: string
  fillPriority: string
  minSheets: string
  minScrap: string
  minPatterns: string
  minOffcutArea: string
  cost: string
  priorities: string
}

function toDraft(profile?: RulesProfile): Draft {
  const rules = profile?.rules ?? defaultRules
  const weights = profile?.objective?.weights ?? defaultWeights
  return {
    code: profile?.code ?? '',
    name: profile?.name ?? '',
    isDefault: profile?.isDefault ?? false,
    cutMode: rules.cutMode ?? 'guillotine',
    grainMode: rules.grainMode ?? 'none',
    allowRotate: rules.allowRotate ?? true,
    preferRemnants: rules.preferRemnants ?? true,
    kerfMm: micronToMm(rules.kerf),
    trimMm: micronToMm(rules.trim),
    offcutMinWMm: micronToMm(rules.offcutMinW),
    offcutMinHMm: micronToMm(rules.offcutMinH),
    offcutMinLengthMm: micronToMm(rules.offcutMinLength),
    minPartDimMm: micronToMm(rules.minPartDim),
    maxCutStages: String(rules.maxCutStages ?? 0),
    maxPartsPerSheet: String(rules.maxPartsPerSheet ?? 0),
    oversPct: String((rules.oversAllowedPct ?? 0) * 100),
    fillPriority: String(weights.fillPriority ?? 0),
    minSheets: String(weights.minSheets ?? 0),
    minScrap: String(weights.minScrap ?? 0),
    minPatterns: String(weights.minPatterns ?? 0),
    minOffcutArea: String(weights.minOffcutArea ?? 0),
    cost: String(weights.cost ?? 0),
    priorities: (profile?.objective?.priorities ?? []).join(', '),
  }
}

function toPayload(draft: Draft): SaveRulesProfileInput {
  const num = (value: string) => Number(value) || 0
  return {
    code: draft.code.trim(),
    name: draft.name.trim(),
    isDefault: draft.isDefault,
    rules: {
      kerf: mmToMicron(num(draft.kerfMm)),
      trim: mmToMicron(num(draft.trimMm)),
      allowRotate: draft.allowRotate,
      grainMode: draft.grainMode,
      cutMode: draft.cutMode,
      maxCutStages: Math.round(num(draft.maxCutStages)),
      offcutMinW: mmToMicron(num(draft.offcutMinWMm)),
      offcutMinH: mmToMicron(num(draft.offcutMinHMm)),
      offcutMinLength: mmToMicron(num(draft.offcutMinLengthMm)),
      minPartDim: mmToMicron(num(draft.minPartDimMm)),
      maxPartsPerSheet: Math.round(num(draft.maxPartsPerSheet)),
      oversAllowedPct: num(draft.oversPct) / 100,
      preferRemnants: draft.preferRemnants,
    },
    objective: {
      weights: {
        fillPriority: num(draft.fillPriority),
        minSheets: num(draft.minSheets),
        minScrap: num(draft.minScrap),
        minPatterns: num(draft.minPatterns),
        minOffcutArea: num(draft.minOffcutArea),
        cost: num(draft.cost),
      },
      priorities: draft.priorities
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean),
    },
  }
}

export function RulesProfileEditor({ profile, onDone }: RulesProfileEditorProps) {
  const dispatch = useAppDispatch()
  const { saveStatus, saveError } = useAppSelector((state) => state.rules)
  const [draft, setDraft] = useState<Draft>(() => toDraft(profile))

  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((current) => ({ ...current, [key]: value }))

  const save = async () => {
    const payload = toPayload(draft)
    const action = profile
      ? await dispatch(updateRulesProfile({ id: profile.id, input: payload }))
      : await dispatch(createRulesProfile(payload))
    if (
      createRulesProfile.fulfilled.match(action) ||
      updateRulesProfile.fulfilled.match(action)
    ) {
      onDone()
    }
  }

  const valid = draft.code.trim().length > 0

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Code" hint={profile ? 'Immutable' : 'e.g. GLASS-DEFAULT'}>
          <Input
            value={draft.code}
            disabled={Boolean(profile)}
            onChange={(event) => set('code', event.target.value)}
          />
        </Field>
        <Field label="Name">
          <Input
            placeholder="Glass: guillotine, 4 mm kerf"
            value={draft.name}
            onChange={(event) => set('name', event.target.value)}
          />
        </Field>
      </div>

      <div className="flex items-center justify-between rounded-md border border-border p-3">
        <div>
          <div className="text-sm font-medium">Plant default</div>
          <p className="text-xs text-muted-foreground">
            Used when a job/optimization does not name a profile.
          </p>
        </div>
        <Switch
          checked={draft.isDefault}
          onCheckedChange={(checked) => set('isDefault', checked)}
        />
      </div>

      <section className="space-y-3">
        <h3 className="text-sm font-medium">Constraints</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Cut mode">
            <Select value={draft.cutMode} onValueChange={(value) => set('cutMode', value as CutMode)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="guillotine">Guillotine (saw/glass)</SelectItem>
                <SelectItem value="free">Free cutting (CNC/router)</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field label="Grain mode">
            <Select
              value={draft.grainMode}
              onValueChange={(value) => set('grainMode', value as GrainMode)}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">None</SelectItem>
                <SelectItem value="along_x">Along X</SelectItem>
                <SelectItem value="along_y">Along Y</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <NumberField label="Kerf (mm)" value={draft.kerfMm} onChange={(v) => set('kerfMm', v)} />
          <NumberField label="Edge trim (mm)" value={draft.trimMm} onChange={(v) => set('trimMm', v)} />
          <NumberField
            label="Offcut min width (mm)"
            value={draft.offcutMinWMm}
            onChange={(v) => set('offcutMinWMm', v)}
          />
          <NumberField
            label="Offcut min height (mm)"
            value={draft.offcutMinHMm}
            onChange={(v) => set('offcutMinHMm', v)}
          />
          <NumberField
            label="Offcut min length (mm)"
            value={draft.offcutMinLengthMm}
            onChange={(v) => set('offcutMinLengthMm', v)}
          />
          <NumberField
            label="Min part dimension (mm)"
            value={draft.minPartDimMm}
            onChange={(v) => set('minPartDimMm', v)}
          />
          <NumberField
            label="Max cut stages"
            hint="0 = unlimited"
            value={draft.maxCutStages}
            onChange={(v) => set('maxCutStages', v)}
          />
          <NumberField
            label="Max parts per sheet"
            hint="0 = unlimited"
            value={draft.maxPartsPerSheet}
            onChange={(v) => set('maxPartsPerSheet', v)}
          />
          <NumberField
            label="Overs allowed (%)"
            value={draft.oversPct}
            onChange={(v) => set('oversPct', v)}
          />
        </div>
        <div className="flex items-center justify-between rounded-md border border-border p-3">
          <div>
            <div className="text-sm font-medium">Allow rotation</div>
            <p className="text-xs text-muted-foreground">Permit 90°/180° part rotation.</p>
          </div>
          <Switch
            checked={draft.allowRotate}
            onCheckedChange={(checked) => set('allowRotate', checked)}
          />
        </div>
        <div className="flex items-center justify-between rounded-md border border-border p-3">
          <div>
            <div className="text-sm font-medium">Prefer remnants</div>
            <p className="text-xs text-muted-foreground">
              Offer paid-for leftovers before fresh stock.
            </p>
          </div>
          <Switch
            checked={draft.preferRemnants}
            onCheckedChange={(checked) => set('preferRemnants', checked)}
          />
        </div>
      </section>

      <section className="space-y-3">
        <h3 className="text-sm font-medium">Objective weights</h3>
        <p className="text-xs text-muted-foreground">
          All terms are minimised except fill priority. Defaults come straight from the engine.
        </p>
        <div className="grid gap-3 sm:grid-cols-3">
          <NumberField
            label="Fill priority"
            value={draft.fillPriority}
            onChange={(v) => set('fillPriority', v)}
          />
          <NumberField label="Min sheets" value={draft.minSheets} onChange={(v) => set('minSheets', v)} />
          <NumberField label="Min scrap" value={draft.minScrap} onChange={(v) => set('minScrap', v)} />
          <NumberField
            label="Min patterns"
            value={draft.minPatterns}
            onChange={(v) => set('minPatterns', v)}
          />
          <NumberField
            label="Min offcut area"
            value={draft.minOffcutArea}
            onChange={(v) => set('minOffcutArea', v)}
          />
          <NumberField label="Cost" value={draft.cost} onChange={(v) => set('cost', v)} />
        </div>
        <Field label="Priorities" hint="Comma separated, e.g. fillPriority, minSheets">
          <Input
            placeholder="fillPriority"
            value={draft.priorities}
            onChange={(event) => set('priorities', event.target.value)}
          />
        </Field>
      </section>

      {saveError && <p className="text-sm text-destructive">{saveError}</p>}

      <div className="flex items-center gap-2">
        <Button onClick={save} disabled={!valid || saveStatus === 'loading'}>
          <Save className="mr-2 h-4 w-4" />
          {saveStatus === 'loading' ? 'Saving…' : profile ? 'Save changes' : 'Create profile'}
        </Button>
        <Button variant="outline" onClick={onDone}>
          <X className="mr-2 h-4 w-4" />
          Cancel
        </Button>
      </div>
    </div>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <Label>{label}</Label>
        {hint && <span className="text-xs text-muted-foreground">{hint}</span>}
      </div>
      {children}
    </div>
  )
}

function NumberField({
  label,
  hint,
  value,
  onChange,
}: {
  label: string
  hint?: string
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Field label={label} hint={hint}>
      <Input
        inputMode="decimal"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  )
}
