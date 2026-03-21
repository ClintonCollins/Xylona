<template>
  <q-card class="full-width">
    <q-card-section>
      <div class="row items-center justify-between">
        <div class="text-h6">{{ formTitle }}</div>
        <router-link
          v-if="!existingGame && !copyGame"
          to="/games/new"
          class="text-caption guided-setup-link">
          Use guided setup
        </router-link>
      </div>
    </q-card-section>
    <q-card-section>
      <q-form>
        <div class="column q-gutter-y-md">
          <div class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
            <q-input
              v-model="game.id"
              :disable="existingGame"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Unique ID"
              hint="ID of the game all lowercase. e.g: minecraft"></q-input>
            <q-input
              v-model="game.name"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Name"
              hint="Name of the game. e.g: Minecraft"></q-input>
            <q-input
              v-model.number="defaultPort"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Default Port"
              hint="Default server port. e.g: 25565"></q-input>
            <q-input
              v-model.number="defaultQueryPort"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Default Query Port"
              hint="Default server query port. e.g: 25565"></q-input>
            <q-input
              v-model.number="game.steamAppid"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Steam App ID"
              hint="Steam AppID if it's available on steamcmd. e.g: 294420"></q-input>
          </div>

          <div class="row q-col-gutter-x-sm full-width">
            <q-toggle
              v-model="game.requireDedicatedIp"
              class="col-6 col-xl-2"
              label="Requires Dedicated IP"></q-toggle>
            <q-toggle
              v-model="game.usesSourceQuery"
              class="col-6 col-xl-2"
              label="Uses source query"></q-toggle>
            <q-toggle
              v-model="game.usesSteamcmd"
              class="col-6 col-xl-2"
              label="Uses Steamcmd"></q-toggle>
            <q-toggle
              v-model="game.requiresSteamGameServerLoginToken"
              class="col-6 col-xl-2"
              label="Steam Login Token Required"></q-toggle>
            <q-toggle
              v-model="game.windowsSupport"
              class="col-6 col-xl-2"
              label="Windows Support"></q-toggle>
            <q-toggle
              v-model="game.linuxSupport"
              class="col-6 col-xl-2"
              label="Linux Support"></q-toggle>
          </div>

          <q-space v-show="game.linuxSupport || game.windowsSupport"></q-space>
          <q-separator v-show="game.linuxSupport || game.windowsSupport"></q-separator>
          <q-space v-show="game.linuxSupport || game.windowsSupport"></q-space>

          <div
            v-show="game.windowsSupport"
            class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.windowsStartCommand"
                label="Windows Start Command"
                placeholder="e.g: java -jar minecraft_server.jar"
                command-only />
            </div>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.windowsStopCommand"
                label="Windows Stop Command"
                placeholder="e.g: /stop"
                command-only />
            </div>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.windowsInstallCommand"
                label="Windows Install Command"
                placeholder="e.g: steamcmd +login anonymous +app_update 294420 +quit"
                command-only />
            </div>
            <q-select
              v-model="game.windowsInstallCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Install Command Type"
              map-options
              emit-value
              hint="Direct sends the command directly. Powershell wraps the call in powershell. Internal is a special command that Xylona handles."
              :options="windowsCommandProcessorOptions">
            </q-select>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.windowsUpdateCommand"
                label="Windows Update Command"
                placeholder="e.g: steamcmd +login anonymous +app_update 294420 +quit"
                command-only />
            </div>
            <q-select
              v-model="game.windowsUpdateCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Update Command Type"
              hint="Direct sends the command directly. Powershell wraps the call in powershell. Internal is a special command that Xylona handles."
              map-options
              emit-value
              :options="windowsCommandProcessorOptions">
            </q-select>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.windowsWorkingDirectory"
                label="Windows Working Directory"
                placeholder="e.g: ./server"
                command-only />
            </div>
          </div>

          <q-space v-show="game.windowsSupport && game.linuxSupport"></q-space>
          <q-separator v-show="game.windowsSupport && game.linuxSupport"></q-separator>
          <q-space v-show="game.windowsSupport && game.linuxSupport"></q-space>

          <div
            v-show="game.linuxSupport"
            class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.linuxStartCommand"
                label="Linux Start Command"
                placeholder="e.g: java -jar minecraft_server.jar"
                command-only />
            </div>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.linuxStopCommand"
                label="Linux Stop Command"
                placeholder="e.g: /stop"
                command-only />
            </div>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.linuxInstallCommand"
                label="Linux Install Command"
                placeholder="e.g: steamcmd +login anonymous +app_update 294420 +quit"
                command-only />
            </div>
            <q-select
              v-model="game.linuxInstallCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Install Command Type"
              hint="Direct sends the command directly. Bash wraps the call in bash. Internal is a special command that Xylona handles."
              map-options
              emit-value
              :options="linuxCommandProcessorOptions">
            </q-select>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.linuxUpdateCommand"
                label="Linux Update Command"
                placeholder="e.g: steamcmd +login anonymous +app_update 294420 +quit"
                command-only />
            </div>
            <q-select
              v-model="game.linuxUpdateCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Update Command Type"
              hint="Direct sends the command directly. Bash wraps the call in bash. Internal is a special command that Xylona handles."
              map-options
              emit-value
              :options="linuxCommandProcessorOptions">
            </q-select>
            <div class="col-12 col-xl-6">
              <PlaceholderInput
                v-model="game.linuxWorkingDirectory"
                label="Linux Working Directory"
                placeholder="e.g: ./server"
                command-only />
            </div>
          </div>
        </div>

        <q-space></q-space>
        <q-separator></q-separator>
        <q-space></q-space>

        <config-schema-list v-model="configSchemas" @edit-schema="navigateToSchemaEditor" />
      </q-form>
    </q-card-section>
    <q-separator></q-separator>
    <q-card-actions class="q-pa-md" align="right">
      <q-btn flat label="Cancel" @click="router.back()"></q-btn>
      <q-btn label="Save" color="primary" @click="submit"></q-btn>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import {
  AddGameRequest,
  AddGameRequestSchema,
  EditGameRequest,
  EditGameRequestSchema,
  GetGameRequest,
  GetGameRequestSchema,
  GetGameResponse,
  UpdateGameConfigSchemasRequestSchema,
} from 'src/proto/xylona_pb'
import { GetXylonaClient, ConnectErrorToString } from '@/utils/shared'
import { computed, onMounted, ref, Ref } from 'vue'
import { useRouter } from 'vue-router'
import { CommandProcessor, Game } from '@/proto/shared_pb'
import ConfigSchemaList from './ConfigSchemaList.vue'
import type { ConfigSchemaEntry } from './ConfigSchemaList.vue'
import PlaceholderInput from '@/components/shared/PlaceholderInput.vue'

