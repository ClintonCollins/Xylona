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
                        <q-input :disable="existingGame" class="col-12 col-xl-6" outlined type="text" label="Unique ID"
                                 v-model="game.id" hint="ID of the game all lowercase. e.g: minecraft"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Name"
                                 v-model="game.name" hint="Name of the game. e.g: Minecraft"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Default Port"
                                 v-model.number="defaultPort" hint="Default server port. e.g: 25565"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Default Query Port"
                                 v-model.number="defaultQueryPort"
                                 hint="Default server query port. e.g: 25565"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Steam App ID"
                                 hint="Steam AppID if it's available on steamcmd. e.g: 294420"
                                 v-model.number="game.steamAppid"></q-input>
                    </div>

                    <div class="row q-col-gutter-x-sm full-width">
                        <q-toggle class="col-6 col-xl-2" v-model="game.requireDedicatedIp"
                                  label="Requires Dedicated IP"></q-toggle>
                        <q-toggle class="col-6 col-xl-2" v-model="game.usesSourceQuery"
                                  label="Uses source query"></q-toggle>
                        <q-toggle class="col-6 col-xl-2" v-model="game.usesSteamcmd" label="Uses Steamcmd"></q-toggle>
                        <q-toggle class="col-6 col-xl-2" v-model="game.requiresSteamGameServerLoginToken"
                                  label="Steam Login Token Required"></q-toggle>
                        <q-toggle class="col-6 col-xl-2" v-model="game.windowsSupport"
                                  label="Windows Support"></q-toggle>
                        <q-toggle class="col-6 col-xl-2" v-model="game.linuxSupport" label="Linux Support"></q-toggle>
                    </div>

                    <q-space v-show="game.linuxSupport || game.windowsSupport"></q-space>
                    <q-separator v-show="game.linuxSupport || game.windowsSupport"></q-separator>
                    <q-space v-show="game.linuxSupport || game.windowsSupport"></q-space>

                    <div v-show="game.windowsSupport"
                         class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Windows Start Command"
                                 hint="Command to start the server on Windows. e.g: java -jar minecraft_server.jar"
                                 v-model="game.windowsStartCommand"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Windows Stop Command"
                                 hint="Command to stop the server on Windows. This is sent as server input. e.g: /stop"
                                 v-model="game.windowsStopCommand"></q-input>
                        <q-input :disable="game.windowsInstallCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
                                 class="col-12 col-xl-6" outlined type="text" label="Windows Install Command"
                                 hint="Command to install the server on Windows. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
                                 v-model="game.windowsInstallCommand"></q-input>
                        <q-select class="col-12 col-xl-6" outlined type="text" label="Windows Install Command Type"
                                  map-options emit-value
                                  hint="Direct sends the command directly. Powershell wraps the call in powershell. Internal is a special command that Xylona handles."
                                  v-model="game.windowsInstallCommandProcessor"
                                  :options="windowsCommandProcessorOptions">
                        </q-select>
                        <q-input :disable="game.windowsUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
                                 class="col-12 col-xl-6" outlined type="text" label="Windows Update Command"
                                 hint="Command to update the server on Windows. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
                                 v-model="game.windowsUpdateCommand"></q-input>
                        <q-select class="col-12 col-xl-6" outlined type="text" label="Windows Update Command Type"
                                  hint="Direct sends the command directly. Powershell wraps the call in powershell. Internal is a special command that Xylona handles."
                                  map-options emit-value v-model="game.windowsUpdateCommandProcessor"
                                  :options="windowsCommandProcessorOptions">
                        </q-select>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Windows Working Directory"
                                 hint="Where should the start command be run from. e.g: ./server"
                                 v-model="game.windowsWorkingDirectory"></q-input>
                    </div>

                    <q-space v-show="game.windowsSupport && game.linuxSupport"></q-space>
                    <q-separator v-show="game.windowsSupport && game.linuxSupport"></q-separator>
                    <q-space v-show="game.windowsSupport && game.linuxSupport"></q-space>

                    <div v-show="game.linuxSupport"
                         class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Linux Start Command"
                                 hint="Command to start the server on Linux. e.g: java -jar minecraft_server.jar"
                                 v-model="game.linuxStartCommand"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Linux Stop Command"
                                 hint="Command to stop the server on Linux. This is sent as server input. e.g: /stop"
                                 v-model="game.linuxStopCommand"></q-input>
                        <q-input :disable="game.linuxInstallCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
                                 class="col-12 col-xl-6" outlined type="text" label="Linux Install Command"
                                 hint="Command to install the server on Linux. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
                                 v-model="game.linuxInstallCommand"></q-input>
                        <q-select class="col-12 col-xl-6" outlined type="text" label="Linux Install Command Type"
                                  hint="Direct sends the command directly. Bash wraps the call in bash. Internal is a special command that Xylona handles."
                                  map-options emit-value v-model="game.linuxInstallCommandProcessor"
                                  :options="linuxCommandProcessorOptions">
                        </q-select>
                        <q-input :disable="game.linuxUpdateCommandProcessor === CommandProcessor.XYLONA_INTERNAL"
                                 class="col-12 col-xl-6" outlined type="text" label="Linux Update Command"
                                 hint="Command to update the server on Linux. e.g: steamcmd +login anonymous +force_install_dir ./server +app_update 294420 +quit"
                                 v-model="game.linuxUpdateCommand"></q-input>
                        <q-select class="col-12 col-xl-6" outlined type="text" label="Linux Update Command Type"
                                  hint="Direct sends the command directly. Bash wraps the call in bash. Internal is a special command that Xylona handles."
                                  map-options emit-value
                                  v-model="game.linuxUpdateCommandProcessor"
                                  :options="linuxCommandProcessorOptions">
                        </q-select>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Linux Working Directory"
                                 hint="Where should the start command be run from. e.g: ./server"
                                 v-model="game.linuxWorkingDirectory"></q-input>
                    </div>
                </div>
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
  AddGameRequest, AddGameRequestSchema, EditGameRequest, EditGameRequestSchema,
  GetGameRequest, GetGameRequestSchema,
  GetGameResponse
} from 'src/proto/xylona_pb'
import { GetXylonaClient } from 'src/utils/shared'
import { computed, onMounted, ref, Ref } from 'vue'
import { useRouter } from 'vue-router'
import { CommandProcessor, Game } from '../../proto/shared_pb'

