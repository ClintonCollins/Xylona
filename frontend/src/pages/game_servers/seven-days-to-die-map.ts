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
