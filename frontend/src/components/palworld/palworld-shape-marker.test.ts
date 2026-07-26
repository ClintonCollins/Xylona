import { describe, expect, it, vi } from 'vitest'

import {
  drawPalworldShape,
  palworldShapeMarker,
  PalworldShapeMarker,
  type PalworldShapeMarkerInternals,
  type PalworldShapeMarkerOptions,
} from './palworld-shape-marker'

// Invokes the real subclass override against a stub layer, so the dispatch to
// Leaflet's own circle drawing is exercised rather than assumed.
function updatePathOf(layer: PalworldShapeMarkerInternals): void {
  const prototype = PalworldShapeMarker.prototype as unknown as {
    _updatePath: (this: PalworldShapeMarkerInternals) => void
  }
  prototype._updatePath.call(layer)
}

// Deliberately exercises the real Leaflet build rather than the SFC test's mock:
// the drawing seam is renderer internals, so a stubbed Leaflet proves nothing.
type PathCall = [string, ...number[]]

function stubContext(calls: PathCall[]): CanvasRenderingContext2D {
  const record =
    (name: string) =>
    (...args: number[]): void => {
      calls.push([name, ...args])
    }
  return {
    beginPath: record('beginPath'),
    moveTo: record('moveTo'),
    lineTo: record('lineTo'),
    closePath: record('closePath'),
  } as unknown as CanvasRenderingContext2D
}

function shapeLayer(
  shape: PalworldShapeMarkerOptions['shape'],
  overrides: Partial<PalworldShapeMarkerInternals> = {},
  contextCalls: PathCall[] = [],
  fillStroke = vi.fn(),
): PalworldShapeMarkerInternals {
  return {
    options: { shape },
    _point: { x: 100, y: 200 },
    _radius: 6,
    _empty: () => false,
    _renderer: {
      _ctx: stubContext(contextCalls),
      _drawing: true,
      _fillStroke: fillStroke,
      _updateCircle: vi.fn(),
    },
    ...overrides,
  }
}

describe('palworldShapeMarker', () => {
  it('builds a real Leaflet CircleMarker carrying its shape', () => {
    const marker = palworldShapeMarker(
      { lat: 10, lng: 20 },
      { shape: 'diamond', radius: 5, fillColor: '#8b5cf6' },
    )

    expect(marker.getLatLng()).toMatchObject({ lat: 10, lng: 20 })
    expect(marker.options).toMatchObject({ shape: 'diamond', radius: 5, fillColor: '#8b5cf6' })
    // Leaflet's own styling path must keep working, since drawing delegates to it.
    expect(marker.options.fill).toBe(true)
    expect(marker.options.stroke).toBe(true)
  })

  it.each([
    { name: 'a circle is never hand-drawn', shape: 'circle' as const, context: true },
    {
      name: 'a shape falls back when the renderer has no 2D context',
      shape: 'diamond' as const,
      context: false,
    },
  ])('routes through Leaflet when $name', ({ shape, context }) => {
    const calls: PathCall[] = []
    const layer = shapeLayer(shape, {}, calls)
    if (!context) {
      delete layer._renderer._ctx
    }

    updatePathOf(layer)

    expect(layer._renderer._updateCircle).toHaveBeenCalledTimes(1)
    expect(calls).toEqual([])
  })

  it.each([
    {
      name: 'a diamond as four points on the radius',
      shape: 'diamond' as const,
      expected: [
        ['beginPath'],
        ['moveTo', 100, 194],
        ['lineTo', 106, 200],
        ['lineTo', 100, 206],
        ['lineTo', 94, 200],
        ['closePath'],
      ],
    },
    {
      name: 'a square pulled in to match the circle area',
      shape: 'square' as const,
      expected: [
        ['beginPath'],
        ['moveTo', 94.84, 194.84],
        ['lineTo', 105.16, 194.84],
        ['lineTo', 105.16, 205.16],
        ['lineTo', 94.84, 205.16],
        ['closePath'],
      ],
    },
  ])('draws $name', ({ shape, expected }) => {
    const calls: PathCall[] = []
    const fillStroke = vi.fn()
    const layer = shapeLayer(shape, {}, calls, fillStroke)

    expect(drawPalworldShape(layer)).toBe(true)
    expect(calls).toEqual(expected)
    // Fill and stroke stay Leaflet's job so weight 0 and dash options still apply.
    expect(fillStroke).toHaveBeenCalledTimes(1)
  })

  it('reports the shape unhandled when the renderer offers no path finisher', () => {
    const calls: PathCall[] = []
    const layer = shapeLayer('diamond', {}, calls)
    delete layer._renderer._fillStroke

    expect(drawPalworldShape(layer)).toBe(false)
    expect(calls).toEqual([])
  })

  it('claims the shape but paints nothing outside Leaflet own draw pass', () => {
    const calls: PathCall[] = []
    const layer = shapeLayer('diamond', {}, calls)
    layer._renderer._drawing = false

    // Handled, so the caller must not fall through to _updateCircle, which
    // makes the same check and would also refuse to paint.
    expect(drawPalworldShape(layer)).toBe(true)
    expect(calls).toEqual([])
  })

  it('still draws a zero-radius marker, because _empty reports 0 rather than true', () => {
    const calls: PathCall[] = []
    const layer = shapeLayer('diamond', { _radius: 0, _empty: () => 0 }, calls)

    expect(drawPalworldShape(layer)).toBe(true)
    // Radius clamps to 1 so the actor never silently vanishes mid-animation.
    expect(calls).toContainEqual(['moveTo', 100, 199])
  })
})
