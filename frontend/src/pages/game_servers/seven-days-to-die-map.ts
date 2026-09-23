import type { Coords } from 'leaflet'

interface SevenDaysToDieMapFocusPlayer {
  online: boolean
  position?: { x: number; z: number }
}

export function initialSevenDaysToDieMapView(
  maxZoom: number,
  players: readonly SevenDaysToDieMapFocusPlayer[],
): { center: [number, number]; zoom: number } {
  const player =
    players.find((candidate) => candidate.online && candidate.position !== undefined) ??
    players.find((candidate) => candidate.position !== undefined)
  const position = player?.position
  return {
    center: position === undefined ? [0, 0] : [position.x, position.z],
    zoom: Math.max(0, maxZoom),
  }
}

export function sevenDaysToDieTileURL(template: string, coordinates: Coords): string {
  const nativeY = -coordinates.y - 1
  return template
    .replace('{z}', String(coordinates.z))
    .replace('{x}', String(coordinates.x))
    .replace('{y}', String(nativeY))
}

export function formatSevenDaysToDieCoordinate(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(value)
}

// 7 Days to Die renders only terrain players have explored, so tiles of
// unexplored land are fully transparent rather than missing.
export type SevenDaysToDieTileState = 'terrain' | 'empty' | 'error'
export type SevenDaysToDieTileHint = '' | 'unexplored' | 'error'

/** Whether RGBA pixel data has any pixel that is not fully transparent. */
export function sevenDaysToDieTileHasTerrain(rgba: ArrayLike<number>): boolean {
  for (let alpha = 3; alpha < rgba.length; alpha += 4) {
    if (rgba[alpha] !== 0) {
      return true
    }
  }
  return false
}

/** Load errors win; "unexplored" only when every tile in view is empty. */
export function sevenDaysToDieTileHint(
  states: Iterable<SevenDaysToDieTileState>,
): SevenDaysToDieTileHint {
  let hasEmpty = false
  let hasTerrain = false
  for (const state of states) {
    if (state === 'error') {
      return 'error'
    }
    if (state === 'terrain') {
      hasTerrain = true
    } else {
      hasEmpty = true
    }
  }
  return hasEmpty && !hasTerrain ? 'unexplored' : ''
}
