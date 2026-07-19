import { describe, expect, it } from 'vitest'

import {
  formatSevenDaysToDieCoordinate,
  initialSevenDaysToDieLayerVisibility,
  sevenDaysToDieCoverage,
  sevenDaysToDieMarkerIcon,
  sevenDaysToDieTileURL,
} from './seven-days-to-die-map'

describe('7 Days to Die map helpers', () => {
  it('translates Leaflet tiles into native signed coordinates', () => {
    expect(
      sevenDaysToDieTileURL('/map/{z}/{x}/{y}.png', {
        x: -3,
        y: 4,
        z: 2,
      }),
    ).toBe('/map/2/-3/-5.png')
  })

  it('starts with every operational layer visible', () => {
    expect(initialSevenDaysToDieLayerVisibility()).toEqual({
      players: true,
      markers: true,
      claims: true,
    })
  })

  it.each([
    { name: 'all loaded', loaded: 8, failed: 0, expected: 100 },
    { name: 'partial coverage', loaded: 3, failed: 1, expected: 75 },
    { name: 'no completed tiles', loaded: 0, failed: 0, expected: null },
  ])('reports $name', ({ loaded, failed, expected }) => {
    expect(sevenDaysToDieCoverage(loaded, failed)).toBe(expected)
  })

  it('formats world coordinates without noisy precision', () => {
    expect(formatSevenDaysToDieCoordinate(123.456)).toBe('123.5')
  })

  it.each([
    { icon: 'home', native: false, expected: 'home' },
    { icon: 'directions_car', native: true, expected: 'directions_car' },
    { icon: 'https://example.test/marker.png', native: true, expected: 'flag' },
    { icon: '', native: false, expected: 'edit_location_alt' },
  ])('normalizes the marker icon $icon', ({ icon, native, expected }) => {
    expect(sevenDaysToDieMarkerIcon(icon, native)).toBe(expected)
  })
})
