import type { DimensionProfile, ProblemPart } from '@/lib/types'

/**
 * A component reduced to what the solver needs, with its material already
 * resolved (component override or the product's default).
 */
export interface ExplodeComponent {
  role: string
  name?: string
  quantity: number
  materialSpecId?: string
  widthMicron: number
  heightMicron: number
  depthMicron: number
}

function slugify(value: string, fallback: string): string {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return slug || fallback
}

/**
 * Explodes a product into cut parts.
 *
 * Each component is a 3D box; its cut face is the two largest dimensions (2D)
 * or the single largest dimension (1D), so orientation does not matter: a
 * vertical glass pane cuts as width × height, a horizontal table top as
 * width × depth, a leg as length × section.
 *
 * Identical parts (same role, dimensions and material) are merged and their
 * quantities summed, so four legs become one part with quantity 4.
 */
export function explodeComponents(args: {
  components: ExplodeComponent[]
  profile: DimensionProfile
  productQuantity: number
  fallbackMaterialSpecId?: string
}): ProblemPart[] {
  const productQuantity = Math.max(1, Math.floor(args.productQuantity) || 1)
  const merged = new Map<string, ProblemPart>()

  args.components.forEach((component, index) => {
    const dims = [component.widthMicron, component.heightMicron, component.depthMicron]
      .filter((value) => Number.isFinite(value) && value > 0)
      .sort((a, b) => b - a)
    if (dims.length === 0) return

    const materialSpecId = component.materialSpecId || args.fallbackMaterialSpecId
    const quantity = Math.max(1, Math.floor(component.quantity) || 1) * productQuantity
    const code = slugify(component.role || component.name || '', `part-${index + 1}`)
    const size =
      args.profile === '1d'
        ? `${Math.round(dims[0] / 1000)}`
        : `${Math.round(dims[0] / 1000)}x${Math.round((dims[1] ?? dims[0]) / 1000)}`
    const materialSuffix = materialSpecId ? materialSpecId.slice(0, 8) : 'none'
    const id = `${code}-${size}-${materialSuffix}`

    let part: ProblemPart
    if (args.profile === '1d') {
      part = { id, code, length: dims[0], quantity, allowRotate: true }
    } else {
      part = { id, code, width: dims[0], height: dims[1] ?? dims[0], quantity, allowRotate: true }
    }
    if (materialSpecId) part.materialSpecId = materialSpecId

    const existing = merged.get(id)
    if (existing) {
      existing.quantity = (existing.quantity ?? 0) + quantity
    } else {
      merged.set(id, part)
    }
  })

  return [...merged.values()]
}
