import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useAppSelector } from '@/app/hooks'

export function SettingsPage() {
  const { health, meta } = useAppSelector((state) => state.backend)
  const solvers = meta?.solvers ?? []

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Runtime information for this installation.
        </p>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Service</CardTitle>
            <CardDescription>From /healthz and /api/v1/meta</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Line label="Name" value={meta?.name ?? 'Size Module'} />
            <Line label="Version" value={health?.version ?? '—'} />
            <Line label="Environment" value={health?.env ?? '—'} />
            <Line label="Database" value={health?.db ?? '—'} />
            <Line label="Solvers" value={String(solvers.length)} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Units &amp; precision</CardTitle>
            <CardDescription>One convention everywhere</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-xs text-muted-foreground">
            <p>
              The database, the API and the solvers all store lengths as integer{' '}
              <strong>micrometers</strong>. Millimeters and inches are formatting concerns.
            </p>
            <p>Kerf, trim and offcut policy live in rules profiles, not in code.</p>
            <p>
              1D bars report length metrics (<code>stockLengthM</code>, <code>usedLengthM</code>);
              2D sheets report areas in m².
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Local tooling</CardTitle>
            <CardDescription>Scripts in db/scripts</CardDescription>
          </CardHeader>
          <CardContent className="space-y-1 text-xs text-muted-foreground">
            <p>
              <code>up.ps1</code> · start PostgreSQL and Adminer
            </p>
            <p>
              <code>migrate.ps1</code> · apply goose migrations
            </p>
            <p>
              <code>new-migration.ps1 -Name x</code> · create the next migration pair
            </p>
            <p>
              <code>seed.ps1</code> · load demo data
            </p>
            <p>
              <code>psql.ps1</code> · open a shell or run a .sql file
            </p>
            <p>
              <code>test.ps1</code> · run the table tests
            </p>
            <p>
              <code>generate.ps1</code> · regenerate sqlc Go code
            </p>
            <p>
              <code>reset.ps1</code> · drop, migrate, seed
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Registered solvers</CardTitle>
          <CardDescription>
            The same capabilities the job layer uses to pick a solver: lower rank wins, and a
            guillotine solver can always serve a free-cutting problem.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {solvers.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Solver</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Dimension</TableHead>
                  <TableHead>Cut mode</TableHead>
                  <TableHead className="text-right">Rank</TableHead>
                  <TableHead>Flags</TableHead>
                  <TableHead>Description</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {solvers.map((solver) => {
                  const caps = solver.capabilities
                  const flags = [
                    caps.Rotation && 'rotation',
                    caps.Grain && 'grain',
                    caps.Remnants && 'remnants',
                    caps.MaxParts > 0 && `max ${caps.MaxParts} parts`,
                  ]
                    .filter(Boolean)
                    .join(' · ')
                  return (
                    <TableRow key={solver.name}>
                      <TableCell className="font-medium">{solver.name}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{solver.version}</TableCell>
                      <TableCell>
                        <Badge variant="secondary">{caps.Dimension}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant={caps.CutMode === 'free' ? 'default' : 'outline'}>
                          {caps.CutMode}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right">{caps.Rank > 0 ? caps.Rank : '—'}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{flags || '—'}</TableCell>
                      <TableCell className="max-w-[380px] text-xs text-muted-foreground">
                        {caps.Description}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          ) : (
            <p className="text-sm text-muted-foreground">
              No solver information — the API is unreachable.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Line({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{value}</span>
    </div>
  )
}
