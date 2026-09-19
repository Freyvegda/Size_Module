import { useEffect } from 'react'
import { CheckCircle2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { fetchStockItems } from '@/features/catalog/catalogSlice'
import {
  acceptPlan,
  loadBarDemoPlan,
  loadDemoPlan,
} from '@/features/optimizer/optimizerSlice'
import { PlanViewer } from '@/features/viewer/PlanViewer'

export function PlanViewerPage() {
  const dispatch = useAppDispatch()
  const result = useAppSelector((state) => state.optimizer.result)
  const { lastPlanId, acceptStatus, acceptResult, acceptError } = useAppSelector(
    (state) => state.optimizer,
  )

  useEffect(() => {
    if (!result) {
      void dispatch(loadDemoPlan())
    }
  }, [dispatch, result])

  const accept = () => {
    if (!lastPlanId) return
    void dispatch(acceptPlan(lastPlanId)).then((action) => {
      if (acceptPlan.fulfilled.match(action)) {
        // The pool changed: keep the stock page in sync.
        void dispatch(fetchStockItems({ status: 'available' }))
      }
    })
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Plan viewer</h1>
          <p className="text-sm text-muted-foreground">
            2D plans get an orthographic drawing and an exploded 3D stack; 1D plans render as bar
            tracks. Click a piece to inspect it.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => dispatch(loadDemoPlan())}>
            Load 2D demo
          </Button>
          <Button variant="outline" onClick={() => dispatch(loadBarDemoPlan())}>
            Load 1D bar demo
          </Button>
          {lastPlanId && (
            <Button onClick={accept} disabled={acceptStatus === 'loading' || acceptStatus === 'ready'}>
              <CheckCircle2 className="mr-2 h-4 w-4" />
              {acceptStatus === 'ready'
                ? 'Plan accepted'
                : acceptStatus === 'loading'
                  ? 'Accepting…'
                  : 'Accept plan'}
            </Button>
          )}
        </div>
      </div>

      {lastPlanId && acceptStatus !== 'ready' && !acceptError && (
        <p className="text-xs text-muted-foreground">
          Accepting freezes the plan, consumes the physical pieces it used, decrements catalog
          on-hand quantities and registers every reusable offcut as a labelled remnant.
        </p>
      )}

      {acceptError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm">
          <div className="font-medium text-destructive">Could not accept the plan</div>
          <p className="mt-1 text-muted-foreground">{acceptError}</p>
        </div>
      )}

      {acceptResult && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Badge className="bg-emerald-600 text-white">accepted</Badge>
              <CardTitle className="text-base">Shop state updated</CardTitle>
            </div>
            <CardDescription>
              {acceptResult.sheets} sheet(s): {acceptResult.formatDecrements} taken from catalog
              on-hand
              {acceptResult.formatShortages > 0 &&
                `, ${acceptResult.formatShortages} without on-hand stock to decrement`}
              {acceptResult.stockConsumed && acceptResult.stockConsumed.length > 0 &&
                `, ${acceptResult.stockConsumed.length} physical piece(s) consumed`}
              .
            </CardDescription>
          </CardHeader>
          {acceptResult.remnantsCreated && acceptResult.remnantsCreated.length > 0 && (
            <CardContent className="text-sm">
              <div className="mb-2 font-medium">
                Remnants registered ({acceptResult.remnantsCreated.length})
              </div>
              <div className="flex flex-wrap gap-2">
                {acceptResult.remnantsCreated.map((remnant) => (
                  <Badge key={remnant.id} variant="secondary">
                    {remnant.label}
                  </Badge>
                ))}
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                Find them under Stock → Pieces &amp; remnants; later runs use them first.
              </p>
            </CardContent>
          )}
        </Card>
      )}

      <PlanViewer />
    </div>
  )
}
