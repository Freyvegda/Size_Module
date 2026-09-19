import { Html, OrbitControls, OrthographicCamera, PerspectiveCamera } from '@react-three/drei'
import { Canvas, useFrame, useThree, type ThreeEvent } from '@react-three/fiber'
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Color,
  type Group,
  type Mesh,
  type MeshStandardMaterial,
  type OrthographicCamera as ThreeOrthoCamera,
} from 'three'

import type { SheetPlan } from '@/lib/types'

import { partColor } from './partColor'
import { placementKey } from './viewerSlice'

// The API speaks micrometers; the scene speaks meters (1 world unit = 1 m).
const UM = 1e-6
// Sheet thickness is exaggerated so the stack is visible.
const VIS_THICKNESS = 0.016
const SHEET_GAP = 0.05
const HIGHLIGHT = '#38bdf8'
const SELECTED = '#f59e0b'
// Exponential damping rate for the per-frame animations (higher = snappier).
const DAMP = 6

export interface PlanSceneProps {
  sheets: SheetPlan[]
  mode: '2d' | '3d'
  selectedSheet: number
  selectedPartId?: string
  showOffcuts: boolean
  /** 0 = compact stack, 1 = fully exploded (3D only). */
  explode: number
  onSelectPart: (partId?: string) => void
  /** Edit mode lets pieces be dragged (2D only). */
  editMode?: boolean
  onMovePart?: (placementId: string, xUm: number, yUm: number) => void
}

export function PlanScene(props: PlanSceneProps) {
  const {
    sheets,
    mode,
    selectedSheet,
    selectedPartId,
    showOffcuts,
    explode,
    onSelectPart,
    editMode = false,
    onMovePart,
  } = props
  const [dragging, setDragging] = useState(false)

  const visibleSheets = useMemo(
    () => (mode === '2d' ? sheets.filter((s) => s.index === selectedSheet) : sheets),
    [mode, sheets, selectedSheet],
  )

  // Front-to-back stacking order. The selected sheet is pulled to the front of
  // the stack so it is never hidden behind another sheet in 3D mode.
  const stackRank = useMemo(() => {
    const ordered = [...sheets]
    const activeIndex = ordered.findIndex((s) => s.index === selectedSheet)
    if (activeIndex > 0) ordered.unshift(...ordered.splice(activeIndex, 1))
    return new Map(ordered.map((sheet, rank) => [sheet.index, rank]))
  }, [sheets, selectedSheet])

  const spread = SHEET_GAP * (1 + explode * 2.5)

  return (
    <Canvas
      dpr={[1, 2]}
      gl={{ antialias: true }}
      onPointerMissed={() => onSelectPart(undefined)}
      style={{ background: '#080d1a' }}
    >
      <ambientLight intensity={0.75} />
      <directionalLight position={[3, 4, 6]} intensity={1.15} />
      <directionalLight position={[-4, -3, -5]} intensity={0.35} />

      {mode === '2d' ? (
        <>
          <OrthographicCamera makeDefault position={[0, 0, 6]} near={0.01} far={50} />
          {visibleSheets[0] && (
            <FitOrthographic
              width={visibleSheets[0].width * UM}
              height={visibleSheets[0].height * UM}
            />
          )}
          {/* Rotation stays locked so the sheet behaves like a drawing. While
              a piece is dragged the controls are disabled so the drag wins. */}
          <OrbitControls
            enableRotate={false}
            enablePan
            enableZoom
            screenSpacePanning
            enabled={!dragging}
          />
        </>
      ) : (
        <>
          <PerspectiveCamera makeDefault position={[3.4, 2.6, 3.6]} fov={46} />
          <OrbitControls
            enablePan
            enableZoom
            enableRotate
            target={[0, 0, 0]}
            minDistance={0.6}
            maxDistance={14}
          />
          <gridHelper
            args={[10, 20, '#1e293b', '#111827']}
            rotation={[Math.PI / 2, 0, 0]}
            position={[0, 0, -1.2]}
          />
        </>
      )}

      {visibleSheets.map((sheet) => {
        const rank = stackRank.get(sheet.index) ?? 0
        const targetZ = mode === '2d' ? 0 : ((sheets.length - 1) / 2 - rank) * spread
        return (
          <SheetMesh
            key={sheet.index}
            sheet={sheet}
            mode={mode}
            targetZ={targetZ}
            active={sheet.index === selectedSheet}
            dimmed={mode === '3d' && sheets.length > 1 && sheet.index !== selectedSheet}
            showLabel={mode === '3d'}
            showOffcuts={showOffcuts}
            selectedPartId={selectedPartId}
            onSelectPart={onSelectPart}
            editMode={editMode && mode === '2d'}
            onMovePart={onMovePart}
            onDragStateChange={setDragging}
          />
        )
      })}
    </Canvas>
  )
}

