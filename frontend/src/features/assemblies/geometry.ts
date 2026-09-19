// Shared geometry for the assembly (product) views. Assemblies are plain
// axis-aligned boxes in a local frame: origin at the bottom-left-front corner,
// x right, y up, z out of the wall. The 2D elevation and the 3D scene both
// consume this shape, so a draft and a saved assembly preview identically.
import type { ComponentKind } from '@/lib/types'

export interface GeometryComponent {
  id: string
  role: string
  kind: ComponentKind
  name?: string
  widthMicron: number
  heightMicron: number
  depthMicron: number
  offsetXMicron: number
  offsetYMicron: number
  offsetZMicron: number
}

/** The subset of an assembly (or a builder draft) the previews need. */
export interface AssemblyGeometry {
  widthMicron: number
  heightMicron: number
  depthMicron: number
  components: GeometryComponent[]
}

/** API speaks micrometers; three.js world units are meters. */
export const UM = 1e-6

export const COMPONENT_COLORS: Record<ComponentKind, string> = {
  beam: '#64748b',
  panel: '#38bdf8',
  custom: '#f59e0b',
}

/** A human label for a component: the name, or the role, or the kind. */
export function componentLabel(component: Pick<GeometryComponent, 'name' | 'role' | 'kind'>): string {
  return component.name?.trim() || component.role?.trim() || component.kind
}

/**
 * Paint order for the elevation: farthest boxes first so the frame lands on
 * top of the glass it holds. Ties break smaller-first.
 */
export function elevationOrder<T extends GeometryComponent>(components: T[]): T[] {
  return [...components].sort((a, b) => {
    const frontA = a.offsetZMicron + a.depthMicron
    const frontB = b.offsetZMicron + b.depthMicron
    if (frontA !== frontB) return frontA - frontB
    return a.widthMicron * a.heightMicron - b.widthMicron * b.heightMicron
  })
}
