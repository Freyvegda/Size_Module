import { MICRON_PER_MM } from '@/lib/format'

import { COMPONENT_COLORS, componentLabel, elevationOrder, type AssemblyGeometry } from './geometry'

const mm = (um: number) => um / MICRON_PER_MM

/**
 * 2D front elevation of a product: x right / y up from the bottom-left origin,
 * the same frame the components are defined in. Rendered as plain SVG.
 */
export function AssemblyElevation({ geometry }: { geometry: AssemblyGeometry }) {
  const { widthMicron, heightMicron, depthMicron } = geometry
  const components = geometry.components ?? []

  if (widthMicron <= 0 || heightMicron <= 0) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-center text-sm text-muted-foreground">
        Set an overall width and height (or add a component) to see the 2D elevation.
      </div>
    )
  }

  const w = mm(widthMicron)
  const h = mm(heightMicron)
  const pad = Math.max(Math.min(w, h) * 0.16, 8)
  const stroke = Math.max(Math.min(w, h) * 0.004, 0.4)
  const font = Math.max(Math.min(w, h) * 0.05, 4)
  const viewW = w + pad * 2
  const viewH = h + pad * 2

  const x = (offsetUm: number) => pad + mm(offsetUm)
  // SVG grows downward; the model grows upward from the bottom edge.
  const y = (offsetUm: number, heightUm: number) => pad + mm(heightMicron - offsetUm - heightUm)

  return (
    <svg
      viewBox={`0 0 ${viewW} ${viewH}`}
      preserveAspectRatio="xMidYMid meet"
      className="h-full w-full"
      role="img"
      aria-label="Assembly elevation"
    >
      {/* Overall bounding box */}
      <rect
        x={pad}
        y={pad}
        width={w}
        height={h}
        fill="#0b1220"
        stroke="#334155"
        strokeWidth={stroke}
        strokeDasharray={`${stroke * 3} ${stroke * 2}`}
      />

      {elevationOrder(components).map((component) => {
        const panel = component.kind === 'panel'
        return (
          <rect
            key={component.id}
            x={x(component.offsetXMicron)}
            y={y(component.offsetYMicron, component.heightMicron)}
            width={Math.max(mm(component.widthMicron), 0)}
            height={Math.max(mm(component.heightMicron), 0)}
            fill={COMPONENT_COLORS[component.kind]}
            fillOpacity={panel ? 0.32 : 0.9}
            stroke={COMPONENT_COLORS[component.kind]}
            strokeWidth={stroke}
          >
            <title>
              {componentLabel(component)} · {Math.round(mm(component.widthMicron))} ×{' '}
              {Math.round(mm(component.heightMicron))} × {Math.round(mm(component.depthMicron))} mm
            </title>
          </rect>
        )
      })}

      {/* Overall width dimension, under the drawing */}
      <text
        x={pad + w / 2}
        y={pad + h + pad * 0.72}
        textAnchor="middle"
        fill="#94a3b8"
        fontSize={font}
        fontFamily="ui-monospace, monospace"
      >
        {Math.round(w)} mm
      </text>

      {/* Overall height dimension, left of the drawing (read bottom-to-top) */}
      <text
        x={pad * 0.4}
        y={pad + h / 2}
        textAnchor="middle"
        fill="#94a3b8"
        fontSize={font}
        fontFamily="ui-monospace, monospace"
        transform={`rotate(-90 ${pad * 0.4} ${pad + h / 2})`}
      >
        {Math.round(h)} mm
      </text>

      {/* Depth is a 3D-only fact; note it so the elevation is not mistaken for it. */}
      <text
        x={pad}
        y={pad * 0.55}
        fill="#64748b"
        fontSize={font * 0.85}
        fontFamily="ui-monospace, monospace"
      >
        depth {Math.round(mm(depthMicron))} mm
      </text>
    </svg>
  )
}
