import { ref, type Ref } from 'vue'

import type { Game } from '@/proto/shared_pb'
import {
  parseStartArgBlocklist,
  parseStartArgsTemplate,
  serializeStartArgBlocklist,
  serializeStartArgsTemplate,
  type StartArgBlock,
  type StartArgBlocklistEntry,
} from '@/components/game_servers/start-args'

function cloneStartArgTemplate(template: StartArgBlock[]): StartArgBlock[] {
  return template.map((block) => ({
    ...block,
    tokens: [...block.tokens],
  }))
}

export function useGameFormStartArgsState(game: Ref<Game>) {
  const linuxStartArgsTemplate = ref<StartArgBlock[]>([])
  const windowsStartArgsTemplate = ref<StartArgBlock[]>([])
  const startArgBlocklist = ref<StartArgBlocklistEntry[]>([])
  const baselineLinuxBaseCommand = ref('')
  const baselineWindowsBaseCommand = ref('')
  const baselineLinuxStartArgsTemplate = ref<StartArgBlock[]>([])
  const baselineWindowsStartArgsTemplate = ref<StartArgBlock[]>([])

  function syncStructuredStartArgsFromGame(): void {
    linuxStartArgsTemplate.value = parseStartArgsTemplate(game.value.linuxStartArgsTemplate)
    windowsStartArgsTemplate.value = parseStartArgsTemplate(game.value.windowsStartArgsTemplate)
    startArgBlocklist.value = parseStartArgBlocklist(game.value.startArgBlocklist)
  }

  function captureRuntimeBaselineFromCurrentState(): void {
    baselineLinuxBaseCommand.value = game.value.linuxBaseCommand
    baselineWindowsBaseCommand.value = game.value.windowsBaseCommand
    baselineLinuxStartArgsTemplate.value = cloneStartArgTemplate(linuxStartArgsTemplate.value)
    baselineWindowsStartArgsTemplate.value = cloneStartArgTemplate(windowsStartArgsTemplate.value)
  }

  function syncStructuredStartArgsToGame(): void {
    game.value.linuxStartArgsTemplate = serializeStartArgsTemplate(linuxStartArgsTemplate.value)
    game.value.windowsStartArgsTemplate = serializeStartArgsTemplate(windowsStartArgsTemplate.value)
    game.value.startArgBlocklist = serializeStartArgBlocklist(startArgBlocklist.value)
  }

  return {
    linuxStartArgsTemplate,
    windowsStartArgsTemplate,
    startArgBlocklist,
    baselineLinuxBaseCommand,
    baselineWindowsBaseCommand,
    baselineLinuxStartArgsTemplate,
    baselineWindowsStartArgsTemplate,
    syncStructuredStartArgsFromGame,
    captureRuntimeBaselineFromCurrentState,
    syncStructuredStartArgsToGame,
  }
}