interface SheetMeshProps {
  sheet: SheetPlan
  mode: '2d' | '3d'
  targetZ: number
  active: boolean
  dimmed: boolean
  showLabel: boolean
  showOffcuts: boolean
  selectedPartId?: string
  onSelectPart: (partId?: string) => void
  editMode: boolean
  onMovePart?: (placementId: string, xUm: number, yUm: number) => void
  onDragStateChange?: (dragging: boolean) => void
}

function SheetMesh({
  sheet,
  mode,
  targetZ,
  active,
  dimmed,
  showLabel,
  showOffcuts,
  selectedPartId,
  onSelectPart,
  editMode,
  onMovePart,
  onDragStateChange,
}: SheetMeshProps) {
  const group = useRef<Group>(null)
  const plate = useRef<MeshStandardMaterial>(null)
  const glow = useRef<Mesh>(null)
  const appear = useRef(0)
  const placed = useRef(false)

  useFrame((_, delta) => {
    const g = group.current
    const k = 1 - Math.exp(-DAMP * delta)

    if (g) {
      if (!placed.current) {
        // Snap to the initial slot on the first frame, then glide on changes.
        g.position.z = targetZ
        placed.current = true
      }
      // Glide each sheet to its slot so re-stacking on selection is smooth.
      g.position.z += (targetZ - g.position.z) * k
      // Gentle pop-in when the sheet first appears (mostly 2D sheet switching).
      appear.current = Math.min(1, appear.current + delta * 3.5)
      const eased = 1 - Math.pow(1 - appear.current, 3)
      g.scale.setScalar(mode === '2d' ? 0.94 + 0.06 * eased : 1)
    }

    if (plate.current) {
      const target = active && !dimmed ? 0.5 : 0
      plate.current.emissiveIntensity += (target - plate.current.emissiveIntensity) * k
    }

    const glowMaterial = glow.current?.material
    if (glowMaterial && 'opacity' in glowMaterial) {
      const target = active ? 0.95 : 0
      glowMaterial.opacity += (target - glowMaterial.opacity) * k
    }
  })

  const w = sheet.width * UM
  const h = sheet.height * UM
  const cx = w / 2
  const cy = h / 2
  const margin = Math.max(Math.min(w, h) * 0.02, 0.02)

  return (
    <group ref={group}>
      {/* Glow plate peeking out around the active sheet to frame the selection. */}
      <mesh ref={glow} position={[0, 0, -VIS_THICKNESS * 0.6]}>
        <boxGeometry args={[w + margin * 2, h + margin * 2, VIS_THICKNESS * 0.5]} />
        <meshBasicMaterial color={HIGHLIGHT} transparent opacity={0} depthWrite={false} />
      </mesh>

      {/* Base plate */}
      <mesh position={[0, 0, -VIS_THICKNESS / 2]}>
        <boxGeometry args={[w, h, VIS_THICKNESS]} />
        <meshStandardMaterial
          ref={plate}
          color="#94a3b8"
          emissive={HIGHLIGHT}
          emissiveIntensity={0}
          metalness={0.15}
          roughness={0.75}
        />
      </mesh>

      {(sheet.placements ?? []).map((pl, index) => {
        const x = (pl.x + pl.w / 2) * UM - cx
        // The layout uses a top-left origin; the scene grows upwards.
        const y = cy - (pl.y + pl.h / 2) * UM
        const key = placementKey(pl)
        const selected = selectedPartId === key
        return (
          <PartMesh
            key={pl.id ?? `${pl.partId}-${index}`}
            x={x}
            y={y}
            width={pl.w * UM}
            height={pl.h * UM}
            color={partColor(pl.partCode, pl.priority)}
            selected={selected}
            highlighted={active && !selected}
            dimmed={dimmed}
            locked={editMode && !!pl.locked}
            draggable={editMode}
            onClick={() => onSelectPart(selected ? undefined : key)}
            onMove={
              onMovePart
                ? (centerX, centerY) => {
                    const xUm = Math.round(((centerX + cx) / UM - pl.w / 2) / 1000) * 1000
                    const yUm = Math.round(((cy - centerY) / UM - pl.h / 2) / 1000) * 1000
                    const maxX = Math.max(0, sheet.width - pl.w)
                    const maxY = Math.max(0, sheet.height - pl.h)
                    onMovePart(key, Math.min(Math.max(0, xUm), maxX), Math.min(Math.max(0, yUm), maxY))
                  }
                : undefined
            }
            onDragStateChange={onDragStateChange}
          />
        )
      })}

      {showOffcuts &&
        (sheet.offcuts ?? []).map((off, index) => (
          <mesh
            key={`offcut-${index}`}
            position={[(off.x + off.w / 2) * UM - cx, cy - (off.y + off.h / 2) * UM, VIS_THICKNESS * 0.25]}
          >
            <boxGeometry args={[off.w * UM, off.h * UM, VIS_THICKNESS * 0.12]} />
            <meshStandardMaterial color="#22c55e" transparent opacity={0.3} />
          </mesh>
        ))}

      {showLabel && (
        <Html position={[-cx, -cy - 0.12, 0]} center style={{ pointerEvents: 'none' }}>
          <div
            style={{
              color: active ? HIGHLIGHT : '#94a3b8',
              fontSize: 11,
              whiteSpace: 'nowrap',
              transition: 'color 300ms ease',
            }}
          >
            {sheet.label ?? sheet.stockCode}
          </div>
        </Html>
      )}
    </group>
  )
}

