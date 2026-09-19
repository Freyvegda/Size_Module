import { useEffect, useState } from 'react'
import { Pencil, Plus, RefreshCw } from 'lucide-react'

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
import { RulesProfileEditor } from '@/features/rules/RulesProfileEditor'
import { fetchRulesProfiles } from '@/features/rules/rulesSlice'
import { micronToMm } from '@/lib/format'
import type { RulesProfile } from '@/lib/types'

export function RulesPage() {
  const dispatch = useAppDispatch()
  const { profiles, status, error } = useAppSelector((state) => state.rules)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<RulesProfile>()

  useEffect(() => {
    void dispatch(fetchRulesProfiles())
  }, [dispatch])

  const openCreate = () => {
    setEditing(undefined)
    setEditorOpen(true)
  }
  const openEdit = (profile: RulesProfile) => {
    setEditing(profile)
    setEditorOpen(true)
  }
  const closeEditor = () => {
    setEditorOpen(false)
    setEditing(undefined)
  }

  const loading = status === 'loading'

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Rules profiles</h1>
          <p className="text-sm text-muted-foreground">
            Named constraint and objective presets. The engine has no glass or wood code path —
            kerf, trim, grain, cut mode, offcut policy and what “best” means are all data. A job or a
            campaign can select a profile by id.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => dispatch(fetchRulesProfiles())} disabled={loading}>
            <RefreshCw className="mr-2 h-4 w-4" />
            {loading ? 'Loading…' : 'Refresh'}
          </Button>
          <Button onClick={openCreate}>
            <Plus className="mr-2 h-4 w-4" />
            New profile
          </Button>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_400px]">
        <Card>
          <CardHeader>
            <CardTitle>Profiles</CardTitle>
            <CardDescription>{profiles.length} preset(s) from /api/v1/rules-profiles</CardDescription>
          </CardHeader>
          <CardContent>
            {error ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/5 p-4 text-sm">
                <div className="font-medium text-destructive">Database not reachable</div>
                <p className="mt-1 text-muted-foreground">{error}</p>
                <p className="mt-2 text-xs text-muted-foreground">
                  Start it with <code>db/scripts/up.ps1</code>, then run the migrations.
                </p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Code</TableHead>
                    <TableHead>Cut mode</TableHead>
                    <TableHead>Grain</TableHead>
                    <TableHead className="text-right">Kerf / trim</TableHead>
                    <TableHead className="text-right">Offcut min</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {profiles.map((profile) => (
                    <TableRow key={profile.id}>
                      <TableCell className="font-medium">
                        <span className="flex items-center gap-2">
                          {profile.code}
                          {profile.isDefault && (
                            <Badge className="bg-emerald-600 text-white">default</Badge>
                          )}
                        </span>
                        {profile.name && (
                          <span className="block text-xs text-muted-foreground">{profile.name}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{profile.rules.cutMode ?? 'guillotine'}</Badge>
                      </TableCell>
                      <TableCell>{profile.rules.grainMode ?? 'none'}</TableCell>
                      <TableCell className="text-right">
                        {micronToMm(profile.rules.kerf ?? 0)} / {micronToMm(profile.rules.trim ?? 0)} mm
                      </TableCell>
                      <TableCell className="text-right">
                        {micronToMm(profile.rules.offcutMinW ?? 0)} mm
                      </TableCell>
                      <TableCell className="text-right">
                        <Button variant="ghost" size="sm" onClick={() => openEdit(profile)}>
                          <Pencil className="mr-1 h-3.5 w-3.5" />
                          Edit
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                  {profiles.length === 0 && !loading && (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center text-sm text-muted-foreground">
                        No profiles yet. Create one — the demo seed ships a glass preset.
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
            <CardTitle>{editing ? `Edit ${editing.code}` : 'New profile'}</CardTitle>
            <CardDescription>
              {editing ? 'PUT /api/v1/rules-profiles/{id}' : 'POST /api/v1/rules-profiles'}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {editorOpen ? (
              <RulesProfileEditor
                key={editing?.id ?? 'new'}
                profile={editing}
                onDone={closeEditor}
              />
            ) : (
              <p className="text-sm text-muted-foreground">
                Select a profile to edit, or create a new one. A profile stores the full rules and
                objective, so a job can select it by id and stay reproducible.
              </p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
