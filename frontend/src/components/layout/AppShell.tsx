import { useEffect } from 'react'
import {
  Boxes,
  Eye,
  LayoutDashboard,
  Layers,
  Play,
  Settings,
  SquareStack,
  Wrench,
} from 'lucide-react'
import { NavLink, Outlet } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { useAppDispatch, useAppSelector } from '@/app/hooks'
import { fetchBackend } from '@/features/backend/backendSlice'

const navigation = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/viewer', label: 'Plan viewer', icon: Eye },
  { to: '/materials', label: 'Materials', icon: Layers },
  { to: '/parts', label: 'Parts', icon: SquareStack },
  { to: '/stock', label: 'Stock', icon: Boxes },
  { to: '/jobs', label: 'Jobs', icon: Play },
  { to: '/settings', label: 'Settings', icon: Settings },
]

export function AppShell() {
  const dispatch = useAppDispatch()
  const { health, meta, status } = useAppSelector((state) => state.backend)

  useEffect(() => {
    if (status === 'idle') {
      void dispatch(fetchBackend())
    }
  }, [dispatch, status])

  const apiBadge = (() => {
    if (status === 'loading') return <Badge variant="secondary">checking API…</Badge>
    if (status === 'error') return <Badge variant="destructive">API offline</Badge>
    if (health?.db === 'up') return <Badge className="bg-emerald-600 text-white">API + DB online</Badge>
    return <Badge variant="secondary">API online · DB offline</Badge>
  })()

  return (
    <div className="flex min-h-svh bg-background">
      <aside className="hidden w-60 shrink-0 border-r border-border bg-sidebar p-4 md:flex md:flex-col">
        <div className="px-2 py-3">
          <div className="flex items-center gap-2 text-lg font-semibold">
            <Wrench className="h-5 w-5 text-sky-500" />
            Size Module
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            Cut planning &amp; material yield
          </p>
        </div>
        <Separator className="my-3" />
        <nav className="flex flex-1 flex-col gap-1">
          {navigation.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                  isActive
                    ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                    : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground'
                }`
              }
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="rounded-md border border-border p-3 text-xs text-muted-foreground">
          <div className="font-medium text-foreground">v{meta?.version ?? '0.1.0-dev'}</div>
          <div>{meta?.solvers.length ?? 0} solver(s) registered</div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-border px-4 md:px-6">
          <div className="text-sm text-muted-foreground">
            <span className="font-medium text-foreground">Cut optimizer</span> · least material, least
            waste
          </div>
          {apiBadge}
        </header>
        <main className="min-w-0 flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