interface PartMeshProps {
  x: number
  y: number
  width: number
  height: number
  color: string
  selected: boolean
  /** The part sits on the active sheet and should glow softly. */
  highlighted: boolean
  /** The part sits on a sheet that is not active and should recede. */
  dimmed: boolean
  /** Locked placements are pinned for the next re-solve. */
  locked?: boolean
  draggable?: boolean
  onClick: () => void
  /** Called with the new scene-local centre after a drag. */
  onMove?: (centerX: number, centerY: number) => void
  onDragStateChange?: (dragging: boolean) => void
}

function PartMesh({
  x,
  y,
  width,
  height,
  color,
  selected,
  highlighted,
  dimmed,
  locked = false,
  draggable = false,
  onClick,
  onMove,
  onDragStateChange,
}: PartMeshProps) {
  const [hovered, setHovered] = useState(false)
  const [preview, setPreview] = useState<[number, number] | null>(null)
  const previewRef = useRef<[number, number] | null>(null)
  const drag = useRef<{ startX: number; startY: number; originX: number; originY: number } | null>(
    null,
  )
  const material = useRef<MeshStandardMaterial>(null)
  const emissive = useRef(new Color('#000000'))

  useEffect(() => {
    document.body.style.cursor = drag.current
      ? 'grabbing'
      : hovered
        ? draggable
          ? 'grab'
          : 'pointer'
        : 'auto'
    return () => {
      document.body.style.cursor = 'auto'
    }
  }, [hovered, draggable])

  useFrame((_, delta) => {
    const m = material.current
    if (!m) return
    const k = 1 - Math.exp(-9 * delta)
    const base = locked ? '#10b981' : '#000000'
    emissive.current.set(selected ? SELECTED : hovered || highlighted ? HIGHLIGHT : base)
    m.emissive.lerp(emissive.current, k)
    const intensity = selected ? 0.8 : hovered ? 0.55 : highlighted ? 0.28 : locked ? 0.4 : 0
    m.emissiveIntensity += (intensity - m.emissiveIntensity) * k
    const opacity = dimmed && !selected ? 0.3 : 1
    m.opacity += (opacity - m.opacity) * k
  })

  const handleClick = (event: ThreeEvent<MouseEvent>) => {
    event.stopPropagation()
    onClick()
  }

  const handlePointerDown = (event: ThreeEvent<PointerEvent>) => {
    if (!draggable || !onMove || event.button !== 0) return
    event.stopPropagation()
    const target = event.target as Element | null
    target?.setPointerCapture?.(event.pointerId)
    drag.current = { startX: event.point.x, startY: event.point.y, originX: x, originY: y }
    previewRef.current = [x, y]
    setPreview([x, y])
    onDragStateChange?.(true)
  }

  const handlePointerMove = (event: ThreeEvent<PointerEvent>) => {
    if (!drag.current) return
    event.stopPropagation()
    const next: [number, number] = [
      drag.current.originX + (event.point.x - drag.current.startX),
      drag.current.originY + (event.point.y - drag.current.startY),
    ]
    previewRef.current = next
    setPreview(next)
  }

  const handlePointerUp = (event: ThreeEvent<PointerEvent>) => {
    if (!drag.current) return
    event.stopPropagation()
    const [px, py] = previewRef.current ?? [x, y]
    drag.current = null
    previewRef.current = null
    setPreview(null)
    onDragStateChange?.(false)
    onMove?.(px, py)
  }

  const [positionX, positionY] = preview ?? [x, y]

  return (
    <mesh
      position={[positionX, positionY, VIS_THICKNESS * 0.35]}
      onClick={handleClick}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerOver={(event) => {
        event.stopPropagation()
        setHovered(true)
      }}
      onPointerOut={() => setHovered(false)}
    >
      <boxGeometry args={[width, height, VIS_THICKNESS * 0.5]} />
      <meshStandardMaterial
        ref={material}
        color={color}
        emissive={SELECTED}
        emissiveIntensity={0}
        roughness={0.55}
        metalness={0.05}
        transparent
      />
    </mesh>
  )
}

