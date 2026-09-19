import type {
  DimensionProfile,
  MaterialSpec,
  Part,
  Problem,
  ProblemPart,
  StockFormat,
  StockItem,
} from '@/lib/types'

/**
 * Turns catalog parts into solver parts. The solver must only ever see cut
 * sizes, so this is the one place the finished size of a catalog part is
 * converted: the backend already applied the routing allowances and returned
 * `cutLengthMicron` / `cutWidthMicron` / `cutHeightMicron`.
 */
export function toProblemPart(part: Part, quantity: number): ProblemPart {
  const base: ProblemPart = {
    id: part.id,
    code: part.code,
    quantity: Math.max(1, Math.floor(quantity) || 1),
    grain: part.grain,
    allowRotate: part.allowRotate,
    priority: part.priority,
  }
  if (part.materialSpecId) base.materialSpecId = part.materialSpecId
  if (part.cutLengthMicron > 0) {
    return { ...base, length: part.cutLengthMicron }
  }
  return {
    ...base,
    width: part.cutWidthMicron || part.finishedWidthMicron,
    height: part.cutHeightMicron || part.finishedHeightMicron,
  }
}

export interface OrderLine {
  part: ProblemPart
  /** Resolved material of the part; groups the problem and scopes its stock. */
  materialSpecId?: string
  dimension: DimensionProfile
}

export interface CatalogOrder {
  /** Groups equal (material, dimension) lines into one solve. */
  key: string
  label: string
  dimension: DimensionProfile
  materialSpecId?: string
  problem: Problem
}

/** Catalog stock for a dimension: one entry per matching format, on-hand qty. */
export function stockFromFormats(
  dimension: DimensionProfile,
  formats: StockFormat[],
): StockItem[] {
  return formats.map((format) =>
    dimension === '1d'
      ? {
          id: format.id,
          code: format.code,
          length: format.lengthMicron,
          ...(format.widthMicron > 0 ? { width: format.widthMicron } : {}),
          quantity: Math.max(format.onHandQty, 1),
          costPerUnit: format.costPerUnit,
        }
      : {
          id: format.id,
          code: format.code,
          width: format.widthMicron,
          height: format.heightMicron,
          quantity: Math.max(format.onHandQty, 1),
          costPerUnit: format.costPerUnit,
        },
  )
}

/**
 * Splits an order into one problem per (material spec, dimension profile), the
 * engine's unit of work. A window that mixes glass panels (2D) and aluminium
 * frames (1D) becomes two solvable problems instead of an impossible mixed one.
 */
export function buildCatalogOrders(args: {
  lines: OrderLine[]
  materialSpecs: MaterialSpec[]
  stockFormats: StockFormat[]
  budgetMs?: number
}): CatalogOrder[] {
  const budgetMs = args.budgetMs ?? 5000
  const groups = new Map<string, { line: OrderLine; parts: ProblemPart[] }>()

  for (const line of args.lines) {
    if (!line.part || (line.part.quantity ?? 0) <= 0) continue
    const key = `${line.materialSpecId ?? 'unbound'}:${line.dimension}`
    const existing = groups.get(key)
    if (existing) {
      existing.parts.push(line.part)
    } else {
      groups.set(key, { line, parts: [line.part] })
    }
  }

  const orders: CatalogOrder[] = []
  for (const [key, group] of groups) {
    const { materialSpecId, dimension } = group.line
    const spec = args.materialSpecs.find((item) => item.id === materialSpecId)
    const formats = args.stockFormats.filter((format) =>
      materialSpecId
        ? format.materialSpecId === materialSpecId
        : format.dimensionProfile === dimension,
    )
    orders.push({
      key,
      label: spec ? `${spec.materialName} · ${spec.name || spec.code}` : dimension,
      dimension,
      materialSpecId,
      problem: {
        parts: group.parts,
        stocks: stockFromFormats(dimension, formats),
        budgetMs,
      },
    })
  }
  return orders
}
