import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Plus, RefreshCw } from 'lucide-react'

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
import { fetchStockFormats } from '@/features/catalog/catalogSlice'
import {
  createCampaign,
  fetchCampaigns,
} from '@/features/campaigns/campaignSlice'
import type { CampaignStatus, StockItem } from '@/lib/types'

const statusVariant: Record<CampaignStatus, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  draft: 'secondary',
  active: 'default',
  completed: 'outline',
  cancelled: 'destructive',
}

export function CampaignsPage() {
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const { campaigns, listStatus, listError, mutateStatus, mutateError } = useAppSelector(
    (state) => state.campaigns,
  )
  const formats = useAppSelector((state) => state.catalog.stockFormats)

  const [name, setName] = useState('')
  const [budgetMs, setBudgetMs] = useState('5000')
  const [useRemnants, setUseRemnants] = useState(true)
  const [quantities, setQuantities] = useState<Record<string, string>>({})

  useEffect(() => {
    void dispatch(fetchCampaigns())
    void dispatch(fetchStockFormats())
  }, [dispatch])

  const stock = useMemo<StockItem[]>(() => {
    const out: StockItem[] = []
    for (const format of formats) {
      const quantity = Number(quantities[format.id] ?? '0') || 0
      if (quantity <= 0) continue
      out.push({
        id: format.id,
        code: format.code,
        width: format.widthMicron,
        height: format.heightMicron,
        length: format.lengthMicron,
        quantity,
        costPerUnit: format.costPerUnit,
      })
    }
    return out
  }, [formats, quantities])

  const submit = async () => {
    const action = await dispatch(
      createCampaign({
        name: name.trim(),
        budgetMs: Number(budgetMs) || 5000,
        useRemnants,
        stock,
      }),
    )
    if (createCampaign.fulfilled.match(action)) {
      navigate(`/campaigns/${action.payload.id}`)
    }
  }

  const valid = name.trim().length > 0 && (stock.length > 0 || useRemnants)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Campaigns</h1>
          <p className="text-sm text-muted-foreground">
            Plan an ordered batch of jobs against one shared stock budget: each
            item consumes the sheets it uses and returns its offcuts to the pool
            for the items that follow.
          </p>
        </div>
        <Button variant="outline" onClick={() => dispatch(fetchCampaigns())}>
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh
        </Button>
      </div>

      <div className="grid gap-4 xl:grid-cols-[1fr_420px]">
        <Card>
          <CardHeader>
            <CardTitle>Campaigns</CardTitle>
            <CardDescription>{campaigns.length} campaign(s) from /api/v1/campaigns</CardDescription>
          </CardHeader>
          <CardContent>
            {listError ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                <div className="font-medium text-destructive">Database not reachable</div>
                <p className="mt-1 text-muted-foreground">{listError}</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Code</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead className="text-right">Budget</TableHead>
                    <TableHead className="text-right">Created</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {campaigns.map((campaign) => (
                    <TableRow key={campaign.id}>
                      <TableCell className="font-mono text-xs">{campaign.code}</TableCell>
                      <TableCell>
                        <Link className="font-medium hover:underline" to={`/campaigns/${campaign.id}`}>
                          {campaign.name}
                        </Link>
                      </TableCell>
                      <TableCell>
                        <Badge variant={statusVariant[campaign.status]}>{campaign.status}</Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        {campaign.stock?.length ?? 0} piece(s)
                      </TableCell>
                      <TableCell className="text-right text-xs text-muted-foreground">
                        {new Date(campaign.createdAt).toLocaleString()}
                      </TableCell>
                    </TableRow>
                  ))}
                  {campaigns.length === 0 && listStatus !== 'loading' && (
                    <TableRow>
                      <TableCell colSpan={5} className="text-center text-sm text-muted-foreground">
                        No campaigns yet. Create one on the right.
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
            <CardTitle>New campaign</CardTitle>
            <CardDescription>
              Pick the formats and quantities the batch may plan with; available
              remnants join the budget when the switch is on.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="campaign-name">Name</Label>
              <Input
                id="campaign-name"
                placeholder="Week 38 glazing"
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="campaign-budget">Solve budget (ms)</Label>
              <Input
                id="campaign-budget"
                inputMode="numeric"
                value={budgetMs}
                onChange={(event) => setBudgetMs(event.target.value)}
              />
            </div>
            <div className="flex items-center justify-between rounded-md border border-border p-3">
              <div>
                <div className="text-sm font-medium">Use available remnants</div>
                <p className="text-xs text-muted-foreground">
                  Adds the plant's labelled leftovers to the budget.
                </p>
              </div>
              <Switch checked={useRemnants} onCheckedChange={setUseRemnants} />
            </div>
            <div className="space-y-2">
              <Label>Stock formats</Label>
              {formats.length === 0 && (
                <p className="text-xs text-muted-foreground">
                  No formats loaded — run db/scripts/seed.ps1 or add formats in Stock.
                </p>
              )}
              {formats.map((format) => (
                <div
                  key={format.id}
                  className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{format.code}</div>
                    <div className="text-xs text-muted-foreground">
                      {format.materialCode} · on hand {format.onHandQty}
                    </div>
                  </div>
                  <Input
                    className="w-20"
                    inputMode="numeric"
                    placeholder="0"
                    value={quantities[format.id] ?? ''}
                    onChange={(event) =>
                      setQuantities((current) => ({ ...current, [format.id]: event.target.value }))
                    }
                  />
                </div>
              ))}
            </div>
            {mutateError && <p className="text-sm text-destructive">{mutateError}</p>}
            <Button className="w-full" disabled={!valid || mutateStatus === 'loading'} onClick={submit}>
              <Plus className="mr-2 h-4 w-4" />
              {mutateStatus === 'loading' ? 'Creating…' : 'Create campaign'}
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
