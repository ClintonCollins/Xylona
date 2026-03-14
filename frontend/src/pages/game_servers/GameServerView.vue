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
            <ClipBoardCopy :clip-board-value="gameServer.name"
                           :display-text="gameServer.name"></ClipBoardCopy>
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
            <ClipBoardCopy :clip-board-value="gameServer.gameName"
                           :display-text="gameServer.gameName"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>IP</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :clip-board-value="gameServer.ip !== undefined ? gameServer.ip.address : ''"
                           :display-text="gameServer.ip?.address"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Port</q-item-section>
          <q-item-section side>
            <ClipBoardCopy :clip-board-value="gameServer.port.toString()"
                           :display-text="gameServer.port.toString()"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Version</q-item-section>
          <q-item-section side>
            <ClipBoardCopy
              :display-text="gameServer.version !== '' ? gameServer.version : 'Unknown version'"
              :clip-board-value="gameServer.version !== '' ? gameServer.version : 'Unknown version'"></ClipBoardCopy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Current player count</q-item-section>
          <q-item-section side>
            {{ currentPlayerCount }} / {{ maxPlayerCount }}
          </q-item-section>
        </q-item>
      </q-list>
      <div class="col-xs-12 col-md-3 q-gutter-md gt-md">
        <q-btn push ripple glossy :disable="disableStartButton" class="bg-success" label="Start"
               @click="startGameServer"></q-btn>
        <q-btn push ripple glossy :disable="disableStopButton" class="bg-error" label="Stop"
               @click="stopGameServer"></q-btn>
        <q-btn push ripple glossy :disable="disableUpdateButton" class="bg-dark" label="Update"
               @click="updateGameServer"></q-btn>
      </div>
      <div class="col-xs-12 col-md-3 q-mt-lg lt-lg">
        <q-btn-group spread push>
          <q-btn push ripple glossy :disable="disableStartButton" class="bg-success" label="Start"
                 @click="startGameServer"></q-btn>
          <q-btn push ripple glossy :disable="disableStopButton" class="bg-error" label="Stop"
                 @click="stopGameServer"></q-btn>
          <q-btn push ripple glossy :disable="disableUpdateButton" class="bg-dark" label="Update"
                 @click="updateGameServer"></q-btn>
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
          <q-btn flat color="primary" icon="send" name="send" type="submit"
                 @click="sendGameServerInput"></q-btn>
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
import {create} from '@bufbuild/protobuf'
import ClipBoardCopy from 'components/ClipBoardCopy.vue'
import StatusBadge from 'components/StatusBadge.vue'
import {QItemSection, QScrollArea} from 'quasar'
import {tabMaximize} from 'quasar-extras-svg-icons/tabler-icons-v2'
import {
  AllServersQueryInfo,
  GameServer,
  GameServerSchema,
  ReadGameServerOutputRequest,
  ReadGameServerOutputRequestSchema,
  ReadGameServerOutputResponse,
  SendGameServerInputRequest,
  SendGameServerInputRequestSchema,
  ServerQuery_Type,
  StartGameServerRequest,
  StartGameServerRequestSchema,
  Status,
  StopGameServerRequest,
  StopGameServerRequestSchema
} from 'src/proto/shared_pb'
import {
  GetGameServerRequest,
  GetGameServerRequestSchema,
  QueryGameServerRequest,
  QueryGameServerRequestSchema,
  QueryGameServerResponse,
  UpdateGameServerRequest,
  UpdateGameServerRequestSchema
} from 'src/proto/xylona_pb'
import {parseConsole} from 'src/utils/console'
import {GetXylonaClient, XylonaEventBus} from 'src/utils/shared'
import {computed, onMounted, Ref, ref} from 'vue'
import {useRoute} from 'vue-router'

const gameServerOutput = ref('')
const route = useRoute()
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const serverInput = ref('')
const gameServerId: Ref<string> = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
const consoleScrollArea = ref<QScrollArea | null>(null)
const consoleHistory = ref<string[]>([])
const consoleHistoryCurrentIndex = ref(0)
const consoleExpanded = ref(false)
const maxConsoleCharacters = 100000

