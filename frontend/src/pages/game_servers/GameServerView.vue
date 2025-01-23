<template>
  <q-card-section>
    <div class="row">
      <div class="text-h6">Your Game Server</div>
    </div>
  </q-card-section>
  <div class="row q-gutter-lg-lg q-col-gutter-lg q-px-md">
    <div class="col-lg-4 col-xs-12 q-gutter-md">
      <q-list separator>
        <q-item>
          <q-item-section>Name</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :clip-board-value="gameServer.name" :display-text="gameServer.name"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Status</q-item-section>
          <q-item-section side>
            <StatusBadge :status="gameServer.status"></StatusBadge>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Game</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :clip-board-value="gameServer.gameName" :display-text="gameServer.gameName"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>IP</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :clip-board-value="gameServer.ip !== undefined ? gameServer.ip.address : ''" :display-text="gameServer.ip?.address"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Port</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :clip-board-value="gameServer.port.toString()" :display-text="gameServer.port.toString()"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Version</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :display-text="gameServer.version !== '' ? gameServer.version : 'Unknown version'"  :clip-board-value="gameServer.version !== '' ? gameServer.version : 'Unknown version'"></ClipBoardCopy>
          </q-item-section>
        </q-item>
      </q-list>
      <div class="col-xs-12 col-md-3 q-gutter-md gt-md">
        <q-btn push ripple glossy :disable="disableStartButton" class="bg-success" label="Start"
               @click="startGameServer"></q-btn>
        <q-btn push ripple glossy :disable="disableStopButton" class="bg-error" label="Stop"
               @click="stopGameServer"></q-btn>
      </div>
      <div class="col-xs-12 col-md-3 q-mt-lg lt-lg">
        <q-btn-group spread push>
          <q-btn push ripple glossy :disable="disableStartButton" class="bg-success" label="Start"
                 @click="startGameServer"></q-btn>
          <q-btn push ripple glossy :disable="disableStopButton" class="bg-error" label="Stop"
                 @click="stopGameServer"></q-btn>
        </q-btn-group>
      </div>
    </div>
    <div class="col col-lg-8 col-xs-12" :class="{expanded: consoleExpanded}">
      <q-scroll-area ref="consoleScrollArea" id="consoleContainer">
        <q-page-sticky position="top-right" :offset="[12, -40]">
          <q-btn @click="consoleExpanded = !consoleExpanded" fab flat square padding="sm" :icon="tabMaximize"
                 text-color="info"/>
        </q-page-sticky>
        <code class="q-pb-md" id="consoleCodeEl" v-html="gameServerOutput"></code>
      </q-scroll-area>
      <q-input autofocus id="consoleInput" hint="Send to console" placeholder="Enter command..."
               @keyup.enter="sendGameServerInput"
               dense square outlined name="consoleInput" @keyup.up="navigateConsoleInputHistory('up')"
               @keyup.down="navigateConsoleInputHistory('down')"
               v-model="serverInput">
        <template v-slot:append>
          <q-btn flat color="primary" icon="send" name="send" type="submit" @click="sendGameServerInput"></q-btn>
        </template>
        <!--        <q-menu v-model="showConsoleCommandCompletionsMenu" no-focus anchor="bottom left" self="top left">-->
        <!--          <q-list style="min-width: 100px">-->
        <!--            <q-item v-close-popup v-for="command in consoleCommandCompletionMatches">-->
        <!--              <q-item-section>{{command.label}}</q-item-section>-->
        <!--            </q-item>-->
        <!--          </q-list>-->
        <!--        </q-menu>-->
      </q-input>
    </div>
  </div>
  <q-card-section>
  </q-card-section>
</template>

<script setup lang="ts">
import { create, fromJson, fromJsonString, toJsonString } from '@bufbuild/protobuf'
import {useRoute} from "vue-router";
import {computed, onMounted, Ref, ref} from "vue";
import { GetXylonaClient, StatusToString, XylonaWebsocketBaseURL } from 'src/utils/shared'
import { Message, Message_Type, MessageSchema, Request, Request_Type, RequestSchema } from 'src/proto/websocket_pb'
import {parseConsole} from "src/utils/console";
import {QItemSection, QScrollArea} from "quasar";
import StatusBadge from "components/StatusBadge.vue";
import ClipBoardCopy from "components/ClipBoardCopy.vue";
import {tabMaximize} from 'quasar-extras-svg-icons/tabler-icons-v2'
import {
  GameServer, GameServerSchema, ReadGameServerOutputRequest, ReadGameServerOutputRequestSchema,
  ReadGameServerOutputResponse, SendGameServerInputRequest, SendGameServerInputRequestSchema, StartGameServerRequest,
  StartGameServerRequestSchema,
  Status, StopGameServerRequest,
  StopGameServerRequestSchema
} from '../../proto/shared_pb'
import { GetGameServerRequest, GetGameServerRequestSchema } from '../../proto/xylona_pb'

const gameServerOutput = ref("")
const route = useRoute()
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const serverInput = ref("")
const gameServerId: Ref<string> = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
const consoleScrollArea = ref<QScrollArea | null>(null)
const consoleHistory = ref<string[]>([])
const consoleHistoryCurrentIndex = ref(0)
const consoleExpanded = ref(false)
const maxConsoleCharacters = 100000

const disableStartButton = computed(() => {
  return gameServer.value.status !== Status.OFFLINE && gameServer.value.status !== Status.UNKNOWN
})

