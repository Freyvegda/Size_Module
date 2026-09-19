import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { SolverCapabilities } from '@/lib/types'

/** Capability chips for a solver, mirroring core.Capabilities from the API. */
export function SolverBadges({
  capabilities,
  className,
}: {
  capabilities: SolverCapabilities
  className?: string
}) {
  const rank =
    capabilities.Rank > 0 ? `rank ${capabilities.Rank}` : 'unranked'
  return (
    <div className={cn('flex flex-wrap items-center gap-1.5', className)}>
      <Badge variant="secondary">{capabilities.Dimension}</Badge>
      <Badge variant={capabilities.CutMode === 'free' ? 'default' : 'outline'}>
        {capabilities.CutMode}
      </Badge>
      <Badge variant="outline">{rank}</Badge>
      {capabilities.Rotation && <Badge variant="ghost">rotation</Badge>}
      {capabilities.Grain && <Badge variant="ghost">grain</Badge>}
      {capabilities.Remnants && <Badge variant="ghost">remnants</Badge>}
      {capabilities.MaxParts > 0 && <Badge variant="ghost">max {capabilities.MaxParts}</Badge>}
    </div>
  )
}
