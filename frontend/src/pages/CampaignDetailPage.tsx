import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ExternalLink, Play, Plus, RefreshCw, Trash2, XCircle } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { fetchParts } from '@/features/catalog/catalogSlice'
import {
  addCampaignItem,
  fetchCampaign,
  removeCampaignItem,
  runCampaignNext,
  updateCampaign,
} from '@/features/campaigns/campaignSlice'
import { toProblemPart } from '@/features/optimizer/buildProblem'
import { loadPlanDetail } from '@/features/optimizer/optimizerSlice'
import { micronToMm } from '@/lib/format'
import type { CampaignItemStatus, ProblemPart, StockItem } from '@/lib/types'

const itemVariant: Record<CampaignItemStatus, 'default' | 'secondary' | 'destructive'> = {
  pending: 'secondary',
  planned: 'default',
  failed: 'destructive',
}

function stockSize(entry: StockItem): string {
  if (entry.length) return `${micronToMm(entry.length)} mm`
  if (entry.width && entry.height) {
    return `${micronToMm(entry.width)} × ${micronToMm(entry.height)} mm`
  }
  return '—'
}

export function CampaignDetailPage() {
  const { id = '' } = useParams()
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { detail, detailStatus, detailError, mutateStatus, mutateError } = useAppSelector(
    (state) => state.campaigns,
  )
  const parts = useAppSelector((state) => state.catalog.parts)

  const [itemName, setItemName] = useState('')
  const [dueDate, setDueDate] = useState('')
  const [partQtys, setPartQtys] = useState<Record<string, string>>({})
  const [lastPlan, setLastPlan] = useState<{ item: string; planId: string } | undefined>()

  useEffect(() => {
    if (id) {
      void dispatch(fetchCampaign(id))
      void dispatch(fetchParts())
    }
  }, [dispatch, id])

  const items = useMemo(() => detail?.items ?? [], [detail])
  const nextPending = items.find((item) => item.status === 'pending')
  const closed = detail ? detail.status === 'completed' || detail.status === 'cancelled' : true

  const selectedParts = useMemo<ProblemPart[]>(() => {
    return parts
      .map((part) => {
        const quantity = Number(partQtys[part.id] ?? '0') || 0
        if (quantity <= 0) return undefined
        return toProblemPart(part, quantity)
      })
      .filter((part): part is ProblemPart => part !== undefined)
  }, [parts, partQtys])

  const runNext = async () => {
    if (!detail || !nextPending) return
    const action = await dispatch(runCampaignNext({ campaignId: detail.id }))
    if (runCampaignNext.fulfilled.match(action)) {
      const planned = (action.payload.items ?? []).find((item) => item.id === nextPending.id)
      if (planned?.planId) {
        setLastPlan({ item: planned.name, planId: planned.planId })
      }
    }
  }

  const addItem = async () => {
    const action = await dispatch(
      addCampaignItem({
        id,
        input: {
          name: itemName.trim(),
          parts: selectedParts,
          dueDate: dueDate || undefined,
        },
      }),
    )
    if (addCampaignItem.fulfilled.match(action)) {
      setItemName('')
      setDueDate('')
      setPartQtys({})
    }
  }

  const openPlan = (planId: string) => {
    void dispatch(loadPlanDetail(planId))
      .unwrap()
      .then(() => navigate('/viewer'))
      .catch(() => {
        // The optimizer slice records the error.
      })
  }

  const cancel = () => {
    if (!detail) return
    void dispatch(updateCampaign({ id: detail.id, input: { status: 'cancelled' } }))
  }

  if (detailError) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-destructive">Could not load the campaign</CardTitle>
          <CardDescription>{detailError}</CardDescription>
        </CardHeader>
      </Card>
    )
  }
  if (!detail) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Loading campaign…</CardTitle>
          <CardDescription>{detailStatus === 'loading' ? 'Fetching the budget and items.' : 'No campaign loaded.'}</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold">{detail.name}</h1>
            <Badge variant={detail.status === 'completed' ? 'outline' : 'secondary'}>
              {detail.status}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            <span className="font-mono text-xs">{detail.code}</span> ·{' '}
            {detail.progress.planned}/{detail.progress.items} item(s) planned ·{' '}
            {detail.stock.length} piece(s) left in the budget
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => dispatch(fetchCampaign(id))}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
          {!closed && (
            <Button variant="outline" onClick={cancel}>
              <XCircle className="mr-2 h-4 w-4" />
              Cancel campaign
            </Button>
          )}
          <Button
            onClick={runNext}
            disabled={closed || !nextPending || mutateStatus === 'loading'}
          >
            <Play className="mr-2 h-4 w-4" />
            {mutateStatus === 'loading' ? 'Solving…' : `Run next item${nextPending ? ` (${nextPending.name})` : ''}`}
          </Button>
        </div>
      </div>

      {mutateError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm">
          <div className="font-medium text-destructive">Campaign action failed</div>
          <p className="mt-1 text-muted-foreground">{mutateError}</p>
        </div>
      )}

      {lastPlan && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-emerald-500/40 bg-emerald-500/5 p-3 text-sm">
          <Badge className="bg-emerald-600 text-white">planned</Badge>
          <span>
            {lastPlan.item} was solved; the plan is a draft version.
          </span>
          <Button size="sm" variant="outline" onClick={() => openPlan(lastPlan.planId)}>
            <ExternalLink className="mr-2 h-3.5 w-3.5" />
            Open plan
          </Button>
        </div>
      )}

      <div className="grid gap-4 xl:grid-cols-[1fr_420px]">
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Items</CardTitle>
              <CardDescription>
                Solved in order against the shared budget; a plan consumes the
                sheets it uses and returns its offcuts.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10 text-right">#</TableHead>
                    <TableHead>Item</TableHead>
                    <TableHead className="text-right">Parts</TableHead>
                    <TableHead>Due</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="text-right">Plan</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="text-right text-xs text-muted-foreground">
                        {item.seq}
                      </TableCell>
                      <TableCell className="font-medium">{item.name}</TableCell>
                      <TableCell className="text-right">
                        {(item.parts ?? []).reduce((sum, part) => sum + (part.quantity ?? 1), 0)}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {item.dueDate ? new Date(item.dueDate).toLocaleDateString() : '—'}
                      </TableCell>
                      <TableCell>
                        <Badge variant={itemVariant[item.status]}>{item.status}</Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          {item.planId && (
                            <Button size="sm" variant="ghost" onClick={() => openPlan(item.planId!)}>
                              <ExternalLink className="mr-1 h-3.5 w-3.5" />
                              Open
                            </Button>
                          )}
                          {item.status === 'pending' && !closed && (
                            <Button
                              size="sm"
                              variant="ghost"
                              onClick={() => dispatch(removeCampaignItem({ id, itemId: item.id }))}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                  {items.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">
                        No items yet — add the first job on the right.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Remaining stock budget</CardTitle>
              <CardDescription>
                {detail.stock.length} piece(s); CMP-… entries are offcuts that
                came back from earlier items.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Piece</TableHead>
                    <TableHead>Size</TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Cost</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {detail.stock.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell>
                        <div className="font-medium">{entry.label || entry.code}</div>
                        {entry.isRemnant && (
                          <Badge variant="secondary" className="mt-1">
                            remnant
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell>{stockSize(entry)}</TableCell>
                      <TableCell className="text-right">{entry.quantity ?? 1}</TableCell>
                      <TableCell className="text-right">
                        {(entry.costPerUnit ?? 0).toFixed(2)}
                      </TableCell>
                    </TableRow>
                  ))}
                  {detail.stock.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={4} className="text-center text-sm text-muted-foreground">
                        Budget empty.
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Add item</CardTitle>
            <CardDescription>
              Pick catalog parts and quantities; the item is appended after the
              current ones.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="item-name">Name</Label>
              <Input
                id="item-name"
                placeholder="Order 1042"
                value={itemName}
                onChange={(event) => setItemName(event.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="item-due">Due date</Label>
              <Input
                id="item-due"
                type="date"
                value={dueDate}
                onChange={(event) => setDueDate(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label>Parts</Label>
              {parts.length === 0 && (
                <p className="text-xs text-muted-foreground">
                  No catalog parts loaded — add some on the Parts page.
                </p>
              )}
              {parts.map((part) => (
                <div
                  key={part.id}
                  className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{part.code}</div>
                    <div className="text-xs text-muted-foreground">
                      {part.finishedLengthMicron > 0
                        ? `${micronToMm(part.finishedLengthMicron)} mm`
                        : `${micronToMm(part.finishedWidthMicron)} × ${micronToMm(part.finishedHeightMicron)} mm`}
                    </div>
                  </div>
                  <Input
                    className="w-20"
                    inputMode="numeric"
                    placeholder="0"
                    value={partQtys[part.id] ?? ''}
                    onChange={(event) =>
                      setPartQtys((current) => ({ ...current, [part.id]: event.target.value }))
                    }
                  />
                </div>
              ))}
            </div>
            <Button
              className="w-full"
              disabled={itemName.trim().length === 0 || selectedParts.length === 0 || closed || mutateStatus === 'loading'}
              onClick={addItem}
            >
              <Plus className="mr-2 h-4 w-4" />
              Add item
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
