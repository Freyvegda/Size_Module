import { lazy, Suspense, useState } from 'react'

import { Button } from '@/components/ui/button'

import { AssemblyElevation } from './AssemblyElevation'
import type { AssemblyGeometry } from './geometry'

// three.js is heavy; keep it in its own chunk behind the lazy import.
const AssemblyScene = lazy(() =>
  import('./AssemblyScene').then((module) => ({ default: module.AssemblyScene })),
)

/** 2D/3D preview of a product, driven by the same geometry. */
export function AssemblyViewer({ geometry }: { geometry: AssemblyGeometry }) {
  const [mode, setMode] = useState<'2d' | '3d'>('2d')

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">
          {mode === '2d' ? 'Front elevation' : '3D orbit · drag to rotate'}
        </span>
        <div className="flex gap-1">
          <Button
            type="button"
            size="sm"
            variant={mode === '2d' ? 'secondary' : 'ghost'}
            onClick={() => setMode('2d')}
          >
            2D
          </Button>
          <Button
            type="button"
            size="sm"
            variant={mode === '3d' ? 'secondary' : 'ghost'}
            onClick={() => setMode('3d')}
          >
            3D
          </Button>
        </div>
      </div>

      <div className="aspect-[4/3] w-full overflow-hidden rounded-md border border-border bg-[#080d1a]">
        {mode === '2d' ? (
          <div className="h-full w-full p-2">
            <AssemblyElevation geometry={geometry} />
          </div>
        ) : (
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                Loading 3D…
              </div>
            }
          >
            <AssemblyScene geometry={geometry} />
          </Suspense>
        )}
      </div>
    </div>
  )
}
