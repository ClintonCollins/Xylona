import type { ComputedRef, InjectionKey, Ref } from 'vue'
import type {
  CommandType,
  EnvironmentValidationIssue,
  EnvironmentVariable,
  Game,
  ModSource,
} from '@/proto/shared_pb'
import type { StartArgBlock, StartArgBlocklistEntry } from '@/components/game_servers/start-args'
import type { ConfigSchemaEntry } from './config-schema-types'
import type { VariantModSummary } from './useGameFormModProfile'

export type Platform = 'windows' | 'linux'
export type GameFormTabID = 'overview' | 'runtime' | 'mods' | 'config'

export interface GameFormContext {
  game: Ref<Game>
  existingGame: Ref<boolean>
  copyGame: Ref<boolean>

  defaultPort: Ref<number | null>
  defaultQueryPort: Ref<number | null>

  activePlatform: Ref<Platform>
  activePlatformResolved: ComputedRef<Platform | null>

  idRules: Array<(val: string) => true | string>
  nameRules: Array<(val: string) => true | string>
  portRules: Array<(val: number | null | string) => true | string>

  commandTypeOptions: Array<{ label: string; value: number }>
  linuxCommandProcessorOptions: Array<{ label: string; value: number }>
  windowsCommandProcessorOptions: Array<{ label: string; value: number }>
  isCommandTypeCommand: (type: CommandType) => boolean
  commandTypeSummary: (commandType: CommandType, operation: 'install' | 'update') => string
  highlightCommand: (cmd: string) => string

  linuxStartArgsTemplate: Ref<StartArgBlock[]>
  windowsStartArgsTemplate: Ref<StartArgBlock[]>
  startArgBlocklist: Ref<StartArgBlocklistEntry[]>
  baselineLinuxBaseCommand: Ref<string>
  baselineWindowsBaseCommand: Ref<string>
  baselineLinuxStartArgsTemplate: Ref<StartArgBlock[]>
  baselineWindowsStartArgsTemplate: Ref<StartArgBlock[]>

  runtimeSequenceExpanded: Ref<boolean>
  runtimePolicyExpanded: Ref<boolean>
  runtimePolicySummary: ComputedRef<string[]>
  runtimePolicyAssistiveSummary: ComputedRef<string>
  toggleRuntimePolicy: () => void
  updateRuntimeSequenceExpanded: (value: boolean) => void
  downstreamImpactServers: Ref<Array<{ name: string; patchCount: number }>>

  defaultEnvRows: Ref<EnvironmentVariable[]>
  defaultEnvIssues: Ref<EnvironmentValidationIssue[]>
  defaultEnvLoading: Ref<boolean>
  defaultEnvSaving: Ref<boolean>
  addDefaultEnvRow: () => void
  removeDefaultEnvRow: (index: number) => void
  saveDefaultEnvironment: () => Promise<void>

  modSourceOptions: Array<{ label: string; value: string }>
  managedModConfig: ComputedRef<boolean>
  variantModSummaries: ComputedRef<VariantModSummary[]>
  activeModSourceLabel: ComputedRef<string>
  addGameModProfile: () => void
  clearGameModProfile: () => void
  onModSourceProviderChanged: (source: ModSource) => void
  readModSourceDisplayValue: (source: ModSource) => string
  updateModSourceDisplayValue: (source: ModSource, value: string | number | null) => void
  getModSourceConfig: (sourceId: string) => {
    mode: string
    primaryLabel: string
    primaryHint: string
    placeholder: string
  }

  configSchemas: Ref<ConfigSchemaEntry[]>
  navigateToSchemaEditor: (fileIndex: number) => Promise<void>

  managedTypedConfig: ComputedRef<boolean>
}

export const gameFormContextKey: InjectionKey<GameFormContext> = Symbol('GameFormContext')