const disableStopButton = computed(() => {
  return gameServer.value.status !== Status.ONLINE
})

onMounted(async () => {
  getGameServerDetails().then(() => {
    getGameServerOutput()
  })
  console.log("Streaming game server output")
  streamGameServerOutput()
})

async function getGameServerDetails() {
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
  try {
    request.id = gameServerId.value
    const response = await GetXylonaClient().getGameServer(request)
    if (response.gameServer === undefined) {
      return
    }
    gameServer.value = response.gameServer
  } catch (e) {
    console.error(e)
  }
}

async function startGameServer() {
  const request: StartGameServerRequest = create(StartGameServerRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().startGameServer(request)
  } catch (e) {
    console.error(e)
  }
}

async function stopGameServer() {
  const request: StopGameServerRequest = create(StopGameServerRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().stopGameServer(request)
  } catch (e) {
    console.error(e)
  }
}

async function getGameServerOutput() {
  const request: ReadGameServerOutputRequest = create(ReadGameServerOutputRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    const response: ReadGameServerOutputResponse = await GetXylonaClient().readGameServerOutput(request)
    const start = performance.now()
    gameServerOutput.value = (gameServerOutput.value + parseConsole(gameServer.value.gameId, response.output)).slice(-maxConsoleCharacters)
    const end = performance.now()
    console.warn(`Took ${end - start}ms to parse console output`)
    if (consoleScrollArea.value === null) {
      return
    }
    setTimeout(() => {
      consoleScrollArea.value?.setScrollPercentage("vertical", 100, 0)
    }, 50)
  } catch (e) {
    console.error(e)
  }
}

function streamGameServerOutput() {
  const apiWebsocket = new WebSocket(XylonaWebsocketBaseURL)
  // TODO listen to other page change/close events.
  window.addEventListener("pagehide", () => {
    console.log("Page hide event. Closing websocket...")
    apiWebsocket.close()
  })
  apiWebsocket.onopen = () => {
    console.log("Websocket opened")
    const consoleOutputRequest: Request = create(RequestSchema, {})
    consoleOutputRequest.type = Request_Type.GetGameServerConsole
    consoleOutputRequest.gameServerId = gameServerId.value

    apiWebsocket.send(toJsonString(RequestSchema, consoleOutputRequest))
    console.log('Sent console output request')
  }
  apiWebsocket.onmessage = (event) => {
    const out: Message = fromJsonString(MessageSchema, event.data)
    switch (out.type) {
      case Message_Type.GameServerConsole:
        const start = performance.now()
        gameServerOutput.value = (gameServerOutput.value + parseConsole(gameServer.value.gameId, out.gameServerConsoleOutput!.output)).slice(-maxConsoleCharacters)
        const end = performance.now()
        console.warn(`Took ${end - start}ms to parse console stream output`)
        setTimeout(() => {
          consoleScrollArea.value?.setScrollPercentage("vertical", 100, 0)
        }, 10)
        break
      case Message_Type.GameServerStatus:
        if (out.gameServerStatusUpdate?.gameServerId !== gameServerId.value) {
          return
        }
        console.log(`Game server status update: ${StatusToString(out.gameServerStatusUpdate.status)}. For game server: ${gameServerId.value}`)
        gameServer.value.status = out.gameServerStatusUpdate.status
        break
      default:
        console.log(`${event.data}`)
        return
    }
  }
  apiWebsocket.onclose = (event) => {
    console.log("Websocket closed")
    event.preventDefault()
    apiWebsocket.close()
    setTimeout(streamGameServerOutput, 3000)
  }
  apiWebsocket.onerror = (event) => {
    console.error(event)
    apiWebsocket.close()
  }
}

async function navigateConsoleInputHistory(direction: string) {
  console.log(`Navigating console input history ${direction}`)
  let historyDirection = 0
  switch (direction.toLowerCase()) {
    case "up":
      historyDirection = -1
      break
    case "down":
      historyDirection = 1
      break
    default:
      return
  }
  if (consoleHistory.value.length === 0) {
    return
  }

  if (consoleHistoryCurrentIndex.value > consoleHistory.value.length) {
    return
  }

  let newIndex = consoleHistoryCurrentIndex.value + historyDirection
  console.log(`New index: ${newIndex}, Current index: ${consoleHistoryCurrentIndex.value}`)
  if (newIndex < 0) {
    return
  }
  if (newIndex > consoleHistory.value.length) {
    return
  }
  consoleHistoryCurrentIndex.value = newIndex
  serverInput.value = consoleHistory.value[newIndex]
}

async function sendGameServerInput() {
  console.log(serverInput.value)
  const request: SendGameServerInputRequest = create(SendGameServerInputRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    request.input = serverInput.value
    await GetXylonaClient().sendGameServerInput(request)
  } catch (e) {
    console.error(e)
    alert(e)
  }
  consoleHistory.value.push(serverInput.value)
  consoleHistoryCurrentIndex.value = consoleHistory.value.length
  serverInput.value = ""
}

</script>

<style scoped>
.expanded {
  z-index: 9999 !important;
  width: 100vw !important;
  min-width: 100vw !important;
  height: 100vh !important;
  min-height: 100vh !important;
  position: fixed !important;
  top: 0;
  left: 0;
  margin: 0;
  padding: 0;

  #consoleContainer {
    min-height: 90% !important;
  }
}

#consoleContainer {
  height: 50dvh;
}
</style>
