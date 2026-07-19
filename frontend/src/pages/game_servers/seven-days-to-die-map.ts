import type { Coords } from 'leaflet'

export interface SevenDaysToDieLayerVisibility {
  players: boolean
  markers: boolean
  claims: boolean
}

export function initialSevenDaysToDieLayerVisibility(): SevenDaysToDieLayerVisibility {
  return { players: true, markers: true, claims: true }
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

export function sevenDaysToDieMarkerIcon(icon: string, native: boolean): string {
  const normalized = icon.trim()
  if (/^[a-z][a-z0-9_]{0,63}$/.test(normalized)) {
    return normalized
  }
  return native ? 'flag' : 'edit_location_alt'
}

export function sevenDaysToDieCoverage(loaded: number, failed: number): number | null {
  const completed = loaded + failed
  if (completed === 0) {
    return null
  }
  return Math.round((loaded / completed) * 100)
}