const $q = useQuasar()
const router = useRouter()
const defaultPort: Ref<number | null> = ref(null)
const defaultQueryPort: Ref<number | null> = ref(null)
const formTitle = computed(() => {
  return existingGame.value ? 'Edit Game' : 'Add Game'
})

const linuxCommandProcessorOptions = [
  { label: 'Direct', value: CommandProcessor.DIRECT },
  { label: 'Bash', value: CommandProcessor.BASH },
  { label: 'Internal', value: CommandProcessor.XYLONA_INTERNAL },
]

const windowsCommandProcessorOptions = [
  { label: 'Direct', value: CommandProcessor.DIRECT },
  { label: 'CMD', value: CommandProcessor.CMD },
  { label: 'PowerShell', value: CommandProcessor.POWERSHELL },
  { label: 'Internal', value: CommandProcessor.XYLONA_INTERNAL },
]

const props = defineProps({
  existingGameId: {
    type: String,
    required: false,
    default: '',
  },
  copyGameId: {
    type: String,
    required: false,
    default: '',
  },
})

const game: Ref<Game> = ref({} as Game)
const existingGame = ref(false)
const copyGame = ref(false)
const gameID = ref('')
const configSchemas = ref<ConfigSchemaEntry[]>([])

async function getGameDetailsFromID() {
  const request: GetGameRequest = create(GetGameRequestSchema, {})
  try {
    request.id = gameID.value
    const response: GetGameResponse = await GetXylonaClient().getGame(request)
    if (response.game === undefined) {
      return
    }
    game.value = response.game
    defaultPort.value = Number(response.game.defaultPort)
    defaultQueryPort.value = Number(response.game.defaultQueryPort)
    if (response.game.configSchemas) {
      try {
        configSchemas.value = JSON.parse(response.game.configSchemas) as ConfigSchemaEntry[]
      } catch {
        configSchemas.value = []
      }
    }
    if (copyGame.value) {
      game.value.id = ''
      game.value.name = `${game.value.name} (Copy)`
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    console.error(err.message)
  }
}

onMounted(async () => {
  if (props.existingGameId !== '') {
    existingGame.value = true
    gameID.value = props.existingGameId
  }
  if (props.copyGameId !== '') {
    copyGame.value = true
    gameID.value = props.copyGameId
  }
  if (existingGame.value || copyGame.value) {
    await getGameDetailsFromID()
  }

  // Check for wizard pre-fill state (only for new game creation, not edit/copy)
  if (!props.existingGameId && !props.copyGameId) {
    const wizardState = history.state?.wizardState
    if (wizardState) {
      game.value.name = wizardState.name || ''
      game.value.id = wizardState.slug || ''
      game.value.steamAppid = wizardState.steamAppId || ''
      game.value.usesSteamcmd = wizardState.usesSteamcmd ?? false
      game.value.windowsSupport = wizardState.windowsSupport ?? false
      game.value.linuxSupport = wizardState.linuxSupport ?? false
      // Pre-fill commands if available
      if (wizardState.installCommand) {
        if (game.value.linuxSupport) game.value.linuxInstallCommand = wizardState.installCommand
        if (game.value.windowsSupport) game.value.windowsInstallCommand = wizardState.installCommand
      }
      if (wizardState.updateCommand) {
        if (game.value.linuxSupport) game.value.linuxUpdateCommand = wizardState.updateCommand
        if (game.value.windowsSupport) game.value.windowsUpdateCommand = wizardState.updateCommand
      }
      if (wizardState.startCommand) {
        if (game.value.linuxSupport) game.value.linuxStartCommand = wizardState.startCommand
        if (game.value.windowsSupport) game.value.windowsStartCommand = wizardState.startCommand
      }
    }
  }
})

function syncConfigSchemas() {
  game.value.configSchemas =
    configSchemas.value.length > 0 ? JSON.stringify(configSchemas.value) : ''
}

async function submit() {
  syncConfigSchemas()
  if (existingGame.value) {
    return await updateExistingGame()
  }
  return await addNewGame()
}

async function navigateToSchemaEditor(fileIndex: number) {
  const id = existingGame.value ? gameID.value : ''
  if (!id) return

  // Persist current config schemas before navigating so the editor can load them
  try {
    const request = create(UpdateGameConfigSchemasRequestSchema, {
      gameId: id,
      configSchemasJson: JSON.stringify(configSchemas.value),
    })
    await GetXylonaClient().updateGameConfigSchemas(request)
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: `Failed to save schemas before editing: ${ConnectErrorToString(err)}`,
      position: 'top',
      timeout: 5000,
    })
    return
  }

  await router.push({ path: `/games/${id}/config-schema/${fileIndex}` })
}

