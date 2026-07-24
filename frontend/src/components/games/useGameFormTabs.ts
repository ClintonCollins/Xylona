import { nextTick, ref, watch } from 'vue'

import type { GameFormTabID } from './GameFormTypes'

const gameFormTabHistoryStateKey = 'xylonaGameFormTab'

const formTabs: Array<{ id: GameFormTabID; label: string; copy: string }> = [
  {
    id: 'overview',
    label: 'Overview',
    copy: 'Identity, ports, platform support, and install or update commands.',
  },
  {
    id: 'runtime',
    label: 'Runtime',
    copy: 'Launch sequence authoring, base command, and advanced runtime policy.',
  },
  {
    id: 'mods',
    label: 'Mods',
    copy: 'Optional mod source wiring for custom game definitions.',
  },
  {
    id: 'console-commands',
    label: 'Console Commands',
    copy: 'Command reference entries, arguments, examples, availability, and risk.',
  },
  {
    id: 'config',
    label: 'Config Files',
    copy: 'Managed configuration files and schema definitions for game servers.',
  },
]

function isGameFormTabID(value: unknown): value is GameFormTabID {
  return (
    value === 'overview' ||
    value === 'runtime' ||
    value === 'mods' ||
    value === 'console-commands' ||
    value === 'config'
  )
}

function readActiveFormTabFromHistory(): GameFormTabID {
  if (typeof window === 'undefined') {
    return 'overview'
  }

  const historyTab = window.history.state?.[gameFormTabHistoryStateKey]
  return isGameFormTabID(historyTab) ? historyTab : 'overview'
}

function persistActiveFormTabToHistory(tabID: GameFormTabID): void {
  if (typeof window === 'undefined') {
    return
  }

  window.history.replaceState(
    {
      ...(window.history.state ?? {}),
      [gameFormTabHistoryStateKey]: tabID,
    },
    '',
  )
}

export function useGameFormTabs() {
  const activeFormTab = ref<GameFormTabID>(readActiveFormTabFromHistory())

  function formTabID(tabID: GameFormTabID): string {
    return `game-form-tab-${tabID}`
  }

  function formTabPanelID(tabID: GameFormTabID): string {
    return `game-form-tab-panel-${tabID}`
  }

  function focusFormTab(tabID: GameFormTabID): void {
    void nextTick(() => {
      const tabElement = document.getElementById(formTabID(tabID))
      if (!(tabElement instanceof HTMLButtonElement)) {
        return
      }

      tabElement.focus()
    })
  }

  function cycleFormTab(fromTabID: GameFormTabID, step: number): void {
    const currentIndex = formTabs.findIndex((tab) => tab.id === fromTabID)
    if (currentIndex === -1) {
      return
    }

    const nextIndex = (currentIndex + step + formTabs.length) % formTabs.length
    const nextTab = formTabs[nextIndex]
    if (!nextTab) {
      return
    }

    activeFormTab.value = nextTab.id
    focusFormTab(nextTab.id)
  }

  function handleFormTabKeydown(event: KeyboardEvent, tabID: GameFormTabID): void {
    if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
      event.preventDefault()
      cycleFormTab(tabID, 1)
      return
    }

    if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
      event.preventDefault()
      cycleFormTab(tabID, -1)
      return
    }

    if (event.key === 'Home') {
      event.preventDefault()
      const firstTab = formTabs[0]
      if (!firstTab) {
        return
      }

      activeFormTab.value = firstTab.id
      focusFormTab(firstTab.id)
      return
    }

    if (event.key === 'End') {
      event.preventDefault()
      const lastTab = formTabs[formTabs.length - 1]
      if (!lastTab) {
        return
      }

      activeFormTab.value = lastTab.id
      focusFormTab(lastTab.id)
    }
  }

  watch(
    activeFormTab,
    (tabID) => {
      persistActiveFormTabToHistory(tabID)
    },
    { immediate: true },
  )

  return {
    formTabs,
    activeFormTab,
    formTabID,
    formTabPanelID,
    handleFormTabKeydown,
  }
}
