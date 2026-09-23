import { describe, expect, it } from 'vitest'

import { fitConsoleSidePanels } from './console-side-panels'

describe('fitConsoleSidePanels', () => {
  const both = { sidebar: true, players: true }

  // Widths are the console main area at common windows with the nav drawer open.
  it.each([
    { label: '1920 keeps both', width: 1622, wanted: both, keep: null, want: both },
    {
      label: '1440 folds the player list first',
      width: 1142,
      wanted: both,
      keep: null,
      want: { sidebar: true, players: false },
    },
    {
      label: '1100 folds both',
      width: 803,
      wanted: both,
      keep: null,
      want: { sidebar: false, players: false },
    },
    {
      label: 'a panel the user opened stays and the other folds',
      width: 983,
      wanted: both,
      keep: 'players' as const,
      want: { sidebar: false, players: true },
    },
    {
      label: 'a panel the user opened stays even when the console gets narrow',
      width: 803,
      wanted: both,
      keep: 'sidebar' as const,
      want: { sidebar: true, players: false },
    },
    {
      label: 'never opens a panel the user closed',
      width: 1622,
      wanted: { sidebar: false, players: true },
      keep: null,
      want: { sidebar: false, players: true },
    },
  ])('$label', ({ width, wanted, keep, want }) => {
    expect(fitConsoleSidePanels(width, wanted, keep)).toEqual(want)
  })
})
