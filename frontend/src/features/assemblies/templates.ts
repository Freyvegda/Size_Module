import type { ComponentKind } from '@/lib/types'

/** A template component in millimetres, ready to be mapped to a draft. */
export interface TemplateComponent {
  role: string
  kind: ComponentKind
  name: string
  quantity: number
  widthMm: number
  heightMm: number
  depthMm: number
  offsetXMm: number
  offsetYMm: number
  offsetZMm: number
}

const beam = (
  role: string,
  name: string,
  widthMm: number,
  heightMm: number,
  depthMm: number,
  offsetXMm: number,
  offsetYMm: number,
  offsetZMm: number,
): TemplateComponent => ({
  role,
  kind: 'beam',
  name,
  quantity: 1,
  widthMm,
  heightMm,
  depthMm,
  offsetXMm,
  offsetYMm,
  offsetZMm,
})

const unique = (values: number[]) => [...new Set(values)]

export interface WindowTemplateInput {
  widthMm: number
  heightMm: number
  frameSectionMm: number
  frameDepthMm: number
  glassThicknessMm: number
  glassInsetMm: number
}

/** Four-bean frame plus a glass pane, sized from the overall opening. */
export function windowTemplate(input: WindowTemplateInput): TemplateComponent[] {
  const { widthMm: w, heightMm: h, frameSectionMm: section, frameDepthMm: depth } = input
  const innerW = Math.max(w - 2 * section, 0)
  const innerH = Math.max(h - 2 * section, 0)
  return [
    beam('frame-bottom', 'Bottom frame', w, section, depth, 0, 0, 0),
    beam('frame-top', 'Top frame', w, section, depth, 0, h - section, 0),
    beam('frame-left', 'Left frame', section, innerH, depth, 0, section, 0),
    beam('frame-right', 'Right frame', section, innerH, depth, w - section, section, 0),
    {
      role: 'glass',
      kind: 'panel',
      name: 'Glass pane',
      quantity: 1,
      widthMm: Math.max(innerW - 2 * input.glassInsetMm, 0),
      heightMm: Math.max(innerH - 2 * input.glassInsetMm, 0),
      depthMm: input.glassThicknessMm,
      offsetXMm: section + input.glassInsetMm,
      offsetYMm: section + input.glassInsetMm,
      offsetZMm: Math.max((depth - input.glassThicknessMm) / 2, 0),
    },
  ]
}

export interface TableTemplateInput {
  widthMm: number
  heightMm: number
  depthMm: number
  topThicknessMm: number
  legSectionMm: number
  apronHeightMm: number
  apronThicknessMm: number
  legInsetMm: number
  includeShelf: boolean
}

/**
 * A table: top panel, four legs, two long and two short aprons, optional shelf.
 * Legs share the `leg` role so the explode step merges them into one part with
 * quantity 4.
 */
export function tableTemplate(input: TableTemplateInput): TemplateComponent[] {
  const {
    widthMm: w,
    heightMm: h,
    depthMm: d,
    topThicknessMm: topT,
    legSectionMm: section,
    apronHeightMm: apronH,
    apronThicknessMm: apronT,
    legInsetMm: inset,
  } = input

  const legHeight = Math.max(h - topT, 0)
  const legXs = unique([inset, Math.max(w - inset - section, inset)])
  const legZs = unique([inset, Math.max(d - inset - section, inset)])
  const apronY = Math.max(h - topT - apronH, 0)
  const longApronW = Math.max(w - 2 * inset, 0)
  const shortApronD = Math.max(d - 2 * inset, 0)

  const components: TemplateComponent[] = [
    {
      role: 'top',
      kind: 'panel',
      name: 'Table top',
      quantity: 1,
      widthMm: w,
      heightMm: topT,
      depthMm: d,
      offsetXMm: 0,
      offsetYMm: Math.max(h - topT, 0),
      offsetZMm: 0,
    },
  ]

  for (const x of legXs) {
    for (const z of legZs) {
      components.push(
        beam('leg', 'Leg', section, legHeight, section, x, 0, z),
      )
    }
  }
  for (const z of unique([inset, Math.max(d - inset - apronT, inset)])) {
    components.push(
      beam('apron-long', 'Long apron', longApronW, apronH, apronT, inset, apronY, z),
    )
  }
  for (const x of unique([inset, Math.max(w - inset - apronT, inset)])) {
    components.push(
      beam('apron-short', 'Short apron', apronT, apronH, shortApronD, x, apronY, inset),
    )
  }

  if (input.includeShelf) {
    components.push({
      role: 'shelf',
      kind: 'panel',
      name: 'Shelf',
      quantity: 1,
      widthMm: Math.max(w - 2 * inset, 0),
      heightMm: topT,
      depthMm: Math.max(d - 2 * inset, 0),
      offsetXMm: inset,
      offsetYMm: Math.round(legHeight * 0.4),
      offsetZMm: inset,
    })
  }

  return components
}
