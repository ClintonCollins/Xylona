import { describe, expect, it } from 'vitest'

import {
  formatSevenDaysToDieCoordinate,
  initialSevenDaysToDieMapView,
  sevenDaysToDieTileHasTerrain,
  sevenDaysToDieTileHint,
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

  it('treats a tile as terrain when any pixel is visible', () => {
    expect(sevenDaysToDieTileHasTerrain(new Uint8ClampedArray(16))).toBe(false)
    expect(sevenDaysToDieTileHasTerrain(new Uint8ClampedArray([9, 9, 9, 0, 0, 0, 0, 1]))).toBe(true)
    // Color without alpha is still transparent.
    expect(sevenDaysToDieTileHasTerrain(new Uint8ClampedArray([255, 255, 255, 0]))).toBe(false)
  })

  it.each([
    { states: [], hint: '' },
    { states: ['empty', 'empty'], hint: 'unexplored' },
    { states: ['empty', 'terrain'], hint: '' },
    { states: ['terrain', 'error'], hint: 'error' },
    { states: ['empty', 'error'], hint: 'error' },
  ] as const)('hints "$hint" for tiles $states', ({ states, hint }) => {
    expect(sevenDaysToDieTileHint(states)).toBe(hint)
  })
})
