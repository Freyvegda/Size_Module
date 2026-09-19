import { Edges, OrbitControls, PerspectiveCamera } from '@react-three/drei'
import { Canvas } from '@react-three/fiber'
import { useMemo } from 'react'

import { COMPONENT_COLORS, UM, type AssemblyGeometry, type GeometryComponent } from './geometry'

/**
 * 3D view of a product: one box per component in the assembly's local frame,
 * centered so an orbit target of the origin keeps the whole product in view.
 * Loaded lazily by the viewer so three.js stays out of the initial bundle.
 */
export function AssemblyScene({ geometry }: { geometry: AssemblyGeometry }) {
  const { widthMicron, heightMicron, depthMicron } = geometry
  const components = geometry.components ?? []

  const w = widthMicron * UM
  const h = heightMicron * UM
  const d = depthMicron * UM
  // Keep a sane scale for empty/degenerate drafts so the camera never sits inside.
  const maxDim = Math.max(w, h, d, 0.3)

  const cameraPosition = useMemo<[number, number, number]>(
    () => [maxDim * 1.5, maxDim * 1.1, maxDim * 1.9],
    [maxDim],
  )

  return (
    <Canvas dpr={[1, 2]} gl={{ antialias: true }} style={{ background: '#080d1a' }}>
      <ambientLight intensity={0.8} />
      <directionalLight position={[3, 4, 6]} intensity={1.2} />
      <directionalLight position={[-4, -2, -5]} intensity={0.35} />

      <PerspectiveCamera makeDefault position={cameraPosition} fov={45} />
      <OrbitControls
        target={[0, 0, 0]}
        enablePan
        enableZoom
        enableRotate
        minDistance={maxDim * 0.4}
        maxDistance={maxDim * 8}
      />
      <gridHelper
        args={[maxDim * 6, 24, '#1e293b', '#111827']}
        position={[0, -h / 2, 0]}
      />

      <group>
        {components.map((component) => (
          <ComponentBox
            key={component.id}
            component={component}
            center={[w / 2, h / 2, d / 2]}
          />
        ))}
      </group>
    </Canvas>
  )
}

function ComponentBox({
  component,
  center,
}: {
  component: GeometryComponent
  center: [number, number, number]
}) {
  const cw = component.widthMicron * UM
  const ch = component.heightMicron * UM
  const cd = component.depthMicron * UM
  const cx = component.offsetXMicron * UM + cw / 2 - center[0]
  const cy = component.offsetYMicron * UM + ch / 2 - center[1]
  const cz = component.offsetZMicron * UM + cd / 2 - center[2]
  const panel = component.kind === 'panel'

  return (
    <mesh position={[cx, cy, cz]}>
      <boxGeometry args={[Math.max(cw, 0.001), Math.max(ch, 0.001), Math.max(cd, 0.001)]} />
      <meshStandardMaterial
        color={COMPONENT_COLORS[component.kind]}
        transparent={panel}
        opacity={panel ? 0.4 : 1}
        roughness={panel ? 0.08 : 0.5}
        metalness={component.kind === 'beam' ? 0.45 : 0.05}
      />
      {!panel && <Edges color="#0f172a" />}
    </mesh>
  )
}
