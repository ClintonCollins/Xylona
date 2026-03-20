<template>
  <q-card class="full-width">
    <q-card-section>
      <div class="row">
        <div class="text-h6">{{ formTitle }}</div>
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
            <q-input
              v-model="game.windowsStartCommand"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Start Command"
              hint="Command to start the server on Windows. e.g: java -jar minecraft_server.jar"></q-input>
            <q-input
              v-model="game.windowsStopCommand"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Stop Command"
              hint="Command to stop the server on Windows. This is sent as server input. e.g: /stop"></q-input>
            <q-input
              v-model="game.windowsInstallCommand"
              :disable="game.windowsInstallCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Install Command"
              hint="Command to install the server on Windows. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></q-input>
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
            <q-input
              v-model="game.windowsUpdateCommand"
              :disable="game.windowsUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Update Command"
              hint="Command to update the server on Windows. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></q-input>
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
            <q-input
              v-model="game.windowsWorkingDirectory"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Windows Working Directory"
              hint="Where should the start command be run from. e.g: ./server"></q-input>
          </div>

          <q-space v-show="game.windowsSupport && game.linuxSupport"></q-space>
          <q-separator v-show="game.windowsSupport && game.linuxSupport"></q-separator>
          <q-space v-show="game.windowsSupport && game.linuxSupport"></q-space>

          <div
            v-show="game.linuxSupport"
            class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
            <q-input
              v-model="game.linuxStartCommand"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Start Command"
              hint="Command to start the server on Linux. e.g: java -jar minecraft_server.jar"></q-input>
            <q-input
              v-model="game.linuxStopCommand"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Stop Command"
              hint="Command to stop the server on Linux. This is sent as server input. e.g: /stop"></q-input>
            <q-input
              v-model="game.linuxInstallCommand"
              :disable="game.linuxInstallCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Install Command"
              hint="Command to install the server on Linux. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></q-input>
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
            <q-input
              v-model="game.linuxUpdateCommand"
              :disable="game.linuxUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Update Command"
              hint="Command to update the server on Linux. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"></q-input>
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
            <q-input
              v-model="game.linuxWorkingDirectory"
              class="col-12 col-xl-6"
              outlined
              type="text"
              label="Linux Working Directory"
              hint="Where should the start command be run from. e.g: ./server"></q-input>
          </div>
        </div>

          <q-space></q-space>
          <q-separator></q-separator>
          <q-space></q-space>

          <config-schema-list
            v-model="configSchemas"
            @edit-schema="navigateToSchemaEditor" />
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

<style scoped></style>
