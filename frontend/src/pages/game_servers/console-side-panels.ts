// Widths match the .sidebar, .player-rail and collapsed-strip rules in GameServerView.vue.
export const consoleMinWidth = 640
const sidebarWidth = 290
const playerRailWidth = 264
const collapsedStripWidth = 44

export type ConsoleSidePanel = 'sidebar' | 'players'

export type ConsoleSidePanels = Record<ConsoleSidePanel, boolean>

/**
 * Folds the side panels the console has no room for, so its output keeps at
 * least `consoleMinWidth` px. The player list folds first (its count stays in
 * the collapsed strip), then server details. `keep` is the panel the user
 * opened last; it stays open even when that leaves the console narrower.
 */
export function fitConsoleSidePanels(
  availableWidth: number,
  wanted: ConsoleSidePanels,
  keep: ConsoleSidePanel | null,
): ConsoleSidePanels {
  const open = { ...wanted }
  const consoleWidth = () =>
    availableWidth -
    (open.sidebar ? sidebarWidth : collapsedStripWidth) -
    (open.players ? playerRailWidth : collapsedStripWidth)

  for (const panel of ['players', 'sidebar'] as const) {
    if (consoleWidth() >= consoleMinWidth) break
    if (panel !== keep) open[panel] = false
  }
  return open
}