/** Fits an orthographic camera to the sheet currently on screen, smoothly. */
function FitOrthographic({ width, height }: { width: number; height: number }) {
  const camera = useThree((state) => state.camera)
  const size = useThree((state) => state.size)
  const initialized = useRef(false)

  const target = useMemo(() => {
    const cam = camera as ThreeOrthoCamera
    if (!cam.isOrthographicCamera) return null
    const fitX = size.width / Math.max(width, 1e-6)
    const fitY = size.height / Math.max(height, 1e-6)
    return Math.max(1, Math.min(fitX, fitY) * 0.88)
  }, [camera, size.width, size.height, width, height])

  // Snap to the first fit so the sheet never starts zoomed out.
  useEffect(() => {
    const cam = camera as ThreeOrthoCamera
    if (!cam.isOrthographicCamera || target == null || initialized.current) return
    cam.zoom = target
    cam.updateProjectionMatrix()
    initialized.current = true
  }, [camera, target])

  useFrame((_, delta) => {
    const cam = camera as ThreeOrthoCamera
    if (!cam.isOrthographicCamera || target == null) return
    if (Math.abs(cam.zoom - target) < 0.0005) return
    const k = 1 - Math.exp(-10 * delta)
    cam.zoom += (target - cam.zoom) * k
    cam.updateProjectionMatrix()
  })

  return null
}