const currentPlayerCount: Ref<number> = ref(0)
const maxPlayerCount: Ref<number> = ref(0)

const disableStartButton = computed(() => {
  return gameServer.value.status !== Status.OFFLINE && gameServer.value.status !== Status.UNKNOWN
})

const disableStopButton = computed(() => {
  return gameServer.value.status !== Status.ONLINE
})

const disableUpdateButton = computed(() => {
  return gameServer.value.status === Status.INSTALLING || gameServer.value.status === Status.UPDATING ||
    gameServer.value.status === Status.ONLINE
})

onMounted(async () => {
  getGameServerDetails().then(() => {
    getGameServerOutput()
    streamGameServerOutput()
    listenForServerQueryInfo()
  }).then(queryGameServer)
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

async function queryGameServer() {
  const request: QueryGameServerRequest = create(QueryGameServerRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    const resp: QueryGameServerResponse = await GetXylonaClient().queryGameServer(request)
    switch (resp.queryInfo?.type) {
      case ServerQuery_Type.Minecraft:
        currentPlayerCount.value = resp.queryInfo.minecraft.numberOfPlayers
        maxPlayerCount.value = resp.queryInfo.minecraft.maxPlayers
        break
      case ServerQuery_Type.Source:
        currentPlayerCount.value = resp.queryInfo.source.players
        maxPlayerCount.value = resp.queryInfo.source.maxPlayers
        break
    }
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

async function updateGameServer() {
  const request: UpdateGameServerRequest = create(UpdateGameServerRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().updateGameServer(request)
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
      consoleScrollArea.value?.setScrollPercentage('vertical', 100, 0)
    }, 50)
  } catch (e) {
    console.error(e)
  }
}

function listenForServerQueryInfo() {
  XylonaEventBus.on('gameServersQueryInfo', (allServersQueryInfo: AllServersQueryInfo) => {
    const queryInfo = allServersQueryInfo.servers[gameServerId.value]
    if (queryInfo === undefined) {
      return
    }
    switch (queryInfo.type) {
      case ServerQuery_Type.Minecraft:
        currentPlayerCount.value = queryInfo.minecraft.numberOfPlayers
        maxPlayerCount.value = queryInfo.minecraft.maxPlayers
        break
      case ServerQuery_Type.Source:
        currentPlayerCount.value = queryInfo.source.players
        maxPlayerCount.value = queryInfo.source.maxPlayers
        break
    }
  })
}

function streamGameServerOutput() {
  // Listen for game server status changes.
  XylonaEventBus.on('gameServerStatus', (serverID: string, serverStatus: Status) => {
    if (serverID !== gameServerId.value) {
      return
    }
    gameServer.value.status = serverStatus
  })

  // Stream game server output.
  XylonaEventBus.on('gameServerConsoleOutput', (serverID: string, output: string) => {
    if (serverID !== gameServerId.value) {
      return
    }
    gameServerOutput.value = (gameServerOutput.value + parseConsole(gameServer.value.gameId, output)).slice(-maxConsoleCharacters)
    if (consoleScrollArea.value === null) {
      return
    }
    setTimeout(() => {
      consoleScrollArea.value?.setScrollPercentage('vertical', 100, 0)
    }, 50)
  })
  // Request the game server to start streaming output.
  XylonaEventBus.emit('gameServerConsoleOutputRequest', gameServerId.value)

  // Handle socket reconnection.
  XylonaEventBus.on('websocketConnected', () => {
    XylonaEventBus.emit('gameServerConsoleOutputRequest', gameServerId.value)
  })
}

async function navigateConsoleInputHistory(direction: string) {
  console.log(`Navigating console input history ${direction}`)
  let historyDirection = 0
  switch (direction.toLowerCase()) {
    case 'up':
      historyDirection = -1
      break
    case 'down':
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
  console.debug(`New index: ${newIndex}, Current index: ${consoleHistoryCurrentIndex.value}`)
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
  serverInput.value = ''
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