const $q = useQuasar()
const router = useRouter()
const defaultPort: Ref<number | null> = ref(null)
const defaultQueryPort: Ref<number | null> = ref(null)
const formTitle = computed(() => {
  return existingGame.value ? 'Edit Game' : 'Add Game'
})

const linuxCommandProcessorOptions = [
  {label: 'Direct', value: CommandProcessor.DIRECT},
  {label: 'Bash', value: CommandProcessor.BASH},
  {label: 'Internal', value: CommandProcessor.XYLONA_INTERNAL}
]

const windowsCommandProcessorOptions = [
  {label: 'Direct', value: CommandProcessor.DIRECT},
  {label: 'CMD', value: CommandProcessor.CMD},
  {label: 'PowerShell', value: CommandProcessor.POWERSHELL},
  {label: 'Internal', value: CommandProcessor.XYLONA_INTERNAL}
]

const props = defineProps({
  existingGameId: {
    type: String,
    required: false,
    default: ''
  },
  copyGameId: {
    type: String,
    required: false,
    default: ''
  }
})

const game: Ref<Game> = ref({} as Game)
const existingGame = ref(false)
const copyGame = ref(false)
const gameID = ref('')

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

async function submit() {
  if (existingGame.value) {
    return await updateExistingGame()
  }
  return await addNewGame()
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
      timeout: 5000
    })
    await router.push({path: '/games'})
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      caption: `Error adding game ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000
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
      timeout: 5000
    })
    await router.push({path: '/games'})
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      caption: `Error updating game ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000
    })
    console.error(err.message)
  }
}

</script>

<style scoped>

</style>
