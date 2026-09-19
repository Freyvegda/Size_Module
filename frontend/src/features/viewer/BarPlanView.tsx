import { micronToMm } from '@/lib/format'
import type { SheetPlan } from '@/lib/types'
import { cn } from '@/lib/utils'

import { partColor } from './partColor'

export interface BarPlanViewProps {
  sheets: SheetPlan[]
  selectedSheet: number
  selectedPartId?: string
  showOffcuts: boolean
  onSelectSheet: (index: number) => void
  onSelectPart: (partId?: string) => void
}

/**
 * 1D plans (bars, profiles, tubes) come back as `SheetPlan`s whose width is the
 * bar length and whose height is the profile height. Rendering them through the
 * 3D sheet scene would show paper-thin slivers, so bars get their own horizontal
 * track view: every bar is one row of coloured segments.
 */
export function BarPlanView({
  sheets,
  selectedSheet,
  selectedPartId,
  showOffcuts,
  onSelectSheet,
  onSelectPart,
}: BarPlanViewProps) {
  return (
    <div className="max-h-[520px] space-y-3 overflow-y-auto p-4">
      {sheets.map((sheet) => {
        const active = sheet.index === selectedSheet
        const pct = (value: number) => `${(value / sheet.width) * 100}%`
        return (
          <div
            key={sheet.index}
            role="button"
            tabIndex={0}
            onClick={() => onSelectSheet(sheet.index)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                onSelectSheet(sheet.index)
              }
            }}
            className={cn(
              'w-full cursor-pointer rounded-md border p-3 text-left transition-all duration-300 ease-out',
              active
                ? 'border-sky-500/60 bg-sky-500/5 shadow-[0_0_0_1px_rgba(56,189,248,0.35)]'
                : 'border-border hover:-translate-y-px hover:bg-muted/50',
            )}
          >
            <div className="mb-2 flex items-center justify-between gap-3 text-xs">
              <span className="font-medium">{sheet.label || sheet.stockCode}</span>
              <span className="text-muted-foreground">
                {(sheet.placements ?? []).length} pieces · {micronToMm(sheet.width)} mm
              </span>
            </div>

            <div className="relative h-12 w-full overflow-hidden rounded border border-border bg-slate-950/60">
              {showOffcuts &&
                (sheet.offcuts ?? []).map((off, index) => (
                  <div
                    key={`offcut-${index}`}
                    className="absolute inset-y-0 border-x border-emerald-400/70 bg-emerald-500/20"
                    style={{ left: pct(off.x), width: pct(off.w) }}
                    title={`offcut ${micronToMm(off.w)} mm`}
                  />
                ))}

              {(sheet.placements ?? []).map((placement, index) => {
                const selected = placement.partId === selectedPartId
                const labelFits = placement.w / sheet.width > 0.07
                return (
                  <button
                    key={`${placement.partId}-${index}`}
                    type="button"
                    onClick={(event) => {
                      event.stopPropagation()
                      onSelectPart(selected ? undefined : placement.partId)
                    }}
                    className={cn(
                      'absolute inset-y-0 flex items-center justify-center overflow-hidden border-r border-black/30 text-[10px] font-medium text-white/90 transition-[filter,box-shadow] duration-300',
                      active && 'brightness-110',
                      selected && 'z-10 ring-2 ring-amber-400',
                    )}
                    style={{
                      left: pct(placement.x),
                      width: pct(placement.w),
                      background: partColor(placement.partCode, placement.priority),
                    }}
                    title={`${placement.partCode} · ${micronToMm(placement.w)} mm`}
                  >
                    {labelFits && <span className="truncate px-1">{placement.partCode}</span>}
                  </button>
                )
              })}
            </div>

            <div className="mt-1.5 flex justify-between text-[10px] text-muted-foreground">
              <span>0</span>
              <span>{micronToMm(sheet.width)} mm</span>
            </div>
          </div>
        )
      })}
    </div>
  )
}
