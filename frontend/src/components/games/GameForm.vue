<template>
  <q-card class="game-form-card full-width">
    <q-card-section class="game-form-header">
      <div class="game-form-title font-display">{{ formTitle }}</div>
      <div v-if="game.name && existingGame" class="game-form-subtitle text-xy-secondary">
        {{ game.name }}
      </div>
    </q-card-section>

    <q-separator class="form-divider" />

    <q-card-section class="game-form-body">
      <q-form>
        <!-- Game Identity -->
        <section class="form-section">
          <div class="section-header">
            <div class="section-title font-display">Identity</div>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model="game.id"
              :disable="existingGame"
              class="col-12 col-md-6"
              outlined
              type="text"
              label="Unique ID"
              hint="ID of the game all lowercase. e.g: minecraft" />
            <q-input
              v-model="game.name"
              class="col-12 col-md-6"
              outlined
              type="text"
              label="Name"
              hint="Name of the game. e.g: Minecraft" />
          </div>
        </section>

        <!-- Networking -->
        <section class="form-section">
          <div class="section-header">
            <div class="section-title font-display">Networking</div>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model.number="defaultPort"
              class="col-12 col-sm-6 col-md-4"
              outlined
              type="number"
              label="Default Port"
              hint="Default server port. e.g: 25565" />
            <q-input
              v-model.number="defaultQueryPort"
              class="col-12 col-sm-6 col-md-4"
              outlined
              type="number"
              label="Default Query Port"
              hint="Default server query port. e.g: 25565" />
            <q-input
              v-model.number="game.steamAppid"
              class="col-12 col-sm-6 col-md-4"
              outlined
              type="number"
              label="Steam App ID"
              hint="Steam AppID if it's available on steamcmd. e.g: 294420" />
          </div>
        </section>

        <!-- Features -->
        <section class="form-section">
          <div class="section-header">
            <div class="section-title font-display">Features</div>
          </div>
          <div class="row q-col-gutter-x-sm full-width">
            <q-toggle
              v-model="game.requireDedicatedIp"
              class="col-6 col-xl-2"
              label="Requires Dedicated IP" />
            <q-toggle
              v-model="game.usesSourceQuery"
              class="col-6 col-xl-2"
              label="Uses source query" />
            <q-toggle v-model="game.usesSteamcmd" class="col-6 col-xl-2" label="Uses Steamcmd" />
            <q-toggle
              v-model="game.requiresSteamGameServerLoginToken"
              class="col-6 col-xl-2"
              label="Steam Login Token Required" />
            <q-toggle
              v-model="game.windowsSupport"
              class="col-6 col-xl-2"
              label="Windows Support" />
            <q-toggle v-model="game.linuxSupport" class="col-6 col-xl-2" label="Linux Support" />
          </div>
        </section>

        <!-- Windows Commands -->
        <section v-show="game.windowsSupport" class="form-section">
          <div class="section-header">
            <q-icon name="desktop_windows" size="18px" class="section-icon" />
            <div class="section-title font-display">Windows Commands</div>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model="game.windowsStartCommand"
              class="col-12 col-xl-6"
              outlined
              type="textarea"
              autogrow
              input-class="font-mono"
              label="Start Command"
              hint="Command to start the server. e.g: java -jar minecraft_server.jar" />
            <q-input
              v-model="game.windowsStopCommand"
              class="col-12 col-xl-6"
              outlined
              type="text"
              input-class="font-mono"
              label="Stop Command"
              hint="Sent as server input. e.g: /stop" />
            <q-input
              v-model="game.windowsInstallCommand"
              :disable="game.windowsInstallCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="textarea"
              autogrow
              input-class="font-mono"
              label="Install Command"
              hint="Command to install the server. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit" />
            <q-select
              v-model="game.windowsInstallCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              label="Install Command Type"
              map-options
              emit-value
              hint="Direct sends the command directly. Powershell wraps the call in powershell. Internal is a special command that Xylona handles."
              :options="windowsCommandProcessorOptions" />
            <q-input
              v-model="game.windowsUpdateCommand"
              :disable="game.windowsUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="textarea"
              autogrow
              input-class="font-mono"
              label="Update Command"
              hint="Command to update the server. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit" />
            <q-select
              v-model="game.windowsUpdateCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              label="Update Command Type"
              hint="Direct sends the command directly. Powershell wraps the call in powershell. Internal is a special command that Xylona handles."
              map-options
              emit-value
              :options="windowsCommandProcessorOptions" />
            <q-input
              v-model="game.windowsWorkingDirectory"
              class="col-12 col-xl-6"
              outlined
              type="text"
              input-class="font-mono"
              label="Working Directory"
              hint="Where should the start command be run from. e.g: ./server" />
          </div>
        </section>

        <!-- Linux Commands -->
        <section v-show="game.linuxSupport" class="form-section">
          <div class="section-header">
            <q-icon name="terminal" size="18px" class="section-icon" />
            <div class="section-title font-display">Linux Commands</div>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model="game.linuxStartCommand"
              class="col-12 col-xl-6"
              outlined
              type="textarea"
              autogrow
              input-class="font-mono"
              label="Start Command"
              hint="Command to start the server. e.g: java -jar minecraft_server.jar" />
            <q-input
              v-model="game.linuxStopCommand"
              class="col-12 col-xl-6"
              outlined
              type="text"
              input-class="font-mono"
              label="Stop Command"
              hint="Sent as server input. e.g: /stop" />
            <q-input
              v-model="game.linuxInstallCommand"
              :disable="game.linuxInstallCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="textarea"
              autogrow
              input-class="font-mono"
              label="Install Command"
              hint="Command to install the server. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit" />
            <q-select
              v-model="game.linuxInstallCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              label="Install Command Type"
              hint="Direct sends the command directly. Bash wraps the call in bash. Internal is a special command that Xylona handles."
              map-options
              emit-value
              :options="linuxCommandProcessorOptions" />
            <q-input
              v-model="game.linuxUpdateCommand"
              :disable="game.linuxUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
              class="col-12 col-xl-6"
              outlined
              type="textarea"
              autogrow
              input-class="font-mono"
              label="Update Command"
              hint="Command to update the server. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit" />
            <q-select
              v-model="game.linuxUpdateCommandProcessor"
              class="col-12 col-xl-6"
              outlined
              label="Update Command Type"
              hint="Direct sends the command directly. Bash wraps the call in bash. Internal is a special command that Xylona handles."
              map-options
              emit-value
              :options="linuxCommandProcessorOptions" />
            <q-input
              v-model="game.linuxWorkingDirectory"
              class="col-12 col-xl-6"
              outlined
              type="text"
              input-class="font-mono"
              label="Working Directory"
              hint="Where should the start command be run from. e.g: ./server" />
          </div>
        </section>

        <!-- Configuration Files -->
        <section class="form-section form-section--last">
          <config-schema-list v-model="configSchemas" @edit-schema="navigateToSchemaEditor" />
        </section>
      </q-form>
    </q-card-section>

    <q-separator class="form-divider" />

    <q-card-actions class="game-form-actions" align="right">
      <q-btn flat label="Cancel" @click="router.back()" />
      <q-btn label="Save" color="primary" @click="submit" />
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

<style scoped>
.game-form-card {
  overflow: hidden;
}

.game-form-header {
  padding: var(--xy-space-md) var(--xy-space-lg);
}

.game-form-title {
  font-size: 1.1rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  letter-spacing: 0.02em;
}

.game-form-subtitle {
  font-size: 0.8rem;
  margin-top: 2px;
}

.form-divider {
  background-color: var(--xy-border);
}

.game-form-body {
  padding: var(--xy-space-sm) var(--xy-space-lg) var(--xy-space-lg);
}

.form-section {
  padding-top: var(--xy-space-lg);
  border-bottom: 1px solid var(--xy-border);
  padding-bottom: var(--xy-space-lg);
}

.form-section:first-child {
  padding-top: var(--xy-space-sm);
}

.form-section--last {
  border-bottom: none;
  padding-bottom: 0;
}

.section-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  margin-bottom: var(--xy-space-md);
}

.section-icon {
  color: var(--xy-text-muted);
}

.section-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--xy-text-secondary);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.game-form-actions {
  padding: var(--xy-space-sm) var(--xy-space-lg);
}
</style>
