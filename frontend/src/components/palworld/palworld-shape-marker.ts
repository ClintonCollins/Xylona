import L, {
  type CircleMarker,
  type CircleMarkerOptions,
  type LatLngExpression,
  type PathOptions,
} from 'leaflet'

import type { PalworldMapMarkerShape } from '@/pages/game_servers/palworld-map'

export interface PalworldShapeMarkerOptions extends CircleMarkerOptions {
  shape: PalworldMapMarkerShape
}

// Leaflet's canvas renderer exposes no public seam for a custom path, so these
// are the internals this module depends on. They are all present in the exact
// leaflet version this package pins; the drawing code degrades to Leaflet's own
// circle rather than throwing if a renderer does not provide them.
export interface PalworldShapeMarkerInternals {
  // Only the shape and the style fields matter while drawing, so this stays
  // looser than the construction options, which also require a radius.
  options: PathOptions & { shape: PalworldMapMarkerShape }
  _point: { x: number; y: number }
  _radius: number
  _empty?: () => boolean | number
  _renderer: {
    _ctx?: CanvasRenderingContext2D
    _drawing?: boolean
    _fillStroke?: (context: CanvasRenderingContext2D, layer: unknown) => void
    _updateCircle: (layer: unknown) => void
  }
}

// A square drawn at the circle's radius reads as the heaviest marker on the map,
// so it is pulled in to roughly match the circle's area instead.
const squareRadiusRatio = 0.86

export function drawPalworldShape(marker: PalworldShapeMarkerInternals): boolean {
  const renderer = marker._renderer
  const context = renderer._ctx
  // An SVG renderer offers neither a 2D context nor a path finisher, so the
  // caller falls back to the circle Leaflet already knows how to draw there.
  if (context === undefined || renderer._fillStroke === undefined) {
    return false
  }
  // Leaflet only permits painting inside its own draw pass, where the canvas is
  // saved and clipped; every built-in shape makes this same check. A radius of
  // zero makes _empty return 0 rather than true, and still has to be drawn.
  if (renderer._drawing !== true || marker._empty?.() === true) {
    return true
  }
  const centerX = marker._point.x
  const centerY = marker._point.y
  const radius = Math.max(Math.round(marker._radius), 1)
  context.beginPath()
  if (marker.options.shape === 'diamond') {
    context.moveTo(centerX, centerY - radius)
    context.lineTo(centerX + radius, centerY)
    context.lineTo(centerX, centerY + radius)
    context.lineTo(centerX - radius, centerY)
  } else {
    const half = radius * squareRadiusRatio
    context.moveTo(centerX - half, centerY - half)
    context.lineTo(centerX + half, centerY - half)
    context.lineTo(centerX + half, centerY + half)
    context.lineTo(centerX - half, centerY + half)
  }
  context.closePath()
  // Styling stays Leaflet's job, so fill, stroke, weight 0 and dash options all
  // keep behaving as they do for a circle.
  renderer._fillStroke(context, marker)
  return true
}

// Leaflet ships no non-circular canvas marker, and drawing every dot as a DOM
// icon at world zoom is exactly the cost the last performance pass removed. This
// subclasses CircleMarker so dots stay on the shared canvas and only the path
// drawing changes, which is the same seam Circle and Polyline extend.
//
// It lives outside the SFC deliberately: `<script setup>` runs per component
// instance, so declaring the class there rebuilt it on every mount, and .vue
// files are not covered by this project's typechecker.
// Leaflet's `extend` is typed as returning a zero-argument constructor, so the
// real CircleMarker signature is restated here rather than lost to `any`.
interface PalworldShapeMarkerClass {
  new (location: LatLngExpression, options: PalworldShapeMarkerOptions): CircleMarker
  prototype: object
}

export const PalworldShapeMarker = L.CircleMarker.extend({
  options: { shape: 'circle' as PalworldMapMarkerShape },

  _updatePath(): void {
    const marker = this as unknown as PalworldShapeMarkerInternals
    if (marker.options.shape === 'circle' || !drawPalworldShape(marker)) {
      marker._renderer._updateCircle(marker)
    }
  },
}) as unknown as PalworldShapeMarkerClass

export function palworldShapeMarker(
  location: LatLngExpression,
  options: PalworldShapeMarkerOptions,
): CircleMarker {
  return new PalworldShapeMarker(location, options)
}
