import { computed, inject, type ComputedRef, type InjectionKey } from 'vue'

/** The open server's name, provided by GameServerLayout to section pages and their dialogs. */
export const gameServerNameKey: InjectionKey<ComputedRef<string>> = Symbol('gameServerName')

/** Empty outside a server's pages, so callers can fall back to generic copy. */
export function useGameServerName(): ComputedRef<string> {
  return inject(gameServerNameKey, () => computed(() => ''), true)
}
