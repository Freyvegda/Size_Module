import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { ExternalLink, RefreshCw } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { loadPlanDetail } from '@/features/optimizer/optimizerSlice'
import { fetchPlans } from '@/features/plans/plansSlice'
import { percent } from '@/lib/format'
import type { PlanStatus } from '@/lib/types'

const statusVariant: Record<PlanStatus, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  draft: 'secondary',
  approved: 'default',
  accepted: 'default',
  archived: 'outline',
}

const exportFormats = ['csv', 'svg', 'dxf', 'pdf'] as const

export function PlansPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { plans, status, error } = useAppSelector((state) => state.plans)

  useEffect(() => {
    void dispatch(fetchPlans())
  }, [dispatch])

  const open = (planId: string) => {
    void dispatch(loadPlanDetail(planId))
      .unwrap()
      .then(() => navigate('/viewer'))
      .catch(() => {
        // The optimizer slice records the error.
      })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Plans</h1>
          <p className="text-sm text-muted-foreground">
            Every archived plan version, newest first. Open one to inspect, edit, accept or export
            it.
          </p>
        </div>
        <Button variant="outline" onClick={() => dispatch(fetchPlans())} disabled={status === 'loading'}>
          <RefreshCw className="mr-2 h-4 w-4" />
          {status === 'loading' ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Archive</CardTitle>
          <CardDescription>{plans.length} plan(s) from /api/v1/plans</CardDescription>
        </CardHeader>
        <CardContent>
          {error ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
              <div className="font-medium text-destructive">Database not reachable</div>
              <p className="mt-1 text-muted-foreground">{error}</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Created</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Ver.</TableHead>
                  <TableHead>Solver</TableHead>
                  <TableHead className="text-right">Sheets</TableHead>
                  <TableHead className="text-right">Parts</TableHead>
                  <TableHead className="text-right">Yield</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {plans.map((plan) => (
                  <TableRow key={plan.id}>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(plan.createdAt).toLocaleString()}
                    </TableCell>
                    <TableCell className="font-medium">{plan.name || '—'}</TableCell>
                    <TableCell>
                      <Badge variant={statusVariant[plan.status]}>{plan.status}</Badge>
                    </TableCell>
                    <TableCell className="text-right">{plan.version}</TableCell>
                    <TableCell className="text-xs">{plan.solver}</TableCell>
                    <TableCell className="text-right">{plan.metrics.sheetCount}</TableCell>
                    <TableCell className="text-right">
                      {plan.metrics.partsPlaced}/{plan.metrics.partsRequested}
                    </TableCell>
                    <TableCell className="text-right">{percent(plan.metrics.yieldPct)}</TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-1">
                        <Button size="sm" variant="ghost" onClick={() => open(plan.id)}>
                          <ExternalLink className="mr-1 h-3.5 w-3.5" />
                          Open
                        </Button>
                        {exportFormats.map((format) => (
                          <a
                            key={format}
                            className="text-xs uppercase text-muted-foreground hover:text-foreground"
                            href={`/api/v1/plans/${plan.id}/exports?format=${format}`}
                          >
                            {format}
                          </a>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
                {plans.length === 0 && status !== 'loading' && (
                  <TableRow>
                    <TableCell colSpan={9} className="text-center text-sm text-muted-foreground">
                      No plans yet. Run an optimization to archive one.
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
