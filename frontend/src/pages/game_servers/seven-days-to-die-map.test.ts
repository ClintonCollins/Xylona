import { describe, expect, it } from 'vitest'

import {
  formatSevenDaysToDieCoordinate,
  initialSevenDaysToDieMapView,
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

  it.each([
    {
      name: 'online player',
      maxZoom: 4,
      players: [
        { online: false, position: { x: 10, z: 20 } },
        { online: true, position: { x: 30, z: 40 } },
      ],
      expected: { center: [30, 40], zoom: 4 },
    },
    {
      name: 'last known player',
      maxZoom: 4,
      players: [{ online: false, position: { x: 10, z: 20 } }],
      expected: { center: [10, 20], zoom: 4 },
    },
    {
      name: 'world origin',
      maxZoom: 0,
      players: [],
      expected: { center: [0, 0], zoom: 0 },
    },
  ])('starts near the $name', ({ maxZoom, players, expected }) => {
    expect(initialSevenDaysToDieMapView(maxZoom, players)).toEqual(expected)
  })

  it('formats world coordinates without noisy precision', () => {
    expect(formatSevenDaysToDieCoordinate(123.456)).toBe('123.5')
  })
})