async function addNewGame() {
  const request: AddGameRequest = create(AddGameRequestSchema, {})
  request.game = game.value
  request.game.defaultPort = BigInt(defaultPort.value ?? 0)
  request.game.defaultQueryPort = BigInt(defaultQueryPort.value ?? 0)
  try {
    await GetXylonaClient().addGame(request)
    $q.notify({
      caption: `${game.value.name} added successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    await router.push({ path: '/games' })
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      caption: `Error adding game ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
    console.error(err.message)
  }
}

async function updateExistingGame() {
  const request: EditGameRequest = create(EditGameRequestSchema, {})
  request.game = game.value as Game
  request.game.defaultPort = BigInt(defaultPort.value ?? 0)
  request.game.defaultQueryPort = BigInt(defaultQueryPort.value ?? 0)
  try {
    await GetXylonaClient().editGame(request)
    $q.notify({
      caption: `${game.value.name} updated successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    await router.push({ path: '/games' })
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      caption: `Error updating game ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
    console.error(err.message)
  }
}
</script>

<style scoped>
.guided-setup-link {
  color: var(--xy-accent);
  text-decoration: none;
  transition: opacity var(--xy-transition-fast);
}

.guided-setup-link:hover {
  opacity: 0.8;
  text-decoration: underline;
}
</style>
